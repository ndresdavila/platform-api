package public

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/ndresdavila/platform-api/internal/adapters/http/middleware"
	"github.com/ndresdavila/platform-api/internal/application"
	"github.com/ndresdavila/platform-api/internal/domain/ports"
)

type Handler struct {
	Facade    *application.Facade
	Tenant    *ports.Tenant
	UploadDir string
}

func NewRouter(h *Handler) http.Handler {
	if h.Tenant != nil {
		return newBFFRouter(h)
	}
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Logger, chimw.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := h.Facade.Core.Health(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "core": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.ResolveTenant(h.Facade))

		r.Get("/catalog/services", h.listServices)
		r.Post("/bookings", h.createBooking)
		r.Get("/bookings", h.listBookings)
		r.Post("/bookings/{bookingId}/cancel", h.cancelBooking)
		r.Post("/payments", h.registerPayment)
		r.Get("/payments", h.listPayments)
	})

	return r
}

func (h *Handler) listServices(w http.ResponseWriter, r *http.Request) {
	tenant, _ := middleware.TenantFromContext(r.Context())
	services, err := h.Facade.ListCatalog(r.Context(), tenant.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"services": services})
}

func (h *Handler) createBooking(w http.ResponseWriter, r *http.Request) {
	tenant, _ := middleware.TenantFromContext(r.Context())
	var body struct {
		CustomerID string    `json:"customerId"`
		ServiceID  string    `json:"serviceId"`
		StartsAt   time.Time `json:"startsAt"`
		Notes      string    `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	customerID, err := uuid.Parse(body.CustomerID)
	if err != nil {
		http.Error(w, `{"error":"invalid customerId"}`, http.StatusBadRequest)
		return
	}
	serviceID, err := uuid.Parse(body.ServiceID)
	if err != nil {
		http.Error(w, `{"error":"invalid serviceId"}`, http.StatusBadRequest)
		return
	}
	b, err := h.Facade.CreateBooking(r.Context(), tenant.ID, ports.CreateBookingInput{
		CustomerID: customerID,
		ServiceID:  serviceID,
		StartsAt:   body.StartsAt,
		Notes:      body.Notes,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (h *Handler) listBookings(w http.ResponseWriter, r *http.Request) {
	tenant, _ := middleware.TenantFromContext(r.Context())
	customerID, err := uuid.Parse(r.URL.Query().Get("customerId"))
	if err != nil {
		http.Error(w, `{"error":"customerId required"}`, http.StatusBadRequest)
		return
	}
	list, err := h.Facade.ListBookings(r.Context(), tenant.ID, customerID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookings": list})
}

func (h *Handler) cancelBooking(w http.ResponseWriter, r *http.Request) {
	tenant, _ := middleware.TenantFromContext(r.Context())
	bookingID, err := uuid.Parse(chi.URLParam(r, "bookingId"))
	if err != nil {
		http.Error(w, `{"error":"invalid bookingId"}`, http.StatusBadRequest)
		return
	}
	b, err := h.Facade.CancelBooking(r.Context(), tenant.ID, bookingID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (h *Handler) registerPayment(w http.ResponseWriter, r *http.Request) {
	tenant, _ := middleware.TenantFromContext(r.Context())
	var body struct {
		CustomerID  string  `json:"customerId"`
		BookingID   *string `json:"bookingId"`
		Method      string  `json:"method"`
		AmountCents int     `json:"amountCents"`
		Currency    string  `json:"currency"`
		Reference   string  `json:"reference"`
		ReceiptURL  string  `json:"receiptUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	customerID, err := uuid.Parse(body.CustomerID)
	if err != nil {
		http.Error(w, `{"error":"invalid customerId"}`, http.StatusBadRequest)
		return
	}
	in := ports.RegisterPaymentInput{
		CustomerID:  customerID,
		Method:      body.Method,
		AmountCents: body.AmountCents,
		Currency:    body.Currency,
		Reference:   body.Reference,
		ReceiptURL:  body.ReceiptURL,
	}
	if body.BookingID != nil && *body.BookingID != "" {
		id, err := uuid.Parse(*body.BookingID)
		if err != nil {
			http.Error(w, `{"error":"invalid bookingId"}`, http.StatusBadRequest)
			return
		}
		in.BookingID = &id
	}
	p, err := h.Facade.RegisterPayment(r.Context(), tenant.ID, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handler) listPayments(w http.ResponseWriter, r *http.Request) {
	tenant, _ := middleware.TenantFromContext(r.Context())
	customerID, err := uuid.Parse(r.URL.Query().Get("customerId"))
	if err != nil {
		http.Error(w, `{"error":"customerId required"}`, http.StatusBadRequest)
		return
	}
	list, err := h.Facade.ListPayments(r.Context(), tenant.ID, customerID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"payments": list})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
}
