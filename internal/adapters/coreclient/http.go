package coreclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/ndresdavila/platform-api/internal/domain/ports"
)

type HTTPClient struct {
	base   string
	client *http.Client
}

func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		base: stringsTrimRightSlash(baseURL),
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func stringsTrimRightSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func (c *HTTPClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/healthz", nil)
	if err != nil {
		return err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("core unhealthy: %s", res.Status)
	}
	return nil
}

func (c *HTTPClient) CreateTenant(ctx context.Context, slug, name string) (*ports.Tenant, error) {
	var out ports.Tenant
	if err := c.post(ctx, "/v1/tenants", map[string]string{"slug": slug, "name": name}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *HTTPClient) GetTenantBySlug(ctx context.Context, slug string) (*ports.Tenant, error) {
	var out ports.Tenant
	if err := c.get(ctx, "/v1/tenants/by-slug/"+slug, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
func (c *HTTPClient) tenantPath(id uuid.UUID, suffix string) string {
	return fmt.Sprintf("/v1/tenants/%s%s", id, suffix)
}
func (c *HTTPClient) GetSettings(ctx context.Context, id uuid.UUID) (*ports.TenantSettings, error) {
	var v ports.TenantSettings
	return &v, c.get(ctx, c.tenantPath(id, "/settings"), &v)
}
func (c *HTTPClient) PutSettings(ctx context.Context, id uuid.UUID, settings ports.TenantSettings) (*ports.TenantSettings, error) {
	var v ports.TenantSettings
	return &v, c.method(ctx, http.MethodPut, c.tenantPath(id, "/settings"), settings, &v)
}
func (c *HTTPClient) UpsertCustomer(ctx context.Context, id uuid.UUID, in ports.UpsertCustomerInput) (*ports.Customer, error) {
	var v ports.Customer
	return &v, c.post(ctx, c.tenantPath(id, "/customers/upsert"), map[string]any{"externalId": in.ExternalID, "email": in.Email, "firstName": in.FirstName, "lastName": in.LastName, "phone": in.Phone, "nationalId": in.NationalID, "role": in.Role}, &v)
}
func (c *HTTPClient) GetCustomerByExternal(ctx context.Context, id uuid.UUID, external string) (*ports.Customer, error) {
	var v ports.Customer
	return &v, c.get(ctx, c.tenantPath(id, "/customers/by-external/"+url.PathEscape(external)), &v)
}
func (c *HTTPClient) GetCustomer(ctx context.Context, tenantID, customerID uuid.UUID) (*ports.Customer, error) {
	var v ports.Customer
	return &v, c.get(ctx, c.tenantPath(tenantID, "/customers/"+customerID.String()), &v)
}
func (c *HTTPClient) UpdateCustomerProfile(ctx context.Context, t, id uuid.UUID, in ports.CustomerProfileInput) (*ports.Customer, error) {
	var v ports.Customer
	return &v, c.patch(ctx, c.tenantPath(t, "/customers/"+id.String()+"/profile"), map[string]any{"nationalId": in.NationalID, "firstName": in.FirstName, "lastName": in.LastName, "phone": in.Phone}, &v)
}
func (c *HTTPClient) UpsertEmployee(ctx context.Context, id uuid.UUID, in ports.UpsertEmployeeInput) (*ports.Employee, error) {
	var v ports.Employee
	return &v, c.post(ctx, c.tenantPath(id, "/employees/upsert"), map[string]any{"externalId": in.ExternalID, "email": in.Email, "firstName": in.FirstName, "lastName": in.LastName, "role": in.Role}, &v)
}

func (c *HTTPClient) ListServices(ctx context.Context, tenantID uuid.UUID) ([]ports.CatalogService, error) {
	var wrap struct {
		Services []ports.CatalogService `json:"services"`
	}
	if err := c.get(ctx, fmt.Sprintf("/v1/tenants/%s/catalog/services", tenantID), &wrap); err != nil {
		return nil, err
	}
	return wrap.Services, nil
}
func (c *HTTPClient) CreateService(ctx context.Context, tenantID uuid.UUID, service ports.CatalogService) (*ports.CatalogService, error) {
	var v ports.CatalogService
	return &v, c.post(ctx, c.tenantPath(tenantID, "/catalog/services"), service, &v)
}

func (c *HTTPClient) CreateBooking(ctx context.Context, tenantID uuid.UUID, in ports.CreateBookingInput) (*ports.Booking, error) {
	var out ports.Booking
	body := map[string]any{
		"customerId": in.CustomerID.String(),
		"serviceId":  in.ServiceID.String(),
		"startsAt":   in.StartsAt,
		"notes":      in.Notes,
	}
	if in.DesignID != nil {
		body["designId"] = in.DesignID.String()
	}
	if err := c.post(ctx, fmt.Sprintf("/v1/tenants/%s/bookings", tenantID), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *HTTPClient) ListBookingsByCustomer(ctx context.Context, tenantID, customerID uuid.UUID) ([]ports.Booking, error) {
	var wrap struct {
		Bookings []ports.Booking `json:"bookings"`
	}
	path := fmt.Sprintf("/v1/tenants/%s/bookings?customerId=%s", tenantID, customerID)
	if err := c.get(ctx, path, &wrap); err != nil {
		return nil, err
	}
	return wrap.Bookings, nil
}

func (c *HTTPClient) CancelBooking(ctx context.Context, tenantID, bookingID uuid.UUID) (*ports.Booking, error) {
	return c.CancelBookingCustomer(ctx, tenantID, bookingID)
}
func (c *HTTPClient) GetBooking(ctx context.Context, t, b uuid.UUID) (*ports.Booking, error) {
	var v ports.Booking
	return &v, c.get(ctx, c.tenantPath(t, "/bookings/"+b.String()), &v)
}
func (c *HTTPClient) ListBookingsByRange(ctx context.Context, t uuid.UUID, from, to time.Time) ([]ports.Booking, error) {
	var v struct {
		Bookings []ports.Booking `json:"bookings"`
	}
	p := c.tenantPath(t, "/bookings?from="+url.QueryEscape(from.Format(time.RFC3339))+"&to="+url.QueryEscape(to.Format(time.RFC3339)))
	if err := c.get(ctx, p, &v); err != nil {
		return nil, err
	}
	return v.Bookings, nil
}
func (c *HTTPClient) CancelBookingCustomer(ctx context.Context, tenantID, bookingID uuid.UUID) (*ports.Booking, error) {
	var out ports.Booking
	path := fmt.Sprintf("/v1/tenants/%s/bookings/%s/cancel-customer", tenantID, bookingID)
	if err := c.post(ctx, path, map[string]any{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
func (c *HTTPClient) CancelBookingStaff(ctx context.Context, t, b uuid.UUID) (*ports.Booking, error) {
	var v ports.Booking
	return &v, c.post(ctx, c.tenantPath(t, "/bookings/"+b.String()+"/cancel-staff"), map[string]any{}, &v)
}
func (c *HTTPClient) AdvanceBooking(ctx context.Context, t, b uuid.UUID, stage *string) (*ports.Booking, error) {
	var v ports.Booking
	return &v, c.post(ctx, c.tenantPath(t, "/bookings/"+b.String()+"/advance"), map[string]any{"to": stage}, &v)
}

func (c *HTTPClient) RegisterPayment(ctx context.Context, tenantID uuid.UUID, in ports.RegisterPaymentInput) (*ports.Payment, error) {
	var out ports.Payment
	body := map[string]any{
		"customerId":  in.CustomerID.String(),
		"method":      in.Method,
		"amountCents": in.AmountCents,
		"currency":    in.Currency,
		"reference":   in.Reference,
		"receiptUrl":  in.ReceiptURL,
	}
	if in.BookingID != nil {
		body["bookingId"] = in.BookingID.String()
	}
	if in.BankAccountID != nil {
		body["bankAccountId"] = in.BankAccountID.String()
	}
	body["bankCode"] = in.BankCode
	if err := c.post(ctx, fmt.Sprintf("/v1/tenants/%s/payments", tenantID), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
func (c *HTTPClient) GetPayment(ctx context.Context, t, p uuid.UUID) (*ports.Payment, error) {
	var v ports.Payment
	return &v, c.get(ctx, c.tenantPath(t, "/payments/"+p.String()), &v)
}
func (c *HTTPClient) MarkPaymentPaid(ctx context.Context, t, p uuid.UUID) (*ports.Payment, error) {
	var v ports.Payment
	return &v, c.post(ctx, c.tenantPath(t, "/payments/"+p.String()+"/mark-paid"), map[string]any{}, &v)
}
func (c *HTTPClient) VoidPayment(ctx context.Context, t, p uuid.UUID) (*ports.Payment, error) {
	var v ports.Payment
	return &v, c.post(ctx, c.tenantPath(t, "/payments/"+p.String()+"/void"), map[string]any{}, &v)
}
func (c *HTTPClient) ListBankAccounts(ctx context.Context, t uuid.UUID) ([]ports.BankAccount, error) {
	var v struct {
		BankAccounts []ports.BankAccount `json:"bankAccounts"`
	}
	if err := c.get(ctx, c.tenantPath(t, "/bank-accounts"), &v); err != nil {
		return nil, err
	}
	return v.BankAccounts, nil
}
func (c *HTTPClient) CreateBankAccount(ctx context.Context, t uuid.UUID, account ports.BankAccount) (*ports.BankAccount, error) {
	var v ports.BankAccount
	return &v, c.post(ctx, c.tenantPath(t, "/bank-accounts"), account, &v)
}
func (c *HTTPClient) CreateDesign(ctx context.Context, t uuid.UUID, in ports.CreateDesignInput) (*ports.Design, error) {
	var v ports.Design
	return &v, c.post(ctx, c.tenantPath(t, "/designs"), map[string]any{"customerId": in.CustomerID.String(), "prompt": in.Prompt, "photoUrl": in.PhotoURL, "modelUsed": in.ModelUsed}, &v)
}
func (c *HTTPClient) ListDesigns(ctx context.Context, t, u uuid.UUID) ([]ports.Design, error) {
	var v struct {
		Designs []ports.Design `json:"designs"`
	}
	if err := c.get(ctx, c.tenantPath(t, "/designs?customerId="+u.String()), &v); err != nil {
		return nil, err
	}
	return v.Designs, nil
}
func (c *HTTPClient) GetDesign(ctx context.Context, t, d uuid.UUID) (*ports.Design, error) {
	var v ports.Design
	return &v, c.get(ctx, c.tenantPath(t, "/designs/"+d.String()), &v)
}
func (c *HTTPClient) GetDesignQuota(ctx context.Context, t, u uuid.UUID) (*ports.DesignQuota, error) {
	var v ports.DesignQuota
	return &v, c.get(ctx, c.tenantPath(t, "/designs/quota?customerId="+u.String()), &v)
}
func (c *HTTPClient) CompleteDesign(ctx context.Context, t, d uuid.UUID, in ports.CompleteDesignInput) (*ports.Design, error) {
	var v ports.Design
	return &v, c.post(ctx, c.tenantPath(t, "/designs/"+d.String()+"/complete"), map[string]any{"resultUrl": in.ResultURL, "failed": in.Failed, "errorMessage": in.ErrorMessage}, &v)
}
func (c *HTTPClient) DeleteDesign(ctx context.Context, t, d uuid.UUID) error {
	return c.del(ctx, c.tenantPath(t, "/designs/"+d.String()))
}

func (c *HTTPClient) ListPaymentsByCustomer(ctx context.Context, tenantID, customerID uuid.UUID) ([]ports.Payment, error) {
	var wrap struct {
		Payments []ports.Payment `json:"payments"`
	}
	path := fmt.Sprintf("/v1/tenants/%s/payments?customerId=%s", tenantID, customerID)
	if err := c.get(ctx, path, &wrap); err != nil {
		return nil, err
	}
	return wrap.Payments, nil
}

func (c *HTTPClient) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *HTTPClient) post(ctx context.Context, path string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}
func (c *HTTPClient) patch(ctx context.Context, path string, body, out any) error {
	return c.method(ctx, http.MethodPatch, path, body, out)
}
func (c *HTTPClient) del(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.base+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}
func (c *HTTPClient) method(ctx context.Context, method, path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *HTTPClient) do(req *http.Request, out any) error {
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("core %s: %s", res.Status, string(raw))
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}
