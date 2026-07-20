package public

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/ndresdavila/platform-api/internal/adapters/keycloak"
	"github.com/ndresdavila/platform-api/internal/domain/ports"
)

func designDTO(d ports.Design) map[string]any {
	return map[string]any{"id": d.ID, "prompt": d.Prompt, "fotoManosUrl": d.PhotoURL, "resultadoUrl": d.ResultURL, "estado": d.Status, "errorMensaje": d.ErrorMessage, "createdAt": d.CreatedAt}
}
func quotaDTO(q *ports.DesignQuota) map[string]any {
	remaining := q.Limit - q.Used
	if t := q.TotalMax - q.TotalUsed; t < remaining {
		remaining = t
	}
	if remaining < 0 {
		remaining = 0
	}
	return map[string]any{"used": q.Used, "limit": q.Limit, "remaining": remaining, "totalUsed": q.TotalUsed, "totalMax": q.TotalMax, "totalRemaining": max(0, q.TotalMax-q.TotalUsed)}
}
func (h *Handler) designEnabled(ctx context.Context) bool {
	s, e := core(h).GetSettings(ctx, h.Tenant.ID)
	return e == nil && s.FeatureDesignAI
}
func (h *Handler) designQuota(w http.ResponseWriter, r *http.Request) {
	if !h.designEnabled(r.Context()) {
		writeJSON(w, 403, map[string]string{"error": "El diseño con IA está desactivado", "code": "FEATURE_DISENO_IA_OFF"})
		return
	}
	q, e := core(h).GetDesignQuota(r.Context(), h.Tenant.ID, customer(r).ID)
	if e != nil {
		bffErr(w, 502, e.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"quota": quotaDTO(q)})
}
func (h *Handler) designs(w http.ResponseWriter, r *http.Request) {
	if !h.designEnabled(r.Context()) {
		writeJSON(w, 403, map[string]string{"error": "El diseño con IA está desactivado", "code": "FEATURE_DISENO_IA_OFF"})
		return
	}
	x, e := core(h).ListDesigns(r.Context(), h.Tenant.ID, customer(r).ID)
	if e != nil {
		bffErr(w, 502, e.Error())
		return
	}
	q, _ := core(h).GetDesignQuota(r.Context(), h.Tenant.ID, customer(r).ID)
	out := make([]map[string]any, len(x))
	for i, v := range x {
		out[i] = designDTO(v)
	}
	writeJSON(w, 200, map[string]any{"disenos": out, "quota": quotaDTO(q)})
}
func (h *Handler) createDesign(w http.ResponseWriter, r *http.Request) {
	if !h.designEnabled(r.Context()) {
		writeJSON(w, 403, map[string]string{"error": "El diseño con IA está desactivado", "code": "FEATURE_DISENO_IA_OFF"})
		return
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		bffErr(w, 400, "Formulario inválido")
		return
	}
	prompt := r.FormValue("prompt")
	if len(prompt) < 5 {
		bffErr(w, 400, "prompt inválido")
		return
	}
	q, e := core(h).GetDesignQuota(r.Context(), h.Tenant.ID, customer(r).ID)
	if e != nil {
		bffErr(w, 502, e.Error())
		return
	}
	if q.Used >= q.Limit || q.TotalUsed >= q.TotalMax {
		writeJSON(w, 429, map[string]any{"error": "Llegaste al límite de diseños IA", "quota": quotaDTO(q)})
		return
	}
	name, e := saveUpload(r, "foto", filepath.Join(h.UploadDir, "manos"))
	if e != nil {
		bffErr(w, 400, "Se requiere foto de las manos")
		return
	}
	photo := "/uploads/manos/" + name
	d, e := core(h).CreateDesign(r.Context(), h.Tenant.ID, ports.CreateDesignInput{CustomerID: customer(r).ID, Prompt: prompt, PhotoURL: photo, ModelUsed: "mock"})
	if e != nil {
		bffErr(w, 502, e.Error())
		return
	}
	go h.completeMockDesign(*d, filepath.Join(h.UploadDir, "manos", name))
	after, _ := core(h).GetDesignQuota(r.Context(), h.Tenant.ID, customer(r).ID)
	writeJSON(w, 201, map[string]any{"diseno": designDTO(*d), "quota": quotaDTO(after)})
}
func (h *Handler) completeMockDesign(d ports.Design, source string) {
	dir := filepath.Join(h.UploadDir, "resultados")
	_ = os.MkdirAll(dir, 0755)
	name := uuid.NewString() + filepath.Ext(source)
	dest := filepath.Join(dir, name)
	in := ports.CompleteDesignInput{ResultURL: "/uploads/resultados/" + name}
	if src, e := os.Open(source); e != nil {
		in.Failed = true
		in.ErrorMessage = e.Error()
	} else {
		out, e := os.Create(dest)
		if e != nil {
			in.Failed = true
			in.ErrorMessage = e.Error()
		} else {
			_, e = io.Copy(out, src)
			_ = out.Close()
			_ = src.Close()
			if e != nil {
				in.Failed = true
				in.ErrorMessage = e.Error()
			}
		}
	}
	_, _ = core(h).CompleteDesign(context.Background(), h.Tenant.ID, d.ID, in)
}
func (h *Handler) deleteDesign(w http.ResponseWriter, r *http.Request) {
	id, e := uuid.Parse(chi.URLParam(r, "id"))
	if e != nil {
		bffErr(w, 400, "id inválido")
		return
	}
	d, e := core(h).ListDesigns(r.Context(), h.Tenant.ID, customer(r).ID)
	if e != nil {
		bffErr(w, 502, e.Error())
		return
	}
	found := false
	for _, v := range d {
		found = found || v.ID == id
	}
	if !found {
		bffErr(w, 404, "Diseño no encontrado")
		return
	}
	if e = core(h).DeleteDesign(r.Context(), h.Tenant.ID, id); e != nil {
		bffErr(w, 502, e.Error())
		return
	}
	q, _ := core(h).GetDesignQuota(r.Context(), h.Tenant.ID, customer(r).ID)
	writeJSON(w, 200, map[string]any{"ok": true, "quota": quotaDTO(q)})
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
		employee, e := core(h).UpsertEmployee(r.Context(), h.Tenant.ID, ports.UpsertEmployeeInput{ExternalID: p.Subject, Email: email, FirstName: first, LastName: last, Role: keycloak.StaffRole(p)})
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
func (h *Handler) staffCitaDTO(ctx context.Context, b ports.Booking) map[string]any {
	v := h.citaDTO(ctx, b)
	c, _ := core(h).GetCustomer(ctx, h.Tenant.ID, b.CustomerID)
	if c != nil {
		v["usuario"] = map[string]any{"id": c.ID, "nombre": c.FirstName, "apellido": c.LastName, "email": c.Email, "telefono": c.Phone, "cedula": c.NationalID}
	}
	pays, _ := core(h).ListPaymentsByCustomer(ctx, h.Tenant.ID, b.CustomerID)
	all := []map[string]any{}
	for _, p := range pays {
		if p.BookingID != nil && *p.BookingID == b.ID {
			all = append(all, paymentDTO(p))
		}
	}
	v["pagos"] = all
	if len(all) > 0 {
		v["pagoResumen"] = map[string]any{"id": all[0]["id"], "estado": all[0]["estado"], "montoCents": all[0]["montoCents"], "tieneComprobante": all[0]["comprobanteUrl"] != ""}
	} else {
		v["pagoResumen"] = nil
	}
	return v
}
func (h *Handler) staffCitas(w http.ResponseWriter, r *http.Request) {
	from, e := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, e2 := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	if e != nil || e2 != nil {
		bffErr(w, 400, "Parámetros from y to (ISO) requeridos")
		return
	}
	x, e := core(h).ListBookingsByRange(r.Context(), h.Tenant.ID, from, to)
	if e != nil {
		bffErr(w, 502, e.Error())
		return
	}
	out := make([]map[string]any, len(x))
	for i, v := range x {
		out[i] = h.staffCitaDTO(r.Context(), v)
	}
	writeJSON(w, 200, map[string]any{"citas": out})
}
func (h *Handler) staffCita(w http.ResponseWriter, r *http.Request) {
	id, e := uuid.Parse(chi.URLParam(r, "id"))
	if e != nil {
		bffErr(w, 400, "id inválido")
		return
	}
	b, e := core(h).GetBooking(r.Context(), h.Tenant.ID, id)
	if e != nil {
		bffErr(w, 404, "Cita no encontrada")
		return
	}
	writeJSON(w, 200, map[string]any{"cita": h.staffCitaDTO(r.Context(), *b)})
}
func (h *Handler) advance(w http.ResponseWriter, r *http.Request) {
	id, e := uuid.Parse(chi.URLParam(r, "id"))
	if e != nil {
		bffErr(w, 400, "id inválido")
		return
	}
	var body struct {
		A string `json:"a"`
	}
	_ = jsonBody(r, &body)
	var to *string
	if body.A != "" {
		to = &body.A
	}
	b, e := core(h).AdvanceBooking(r.Context(), h.Tenant.ID, id, to)
	if e != nil {
		bffErr(w, 400, e.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"cita": h.staffCitaDTO(r.Context(), *b)})
}
func (h *Handler) staffCancel(w http.ResponseWriter, r *http.Request) {
	id, e := uuid.Parse(chi.URLParam(r, "id"))
	if e != nil {
		bffErr(w, 400, "id inválido")
		return
	}
	b, e := core(h).CancelBookingStaff(r.Context(), h.Tenant.ID, id)
	if e != nil {
		bffErr(w, 400, e.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"cita": h.staffCitaDTO(r.Context(), *b)})
}
func (h *Handler) verifyPayment(w http.ResponseWriter, r *http.Request) {
	id, e := uuid.Parse(chi.URLParam(r, "id"))
	if e != nil {
		bffErr(w, 400, "id inválido")
		return
	}
	p, e := core(h).MarkPaymentPaid(r.Context(), h.Tenant.ID, id)
	if e != nil {
		bffErr(w, 400, e.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"pago": paymentDTO(*p), "cita": nil})
}
func (h *Handler) voidPayment(w http.ResponseWriter, r *http.Request) {
	id, e := uuid.Parse(chi.URLParam(r, "id"))
	if e != nil {
		bffErr(w, 400, "id inválido")
		return
	}
	p, e := core(h).VoidPayment(r.Context(), h.Tenant.ID, id)
	if e != nil {
		bffErr(w, 400, e.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"pago": paymentDTO(*p), "cita": nil})
}
func (h *Handler) receipt(w http.ResponseWriter, r *http.Request) {
	id, e := uuid.Parse(chi.URLParam(r, "id"))
	if e != nil {
		bffErr(w, 400, "id inválido")
		return
	}
	p, e := core(h).GetPayment(r.Context(), h.Tenant.ID, id)
	if e != nil || p.ReceiptURL == "" {
		bffErr(w, 404, "Comprobante no encontrado")
		return
	}
	relative := strings.TrimPrefix(filepath.Clean(p.ReceiptURL), "/uploads/")
	path := filepath.Join(h.UploadDir, relative)
	if !strings.HasPrefix(path, filepath.Clean(h.UploadDir)+string(os.PathSeparator)) {
		bffErr(w, 404, "Comprobante no encontrado")
		return
	}
	http.ServeFile(w, r, path)
}
