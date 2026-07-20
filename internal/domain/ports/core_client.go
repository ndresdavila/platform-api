package ports

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// CoreClient is the outbound port to platform-core (driven adapter implements it).
type CoreClient interface {
	Health(ctx context.Context) error

	CreateTenant(ctx context.Context, slug, name string) (*Tenant, error)
	GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error)
	GetSettings(ctx context.Context, tenantID uuid.UUID) (*TenantSettings, error)
	PutSettings(ctx context.Context, tenantID uuid.UUID, settings TenantSettings) (*TenantSettings, error)
	UpsertCustomer(ctx context.Context, tenantID uuid.UUID, in UpsertCustomerInput) (*Customer, error)
	GetCustomerByExternal(ctx context.Context, tenantID uuid.UUID, externalID string) (*Customer, error)
	GetCustomer(ctx context.Context, tenantID, customerID uuid.UUID) (*Customer, error)
	UpdateCustomerProfile(ctx context.Context, tenantID, customerID uuid.UUID, in CustomerProfileInput) (*Customer, error)
	UpsertEmployee(ctx context.Context, tenantID uuid.UUID, in UpsertEmployeeInput) (*Employee, error)
	ListServices(ctx context.Context, tenantID uuid.UUID) ([]CatalogService, error)
	CreateService(ctx context.Context, tenantID uuid.UUID, service CatalogService) (*CatalogService, error)
	GetBooking(ctx context.Context, tenantID, bookingID uuid.UUID) (*Booking, error)
	CreateBooking(ctx context.Context, tenantID uuid.UUID, in CreateBookingInput) (*Booking, error)
	ListBookingsByCustomer(ctx context.Context, tenantID, customerID uuid.UUID) ([]Booking, error)
	// CancelBooking is retained for the small v1 facade.
	CancelBooking(ctx context.Context, tenantID, bookingID uuid.UUID) (*Booking, error)
	ListBookingsByRange(ctx context.Context, tenantID uuid.UUID, from, to time.Time) ([]Booking, error)
	CancelBookingCustomer(ctx context.Context, tenantID, bookingID uuid.UUID) (*Booking, error)
	CancelBookingStaff(ctx context.Context, tenantID, bookingID uuid.UUID) (*Booking, error)
	AdvanceBooking(ctx context.Context, tenantID, bookingID uuid.UUID, stage *string) (*Booking, error)
	RegisterPayment(ctx context.Context, tenantID uuid.UUID, in RegisterPaymentInput) (*Payment, error)
	ListPaymentsByCustomer(ctx context.Context, tenantID, customerID uuid.UUID) ([]Payment, error)
	GetPayment(ctx context.Context, tenantID, paymentID uuid.UUID) (*Payment, error)
	MarkPaymentPaid(ctx context.Context, tenantID, paymentID uuid.UUID) (*Payment, error)
	VoidPayment(ctx context.Context, tenantID, paymentID uuid.UUID) (*Payment, error)
	ListBankAccounts(ctx context.Context, tenantID uuid.UUID) ([]BankAccount, error)
	CreateBankAccount(ctx context.Context, tenantID uuid.UUID, account BankAccount) (*BankAccount, error)
	CreateDesign(ctx context.Context, tenantID uuid.UUID, in CreateDesignInput) (*Design, error)
	ListDesigns(ctx context.Context, tenantID, customerID uuid.UUID) ([]Design, error)
	GetDesign(ctx context.Context, tenantID, designID uuid.UUID) (*Design, error)
	GetDesignQuota(ctx context.Context, tenantID, customerID uuid.UUID) (*DesignQuota, error)
	CompleteDesign(ctx context.Context, tenantID, designID uuid.UUID, in CompleteDesignInput) (*Design, error)
	DeleteDesign(ctx context.Context, tenantID, designID uuid.UUID) error
}

