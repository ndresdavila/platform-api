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

func (c *HTTPClient) UpsertEmployee(ctx context.Context, id uuid.UUID, in ports.UpsertEmployeeInput) (*ports.Employee, error) {
	var v ports.Employee
	return &v, c.post(ctx, c.tenantPath(id, "/employees/upsert"), map[string]any{
		"externalId": in.ExternalID, "email": in.Email, "firstName": in.FirstName, "lastName": in.LastName, "role": in.Role,
	}, &v)
}

func (c *HTTPClient) ListPersons(ctx context.Context, tenantID uuid.UUID, q string) ([]ports.Person, error) {
	var out []ports.Person
	path := c.tenantPath(tenantID, "/persons")
	if q != "" {
		path += "?q=" + url.QueryEscape(q)
	}
	return out, c.get(ctx, path, &out)
}

func (c *HTTPClient) CreatePerson(ctx context.Context, tenantID uuid.UUID, in ports.CreatePersonInput) (*ports.Person, error) {
	var v ports.Person
	return &v, c.post(ctx, c.tenantPath(tenantID, "/persons"), in, &v)
}

func (c *HTTPClient) ListProducts(ctx context.Context, tenantID uuid.UUID, q string) ([]ports.Product, error) {
	var out []ports.Product
	path := c.tenantPath(tenantID, "/products")
	if q != "" {
		path += "?q=" + url.QueryEscape(q)
	}
	return out, c.get(ctx, path, &out)
}

func (c *HTTPClient) CreateProduct(ctx context.Context, tenantID uuid.UUID, in ports.CreateProductInput) (*ports.Product, error) {
	var v ports.Product
	return &v, c.post(ctx, c.tenantPath(tenantID, "/products"), in, &v)
}

func (c *HTTPClient) ListInvoices(ctx context.Context, tenantID uuid.UUID) ([]ports.ElectronicDocument, error) {
	var out []ports.ElectronicDocument
	return out, c.get(ctx, c.tenantPath(tenantID, "/invoices"), &out)
}

func (c *HTTPClient) GetInvoice(ctx context.Context, tenantID, invoiceID uuid.UUID) (*ports.ElectronicDocument, error) {
	var v ports.ElectronicDocument
	return &v, c.get(ctx, c.tenantPath(tenantID, "/invoices/"+invoiceID.String()), &v)
}

func (c *HTTPClient) CreateInvoice(ctx context.Context, tenantID uuid.UUID, in ports.CreateInvoiceInput) (*ports.ElectronicDocument, error) {
	var v ports.ElectronicDocument
	body := map[string]any{
		"docType": in.DocType, "partyKind": in.PartyKind, "personId": in.PersonID,
		"personName": in.PersonName, "personIdentification": in.PersonIdentification,
		"establishment": in.Establishment, "emissionPoint": in.EmissionPoint, "issueDate": in.IssueDate,
		"dueDays": in.DueDays, "reference": in.Reference, "seller": in.Seller, "description": in.Description,
		"isExport": in.IsExport, "sendToSri": in.SendToSRI, "createdByExternalId": in.CreatedByExternalID,
		"lines": in.Lines,
	}
	return &v, c.post(ctx, c.tenantPath(tenantID, "/invoices"), body, &v)
}

func (c *HTTPClient) SendInvoiceSRI(ctx context.Context, tenantID, invoiceID uuid.UUID) (*ports.ElectronicDocument, error) {
	var v ports.ElectronicDocument
	return &v, c.post(ctx, c.tenantPath(tenantID, "/invoices/"+invoiceID.String()+"/send-sri"), map[string]any{}, &v)
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
