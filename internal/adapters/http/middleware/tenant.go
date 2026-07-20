package middleware

import (
	"context"
	"net/http"

	"github.com/ndresdavila/platform-api/internal/application"
	"github.com/ndresdavila/platform-api/internal/domain/ports"
)

type ctxKey string

const TenantKey ctxKey = "tenant"

func TenantFromContext(ctx context.Context) (*ports.Tenant, bool) {
	t, ok := ctx.Value(TenantKey).(*ports.Tenant)
	return t, ok
}

// ResolveTenant loads tenant from X-Tenant-Slug (or default) via core.
func ResolveTenant(facade *application.Facade) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slug := r.Header.Get("X-Tenant-Slug")
			tenant, err := facade.ResolveTenant(r.Context(), slug)
			if err != nil {
				http.Error(w, `{"error":"tenant not found"}`, http.StatusNotFound)
				return
			}
			ctx := context.WithValue(r.Context(), TenantKey, tenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
