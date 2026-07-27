package public

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/ndresdavila/platform-api/internal/adapters/keycloak"
	"github.com/ndresdavila/platform-api/internal/domain/ports"
)

type contextKey string

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

// withCORS lets the staff Next.js app (and other browser clients) call this API.
// Reflects Origin so localhost:3002 and LAN IPs both work in local/dev.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func newBFFRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(withCORS, chimw.RequestID, chimw.RealIP, chimw.Logger, chimw.Recoverer)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := core(h).Health(r.Context()); err != nil {
			writeJSON(w, 503, map[string]any{"ok": false, "core": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	r.Route("/api/staff", func(r chi.Router) {
		r.Use(h.requireStaff)
		r.Get("/me", h.staffMe)
		r.Get("/personas", h.listPersonas)
		r.Post("/personas", h.createPersona)
		r.Get("/productos", h.listProductos)
		r.Post("/productos", h.createProducto)
		r.Get("/documentos", h.listDocumentos)
		r.Get("/documentos/{id}", h.getDocumento)
		r.Post("/documentos", h.createDocumento)
		r.Post("/documentos/{id}/enviar-sri", h.sendDocumentoSRI)
	})
	return r
}

func (h *Handler) requireStaff(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, e := keycloak.VerifyStaffAccessToken(r.Context(), bearer(r))
		if e != nil {
			bffErr(w, 401, "Token de empleado inválido")
			return
		}
		if !keycloak.HasStaffAccess(p) {
			bffErr(w, 403, "Sin permisos de empleado")
			return
		}
		email, first, last := keycloak.Profile(p)
		employee, e := core(h).UpsertEmployee(r.Context(), h.Tenant.ID, ports.UpsertEmployeeInput{
			ExternalID: p.Subject, Email: email, FirstName: first, LastName: last, Role: keycloak.StaffRole(p),
		})
		if e != nil {
			bffErr(w, 502, e.Error())
			return
		}
		if !employee.Active {
			bffErr(w, 403, "Cuenta de empleado desactivada")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), staffKey, employee)))
	})
}

func staff(r *http.Request) *ports.Employee {
	v, _ := r.Context().Value(staffKey).(*ports.Employee)
	return v
}

func employeeDTO(e *ports.Employee) map[string]any {
	return map[string]any{"id": e.ID, "keycloakId": e.ExternalID, "email": e.Email, "nombre": e.FirstName, "apellido": e.LastName, "rolEmpleado": e.Role}
}

func (h *Handler) staffMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"empleado": employeeDTO(staff(r))})
}

func money(cents int) float64 { return float64(cents) / 100.0 }

func documentoDTO(d ports.ElectronicDocument) map[string]any {
	lines := make([]map[string]any, 0, len(d.Lines))
	for _, l := range d.Lines {
		lines = append(lines, map[string]any{
			"id": l.ID, "lineNo": l.LineNo, "productId": l.ProductID, "producto": l.ProductName,
			"unidad": l.Unit, "cantidad": l.Quantity, "precioUnitario": money(l.UnitPriceCents),
			"iva": l.IVARate, "descuentoPct": l.DiscountPercent, "descuento": money(l.DiscountCents),
			"subtotal": money(l.SubtotalCents),
		})
	}
	return map[string]any{
		"id": d.ID, "tipo": d.DocType, "tipoPersona": d.PartyKind, "personaId": d.PersonID,
		"persona": d.PersonName, "identificacion": d.PersonIdentification,
		"establecimiento": d.Establishment, "puntoEmision": d.EmissionPoint,
		"numero": d.DocumentNumber, "claveAcceso": d.AccessKey,
		"fechaEmision": d.IssueDate.Format("02/01/2006"), "vencimientoDias": d.DueDays,
		"referencia": d.Reference, "vendedor": d.Seller, "descripcion": d.Description,
		"exportacion": d.IsExport, "estado": d.Status, "mensajeSri": d.SRIMessage,
		"subtotal15": money(d.Subtotal15Cents), "subtotal5": money(d.Subtotal5Cents),
		"subtotal0": money(d.Subtotal0Cents), "descuento": money(d.DiscountCents),
		"iva15": money(d.IVA15Cents), "iva5": money(d.IVA5Cents), "ice": money(d.ICECents),
		"total": money(d.TotalCents), "lineas": lines,
	}
}