type Tenant struct {
	ID     uuid.UUID `json:"id"`
	Slug   string    `json:"slug"`
	Name   string    `json:"name"`
	Active bool      `json:"active"`
}
type TenantSettings struct {
	TenantID           uuid.UUID `json:"tenantId"`
	FeatureDesignAI    bool      `json:"featureDesignAi"`
	DesignAIDailyLimit int       `json:"designAiDailyLimit"`
	DesignAIMaxTotal   int       `json:"designAiMaxTotal"`
}
type Customer struct {
	ID         uuid.UUID `json:"id"`
	ExternalID string    `json:"externalId"`
	NationalID *string   `json:"nationalId"`
	Email      string    `json:"email"`
	FirstName  string    `json:"firstName"`
	LastName   string    `json:"lastName"`
	Phone      string    `json:"phone"`
	Role       string    `json:"role"`
	Active     bool      `json:"active"`
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
type UpsertCustomerInput struct {
	ExternalID, Email, FirstName, LastName, Phone, Role string
	NationalID                                          *string
}
type CustomerProfileInput struct{ NationalID, FirstName, LastName, Phone string }
type UpsertEmployeeInput struct{ ExternalID, Email, FirstName, LastName, Role string }

type CatalogService struct {
	ID          uuid.UUID `json:"id"`
	TenantID    uuid.UUID `json:"tenantId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	DurationMin int       `json:"durationMin"`
	PriceCents  int       `json:"priceCents"`
	Currency    string    `json:"currency"`
	Active      bool      `json:"active"`
}

type Booking struct {
	ID             uuid.UUID  `json:"id"`
	TenantID       uuid.UUID  `json:"tenantId"`
	CustomerID     uuid.UUID  `json:"customerId"`
	ServiceID      uuid.UUID  `json:"serviceId"`
	StartsAt       time.Time  `json:"startsAt"`
	Status         string     `json:"status"`
	Notes          string     `json:"notes"`
	DesignID       *uuid.UUID `json:"designId,omitempty"`
	OperativeStage string     `json:"operativeStage"`
	CreatedAt      time.Time  `json:"createdAt"`
}

type Payment struct {
	ID            uuid.UUID  `json:"id"`
	TenantID      uuid.UUID  `json:"tenantId"`
	CustomerID    uuid.UUID  `json:"customerId"`
	BookingID     *uuid.UUID `json:"bookingId,omitempty"`
	Method        string     `json:"method"`
	Status        string     `json:"status"`
	AmountCents   int        `json:"amountCents"`
	Currency      string     `json:"currency"`
	BankAccountID *uuid.UUID `json:"bankAccountId,omitempty"`
	BankCode      string     `json:"bankCode"`
	Reference     string     `json:"reference"`
	ReceiptURL    string     `json:"receiptUrl"`
	AdminNotes    string     `json:"adminNotes"`
	CreatedAt     time.Time  `json:"createdAt"`
}
type BankAccount struct {
	ID            uuid.UUID `json:"id"`
	BankCode      string    `json:"bankCode"`
	HolderName    string    `json:"holderName"`
	AccountNumber string    `json:"accountNumber"`
	AccountType   string    `json:"accountType"`
	TaxID         string    `json:"taxId"`
	NotifyEmail   string    `json:"notifyEmail"`
	Active        bool      `json:"active"`
	SortOrder     int       `json:"sortOrder"`
}
type Design struct {
	ID           uuid.UUID `json:"id"`
	CustomerID   uuid.UUID `json:"customerId"`
	Prompt       string    `json:"prompt"`
	PhotoURL     string    `json:"photoUrl"`
	ResultURL    string    `json:"resultUrl"`
	ModelUsed    string    `json:"modelUsed"`
	Status       string    `json:"status"`
	ErrorMessage string    `json:"errorMessage"`
	CreatedAt    time.Time `json:"createdAt"`
}
type DesignQuota struct {
	Used      int `json:"used"`
	Limit     int `json:"limit"`
	TotalUsed int `json:"totalUsed"`
	TotalMax  int `json:"totalMax"`
}

type CreateBookingInput struct {
	CustomerID uuid.UUID
	ServiceID  uuid.UUID
	StartsAt   time.Time
	Notes      string
	DesignID   *uuid.UUID
}

type RegisterPaymentInput struct {
	CustomerID    uuid.UUID
	BookingID     *uuid.UUID
	Method        string
	AmountCents   int
	Currency      string
	Reference     string
	ReceiptURL    string
	BankAccountID *uuid.UUID
	BankCode      string
}
type CreateDesignInput struct {
	CustomerID                  uuid.UUID
	Prompt, PhotoURL, ModelUsed string
}
type CompleteDesignInput struct {
	ResultURL    string
	Failed       bool
	ErrorMessage string
}
