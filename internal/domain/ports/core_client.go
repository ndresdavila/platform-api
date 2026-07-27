package ports

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// CoreClient is the outbound port to platform-core (driven adapter implements it).
type CoreClient interface {
	Health(ctx context.Context) error

	GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error)
	UpsertEmployee(ctx context.Context, tenantID uuid.UUID, in UpsertEmployeeInput) (*Employee, error)

	ListPersons(ctx context.Context, tenantID uuid.UUID, q string) ([]Person, error)
	CreatePerson(ctx context.Context, tenantID uuid.UUID, in CreatePersonInput) (*Person, error)
	ListProducts(ctx context.Context, tenantID uuid.UUID, q string) ([]Product, error)
	CreateProduct(ctx context.Context, tenantID uuid.UUID, in CreateProductInput) (*Product, error)
	ListInvoices(ctx context.Context, tenantID uuid.UUID) ([]ElectronicDocument, error)
	GetInvoice(ctx context.Context, tenantID, invoiceID uuid.UUID) (*ElectronicDocument, error)
	CreateInvoice(ctx context.Context, tenantID uuid.UUID, in CreateInvoiceInput) (*ElectronicDocument, error)
	SendInvoiceSRI(ctx context.Context, tenantID, invoiceID uuid.UUID) (*ElectronicDocument, error)
}

type Tenant struct {
	ID     uuid.UUID `json:"id"`
	Slug   string    `json:"slug"`
	Name   string    `json:"name"`
	Active bool      `json:"active"`
}

type Employee struct {
	ID         uuid.UUID `json:"id"`
	ExternalID string    `json:"externalId"`
	Email      string    `json:"email"`
	FirstName  string    `json:"firstName"`
	LastName   string    `json:"lastName"`
	Role       string    `json:"role"`
	Active     bool      `json:"active"`
}

type UpsertEmployeeInput struct{ ExternalID, Email, FirstName, LastName, Role string }

type Person struct {
	ID                 uuid.UUID `json:"id"`
	Kind               string    `json:"kind"`
	IdentificationType string    `json:"identificationType"`
	Identification     string    `json:"identification"`
	Name               string    `json:"name"`
	Email              string    `json:"email"`
	Phone              string    `json:"phone"`
	Address            string    `json:"address"`
	Active             bool      `json:"active"`
}

type Product struct {
	ID         uuid.UUID `json:"id"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Unit       string    `json:"unit"`
	PriceCents int       `json:"priceCents"`
	IVARate    float64   `json:"ivaRate"`
	Active     bool      `json:"active"`
}

type ElectronicDocumentLine struct {
	ID              uuid.UUID  `json:"id"`
	LineNo          int        `json:"lineNo"`
	ProductID       *uuid.UUID `json:"productId,omitempty"`
	ProductName     string     `json:"productName"`
	Unit            string     `json:"unit"`
	Quantity        float64    `json:"quantity"`
	UnitPriceCents  int        `json:"unitPriceCents"`
	IVARate         float64    `json:"ivaRate"`
	DiscountPercent float64    `json:"discountPercent"`
	DiscountCents   int        `json:"discountCents"`
	SubtotalCents   int        `json:"subtotalCents"`
}

type ElectronicDocument struct {
	ID                   uuid.UUID                `json:"id"`
	DocType              string                   `json:"docType"`
	PartyKind            string                   `json:"partyKind"`
	PersonID             *uuid.UUID               `json:"personId,omitempty"`
	PersonName           string                   `json:"personName"`
	PersonIdentification string                   `json:"personIdentification"`
	Establishment        string                   `json:"establishment"`
	EmissionPoint        string                   `json:"emissionPoint"`
	DocumentNumber       string                   `json:"documentNumber"`
	AccessKey            string                   `json:"accessKey"`
	IssueDate            time.Time                `json:"issueDate"`
	DueDays              int                      `json:"dueDays"`
	Reference            string                   `json:"reference"`
	Seller               string                   `json:"seller"`
	Description          string                   `json:"description"`
	IsExport             bool                     `json:"isExport"`
	Status               string                   `json:"status"`
	SRIMessage           string                   `json:"sriMessage"`
	Subtotal15Cents      int                      `json:"subtotal15Cents"`
	Subtotal5Cents       int                      `json:"subtotal5Cents"`
	Subtotal0Cents       int                      `json:"subtotal0Cents"`
	DiscountCents        int                      `json:"discountCents"`
	IVA15Cents           int                      `json:"iva15Cents"`
	IVA5Cents            int                      `json:"iva5Cents"`
	ICECents             int                      `json:"iceCents"`
	TotalCents           int                      `json:"totalCents"`
	Lines                []ElectronicDocumentLine `json:"lines,omitempty"`
}

type CreatePersonInput struct {
	Kind, IdentificationType, Identification, Name, Email, Phone, Address string
}

type CreateProductInput struct {
	Code, Name, Unit string
	PriceCents       int
	IVARate          float64
}

type InvoiceLineInput struct {
	ProductID       *uuid.UUID `json:"productId"`
	ProductName     string     `json:"productName"`
	Unit            string     `json:"unit"`
	Quantity        float64    `json:"quantity"`
	UnitPriceCents  int        `json:"unitPriceCents"`
	IVARate         float64    `json:"ivaRate"`
	DiscountPercent float64    `json:"discountPercent"`
}

type CreateInvoiceInput struct {
	DocType, PartyKind, PersonName, PersonIdentification string
	PersonID                                             *uuid.UUID
	Establishment, EmissionPoint, IssueDate              string
	DueDays                                              int
	Reference, Seller, Description                       string
	IsExport, SendToSRI                                  bool
	CreatedByExternalID                                  string
	Lines                                                []InvoiceLineInput
}