func (h *Handler) listPersonas(w http.ResponseWriter, r *http.Request) {
	list, err := core(h).ListPersons(r.Context(), h.Tenant.ID, r.URL.Query().Get("q"))
	if err != nil {
		bffErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"personas": list})
}

func (h *Handler) createPersona(w http.ResponseWriter, r *http.Request) {
	var body ports.CreatePersonInput
	if err := jsonBody(r, &body); err != nil {
		bffErr(w, 400, "JSON inválido")
		return
	}
	p, err := core(h).CreatePerson(r.Context(), h.Tenant.ID, body)
	if err != nil {
		bffErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"persona": p})
}

func (h *Handler) listProductos(w http.ResponseWriter, r *http.Request) {
	list, err := core(h).ListProducts(r.Context(), h.Tenant.ID, r.URL.Query().Get("q"))
	if err != nil {
		bffErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"productos": list})
}

func (h *Handler) createProducto(w http.ResponseWriter, r *http.Request) {
	var body ports.CreateProductInput
	if err := jsonBody(r, &body); err != nil {
		bffErr(w, 400, "JSON inválido")
		return
	}
	p, err := core(h).CreateProduct(r.Context(), h.Tenant.ID, body)
	if err != nil {
		bffErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"producto": p})
}

func (h *Handler) listDocumentos(w http.ResponseWriter, r *http.Request) {
	list, err := core(h).ListInvoices(r.Context(), h.Tenant.ID)
	if err != nil {
		bffErr(w, 502, err.Error())
		return
	}
	out := make([]map[string]any, len(list))
	for i, d := range list {
		out[i] = documentoDTO(d)
	}
	writeJSON(w, 200, map[string]any{"documentos": out})
}

func (h *Handler) getDocumento(w http.ResponseWriter, r *http.Request) {
	id, e := uuid.Parse(chi.URLParam(r, "id"))
	if e != nil {
		bffErr(w, 400, "id inválido")
		return
	}
	d, err := core(h).GetInvoice(r.Context(), h.Tenant.ID, id)
	if err != nil {
		bffErr(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"documento": documentoDTO(*d)})
}

func (h *Handler) createDocumento(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Tipo            string                   `json:"tipo"`
		TipoPersona     string                   `json:"tipoPersona"`
		PersonaID       *uuid.UUID               `json:"personaId"`
		Persona         string                   `json:"persona"`
		Identificacion  string                   `json:"identificacion"`
		Establecimiento string                   `json:"establecimiento"`
		PuntoEmision    string                   `json:"puntoEmision"`
		FechaEmision    string                   `json:"fechaEmision"`
		VencimientoDias int                      `json:"vencimientoDias"`
		Referencia      string                   `json:"referencia"`
		Vendedor        string                   `json:"vendedor"`
		Descripcion     string                   `json:"descripcion"`
		Exportacion     bool                     `json:"exportacion"`
		EnviarSRI       bool                     `json:"enviarSri"`
		Lineas          []ports.InvoiceLineInput `json:"lineas"`
	}
	if err := jsonBody(r, &body); err != nil {
		bffErr(w, 400, "JSON inválido")
		return
	}
	emp := staff(r)
	d, err := core(h).CreateInvoice(r.Context(), h.Tenant.ID, ports.CreateInvoiceInput{
		DocType: body.Tipo, PartyKind: body.TipoPersona, PersonID: body.PersonaID,
		PersonName: body.Persona, PersonIdentification: body.Identificacion,
		Establishment: body.Establecimiento, EmissionPoint: body.PuntoEmision,
		IssueDate: body.FechaEmision, DueDays: body.VencimientoDias,
		Reference: body.Referencia, Seller: body.Vendedor, Description: body.Descripcion,
		IsExport: body.Exportacion, SendToSRI: body.EnviarSRI,
		CreatedByExternalID: emp.ExternalID, Lines: body.Lineas,
	})
	if err != nil {
		bffErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"documento": documentoDTO(*d)})
}

func (h *Handler) sendDocumentoSRI(w http.ResponseWriter, r *http.Request) {
	id, e := uuid.Parse(chi.URLParam(r, "id"))
	if e != nil {
		bffErr(w, 400, "id inválido")
		return
	}
	d, err := core(h).SendInvoiceSRI(r.Context(), h.Tenant.ID, id)
	if err != nil {
		bffErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"documento": documentoDTO(*d)})
}
