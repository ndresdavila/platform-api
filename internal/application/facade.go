package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/ndresdavila/platform-api/internal/domain/ports"
)

// Facade orchestrates public use-cases against platform-core only.
type Facade struct {
	Core              ports.CoreClient
	DefaultTenantSlug string
}

func (f *Facade) ResolveTenant(ctx context.Context, slug string) (*ports.Tenant, error) {
	if slug == "" {
		slug = f.DefaultTenantSlug
	}
	if slug == "" {
		return nil, fmt.Errorf("tenant slug required")
	}
	return f.Core.GetTenantBySlug(ctx, slug)
}

func (f *Facade) ListCatalog(ctx context.Context, tenantID uuid.UUID) ([]ports.CatalogService, error) {
	return f.Core.ListServices(ctx, tenantID)
}

func (f *Facade) CreateBooking(ctx context.Context, tenantID uuid.UUID, in ports.CreateBookingInput) (*ports.Booking, error) {
	return f.Core.CreateBooking(ctx, tenantID, in)
}

func (f *Facade) ListBookings(ctx context.Context, tenantID, customerID uuid.UUID) ([]ports.Booking, error) {
	return f.Core.ListBookingsByCustomer(ctx, tenantID, customerID)
}

func (f *Facade) CancelBooking(ctx context.Context, tenantID, bookingID uuid.UUID) (*ports.Booking, error) {
	return f.Core.CancelBooking(ctx, tenantID, bookingID)
}

func (f *Facade) RegisterPayment(ctx context.Context, tenantID uuid.UUID, in ports.RegisterPaymentInput) (*ports.Payment, error) {
	return f.Core.RegisterPayment(ctx, tenantID, in)
}

func (f *Facade) ListPayments(ctx context.Context, tenantID, customerID uuid.UUID) ([]ports.Payment, error) {
	return f.Core.ListPaymentsByCustomer(ctx, tenantID, customerID)
}
