package public

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/ndresdavila/platform-api/internal/adapters/keycloak"
	"github.com/ndresdavila/platform-api/internal/domain/ports"
)

type contextKey string

const customerKey contextKey = "customer"
const staffKey contextKey = "staff"

func core(h *Handler) ports.CoreClient      { return h.Facade.Core }
func jsonBody(r *http.Request, v any) error { return json.NewDecoder(r.Body).Decode(v) }
func bffErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
func bearer(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if strings.HasPrefix(v, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(v, "Bearer "))
	}
	return r.URL.Query().Get("access_token")
}

func newBFFRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Logger, chimw.Recoverer)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := core(h).Health(r.Context()); err != nil {
			writeJSON(w, 503, map[string]any{"ok": false, "core": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", http.FileServer(http.Dir(h.UploadDir))))
	r.Route("/api", func(r chi.Router) {
		r.Get("/features", h.features)
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", h.register)
			r.Post("/login", h.login)
			r.Post("/session", h.session)
			r.Post("/refresh", h.refresh)
			r.Group(func(r chi.Router) { r.Use(h.requireCustomer); r.Get("/me", h.me); r.Patch("/profile", h.profile) })
		})
		r.Group(func(r chi.Router) {
			r.Use(h.requireCustomer, h.requireProfile)
			r.Get("/servicios", h.services)
			r.Get("/citas", h.citas)
			r.Post("/citas", h.createCita)
			r.Patch("/citas/{id}/cancelar", h.cancelCita)
			r.Get("/pagos/cuentas", h.accounts)
			r.Get("/pagos", h.pagos)
			r.Post("/pagos/reportar", h.reportPago)
			r.Get("/disenos/quota", h.designQuota)
			r.Get("/disenos", h.designs)
			r.Post("/disenos", h.createDesign)
			r.Delete("/disenos/{id}", h.deleteDesign)
		})
		r.Route("/staff", func(r chi.Router) {
			r.Use(h.requireStaff)
			r.Get("/me", h.staffMe)
			r.Get("/citas", h.staffCitas)
			r.Get("/citas/{id}", h.staffCita)
			r.Post("/citas/{id}/avanzar", h.advance)
			r.Post("/citas/{id}/cancelar", h.staffCancel)
			r.Post("/pagos/{id}/verificar", h.verifyPayment)
			r.Post("/pagos/{id}/anular", h.voidPayment)
			r.Get("/pagos/{id}/comprobante", h.receipt)
		})
	})
	return r
}
func (h *Handler) customerFromPayload(ctx context.Context, p *keycloak.Payload) (*ports.Customer, error) {
	email, first, last := keycloak.Profile(p)
	if c, err := core(h).GetCustomerByExternal(ctx, h.Tenant.ID, p.Subject); err == nil {
		return c, nil
	}
	return core(h).UpsertCustomer(ctx, h.Tenant.ID, ports.UpsertCustomerInput{
		ExternalID: p.Subject, Email: email, FirstName: first, LastName: last, Role: keycloak.RolesFromPayload(p),
	})
}
func (h *Handler) requireCustomer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearer(r)
		if token == "" {
			bffErr(w, 401, "No autorizado")
			return
		}
		p, err := keycloak.VerifyAccessToken(r.Context(), token)
		if err != nil {
			bffErr(w, 401, "Token inválido")
			return
		}
		c, err := h.customerFromPayload(r.Context(), p)
		if err != nil {
			bffErr(w, 502, err.Error())
			return
		}
		if !c.Active {
			bffErr(w, 403, "Cuenta desactivada")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), customerKey, c)))
	})
}
func customer(r *http.Request) *ports.Customer {
	v, _ := r.Context().Value(customerKey).(*ports.Customer)
	return v
}
func (h *Handler) requireProfile(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c := customer(r); c == nil || c.NationalID == nil || *c.NationalID == "" {
			writeJSON(w, 403, map[string]string{"error": "Completa tu perfil (cédula) para continuar", "code": "PROFILE_INCOMPLETE"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
func userDTO(c *ports.Customer) map[string]any {
	var cedula any
	if c.NationalID != nil {
		cedula = *c.NationalID
	}
	return map[string]any{"id": c.ID, "keycloakId": c.ExternalID, "cedula": cedula, "email": c.Email, "nombre": c.FirstName, "apellido": c.LastName, "telefono": c.Phone, "rol": c.Role, "needsProfile": c.NationalID == nil || *c.NationalID == ""}
}
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var b struct{ Cedula, Email, Password, Nombre, Apellido, Telefono string }
	if jsonBody(r, &b) != nil || len(b.Cedula) < 10 || b.Email == "" || len(b.Password) < 6 {
		bffErr(w, 400, "Datos de registro inválidos")
		return
	}
	id, err := keycloak.CreateUser(r.Context(), b.Email, b.Password, b.Nombre, b.Apellido)
	if err != nil {
		bffErr(w, 409, err.Error())
		return
	}
	if err = keycloak.AssignRealmRole(r.Context(), id, "cliente"); err != nil {
		bffErr(w, 502, err.Error())
		return
	}
	tokens, err := keycloak.PasswordGrant(r.Context(), strings.ToLower(b.Email), b.Password)
	if err != nil {
		bffErr(w, 401, err.Error())
		return
	}
	c, err := core(h).UpsertCustomer(r.Context(), h.Tenant.ID, ports.UpsertCustomerInput{ExternalID: id, Email: strings.ToLower(b.Email), FirstName: b.Nombre, LastName: b.Apellido, Phone: b.Telefono, NationalID: &b.Cedula, Role: "CLIENTE"})
	if err != nil {
		bffErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"accessToken": tokens.AccessToken, "refreshToken": tokens.RefreshToken, "expiresIn": tokens.ExpiresIn, "user": userDTO(c)})
}
func (h *Handler) tokenResponse(w http.ResponseWriter, r *http.Request, t *keycloak.TokenSet) {
	p, err := keycloak.VerifyAccessToken(r.Context(), t.AccessToken)
	if err != nil {
		bffErr(w, 401, "Token inválido")
		return
	}
	c, err := h.customerFromPayload(r.Context(), p)
	if err != nil {
		bffErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"accessToken": t.AccessToken, "refreshToken": t.RefreshToken, "expiresIn": t.ExpiresIn, "user": userDTO(c)})
}
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var b struct{ Email, Password string }
	if jsonBody(r, &b) != nil {
		bffErr(w, 400, "JSON inválido")
		return
	}
	t, err := keycloak.PasswordGrant(r.Context(), strings.ToLower(b.Email), b.Password)
	if err != nil {
		bffErr(w, 401, "Credenciales inválidas")
		return
	}
	h.tokenResponse(w, r, t)
}
func (h *Handler) session(w http.ResponseWriter, r *http.Request) {
	token := bearer(r)
	p, err := keycloak.VerifyAccessToken(r.Context(), token)
	if err != nil {
		bffErr(w, 401, "Token inválido")
		return
	}
	c, err := h.customerFromPayload(r.Context(), p)
	if err != nil {
		bffErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"user": userDTO(c)})
}
func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var b struct {
		RefreshToken string `json:"refreshToken"`
	}
	if jsonBody(r, &b) != nil || b.RefreshToken == "" {
		bffErr(w, 400, "refreshToken requerido")
		return
	}
	t, err := keycloak.RefreshGrant(r.Context(), b.RefreshToken)
	if err != nil {
		bffErr(w, 401, "Refresh inválido")
		return
	}
	h.tokenResponse(w, r, t)
}
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"user": userDTO(customer(r))})
}
func (h *Handler) profile(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Cedula    string `json:"cedula"`
		Nombre    string `json:"nombre"`
		Apellido  string `json:"apellido"`
		Telefono  string `json:"telefono"`
	}
	if jsonBody(r, &b) != nil || len(strings.TrimSpace(b.Cedula)) < 10 {
		bffErr(w, 400, "Cédula inválida")
		return
	}
	c, err := core(h).UpdateCustomerProfile(r.Context(), h.Tenant.ID, customer(r).ID, ports.CustomerProfileInput{
		NationalID: strings.TrimSpace(b.Cedula), FirstName: strings.TrimSpace(b.Nombre),
		LastName: strings.TrimSpace(b.Apellido), Phone: strings.TrimSpace(b.Telefono),
	})
	if err != nil {
		bffErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"user": userDTO(c)})
}
func (h *Handler) features(w http.ResponseWriter, r *http.Request) {
	s, err := core(h).GetSettings(r.Context(), h.Tenant.ID)
	if err != nil {
		bffErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"features": map[string]bool{"disenoIa": s.FeatureDesignAI}, "disenoIaDailyLimit": s.DesignAIDailyLimit, "disenoIaMaxTotal": s.DesignAIMaxTotal})
}
func svcDTO(s ports.CatalogService) map[string]any {
	return map[string]any{"id": s.ID, "nombre": s.Name, "descripcion": s.Description, "duracionMin": s.DurationMin, "precioCents": s.PriceCents}
}
func (h *Handler) services(w http.ResponseWriter, r *http.Request) {
	x, err := core(h).ListServices(r.Context(), h.Tenant.ID)
	if err != nil {
		bffErr(w, 502, err.Error())
		return
	}
	out := make([]map[string]any, len(x))
	for i, v := range x {
		out[i] = svcDTO(v)
	}
	writeJSON(w, 200, map[string]any{"servicios": out})
}
func bookingStatus(s string) string {
	m := map[string]string{"PENDING": "PENDIENTE", "CONFIRMED": "CONFIRMADA", "IN_PROGRESS": "EN_CURSO", "COMPLETED": "COMPLETADA", "CANCELLED": "CANCELADA", "NO_SHOW": "NO_ASISTIO"}
	if x := m[s]; x != "" {
		return x
	}
	return s
}
func paymentStatus(s string) string {
	m := map[string]string{"PENDING": "PENDIENTE", "REPORTED": "REPORTADO", "PAID": "VERIFICADO", "VOIDED": "RECHAZADO"}
	if x := m[s]; x != "" {
		return x
	}
	return s
}
func method(s string) string {
	m := map[string]string{"CASH": "EFECTIVO", "TRANSFER": "TRANSFERENCIA", "CARD": "TARJETA"}
	if x := m[s]; x != "" {
		return x
	}
	return s
}
func (h *Handler) citaDTO(ctx context.Context, b ports.Booking) map[string]any {
	s, _ := core(h).ListServices(ctx, h.Tenant.ID)
	var service any
	for _, x := range s {
		if x.ID == b.ServiceID {
			service = svcDTO(x)
		}
	}
	pays, _ := core(h).ListPaymentsByCustomer(ctx, h.Tenant.ID, b.CustomerID)
	var pay any
	for _, p := range pays {
		if p.BookingID != nil && *p.BookingID == b.ID {
			pay = map[string]any{"id": p.ID, "estado": paymentStatus(p.Status), "metodoPago": method(p.Method), "montoCents": p.AmountCents}
			break
		}
	}
	return map[string]any{"id": b.ID, "fechaHora": b.StartsAt, "estado": bookingStatus(b.Status), "etapaOperativa": b.OperativeStage, "notas": b.Notes, "servicio": service, "pago": pay}
}
func (h *Handler) citas(w http.ResponseWriter, r *http.Request) {
	x, err := core(h).ListBookingsByCustomer(r.Context(), h.Tenant.ID, customer(r).ID)
	if err != nil {
		bffErr(w, 502, err.Error())
		return
	}
	out := make([]map[string]any, len(x))
	for i, b := range x {
		out[i] = h.citaDTO(r.Context(), b)
	}
	writeJSON(w, 200, map[string]any{"citas": out})
}
func (h *Handler) createCita(w http.ResponseWriter, r *http.Request) {
	var b struct{ ServicioID, FechaHora, DisenoID, Notas string }
	if jsonBody(r, &b) != nil {
		bffErr(w, 400, "JSON inválido")
		return
	}
	sid, err := uuid.Parse(b.ServicioID)
	if err != nil {
		bffErr(w, 400, "servicioId inválido")
		return
	}
	at, err := time.Parse(time.RFC3339, b.FechaHora)
	if err != nil {
		bffErr(w, 400, "fechaHora inválida")
		return
	}
	var d *uuid.UUID
	if b.DisenoID != "" {
		i, e := uuid.Parse(b.DisenoID)
		if e != nil {
			bffErr(w, 400, "disenoId inválido")
			return
		}
		d = &i
	}
	v, err := core(h).CreateBooking(r.Context(), h.Tenant.ID, ports.CreateBookingInput{CustomerID: customer(r).ID, ServiceID: sid, StartsAt: at, Notes: b.Notas, DesignID: d})
	if err != nil {
		bffErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"cita": h.citaDTO(r.Context(), *v)})
}
func (h *Handler) cancelCita(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		bffErr(w, 400, "id inválido")
		return
	}
	b, err := core(h).CancelBookingCustomer(r.Context(), h.Tenant.ID, id)
	if err != nil {
		bffErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"cita": h.citaDTO(r.Context(), *b)})
}
func bankDTO(b ports.BankAccount) map[string]any {
	names := map[string]string{"PRODUBANCO": "Produbanco", "BANCO_DEL_PACIFICO": "Banco del Pacífico", "BANCO_DE_GUAYAQUIL": "Banco de Guayaquil"}
	return map[string]any{"id": b.ID, "banco": b.BankCode, "bancoNombre": names[b.BankCode], "nombreTitular": b.HolderName, "numeroCuenta": b.AccountNumber, "tipoCuenta": b.AccountType, "cedulaRuc": b.TaxID, "emailNotif": b.NotifyEmail}
}
func (h *Handler) accounts(w http.ResponseWriter, r *http.Request) {
	x, err := core(h).ListBankAccounts(r.Context(), h.Tenant.ID)
	if err != nil {
		bffErr(w, 502, err.Error())
		return
	}
	out := []map[string]any{}
	for _, v := range x {
		if v.Active {
			out = append(out, bankDTO(v))
		}
	}
	writeJSON(w, 200, map[string]any{"cuentas": out})
}
func paymentDTO(p ports.Payment) map[string]any {
	return map[string]any{"id": p.ID, "montoCents": p.AmountCents, "moneda": p.Currency, "referencia": p.Reference, "estado": paymentStatus(p.Status), "banco": p.BankCode, "metodoPago": method(p.Method), "comprobanteUrl": p.ReceiptURL, "createdAt": p.CreatedAt}
}
func (h *Handler) pagos(w http.ResponseWriter, r *http.Request) {
	x, err := core(h).ListPaymentsByCustomer(r.Context(), h.Tenant.ID, customer(r).ID)
	if err != nil {
		bffErr(w, 502, err.Error())
		return
	}
	out := make([]map[string]any, len(x))
	for i, v := range x {
		out[i] = paymentDTO(v)
	}
	writeJSON(w, 200, map[string]any{"pagos": out})
}
func saveUpload(r *http.Request, field, dir string) (string, error) {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return "", err
	}
	f, h, err := r.FormFile(field)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err = os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	name := uuid.NewString() + filepath.Ext(h.Filename)
	dst, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	defer dst.Close()
	_, err = io.Copy(dst, f)
	return name, err
}
func (h *Handler) reportPago(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		bffErr(w, 400, "Formulario inválido")
		return
	}
	met := r.FormValue("metodoPago")
	if met == "TARJETA" {
		bffErr(w, 400, "Pago con tarjeta estará disponible próximamente")
		return
	}
	if met != "EFECTIVO" && met != "TRANSFERENCIA" {
		bffErr(w, 400, "metodoPago inválido")
		return
	}
	amount := 0
	_, err := fmt.Sscan(r.FormValue("montoCents"), &amount)
	if err != nil || amount <= 0 {
		bffErr(w, 400, "montoCents inválido")
		return
	}
	var bid *uuid.UUID
	if raw := r.FormValue("citaId"); raw != "" {
		id, e := uuid.Parse(raw)
		if e != nil {
			bffErr(w, 400, "citaId inválido")
			return
		}
		bid = &id
	}
	in := ports.RegisterPaymentInput{CustomerID: customer(r).ID, BookingID: bid, Method: map[string]string{"EFECTIVO": "CASH", "TRANSFERENCIA": "TRANSFER"}[met], AmountCents: amount, Currency: "USD", Reference: r.FormValue("referencia")}
	if met == "TRANSFERENCIA" {
		raw := r.FormValue("cuentaBancariaId")
		id, e := uuid.Parse(raw)
		if e != nil {
			bffErr(w, 400, "Selecciona una cuenta bancaria")
			return
		}
		name, e := saveUpload(r, "comprobante", filepath.Join(h.UploadDir, "comprobantes"))
		if e != nil {
			writeJSON(w, 400, map[string]string{"error": "Debes adjuntar el comprobante de la transferencia", "code": "COMPROBANTE_REQUIRED"})
			return
		}
		in.BankAccountID = &id
		in.ReceiptURL = "/uploads/comprobantes/" + name
	}
	p, e := core(h).RegisterPayment(r.Context(), h.Tenant.ID, in)
	if e != nil {
		bffErr(w, 502, e.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"pago": paymentDTO(*p)})
}
