package platform

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/ndresdavila/platform-api/internal/adapters/coreclient"
	"github.com/ndresdavila/platform-api/internal/adapters/http/public"
	"github.com/ndresdavila/platform-api/internal/application"
)

type Config struct {
	HTTPAddr          string
	CoreBaseURL       string
	DefaultTenantSlug string
}

func LoadConfig() Config {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8092"
	}
	core := os.Getenv("CORE_BASE_URL")
	if core == "" {
		core = "http://localhost:8093"
	}
	return Config{
		HTTPAddr:          addr,
		CoreBaseURL:       core,
		DefaultTenantSlug: env("DEFAULT_TENANT_SLUG", "masterview"),
	}
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func NewServer(cfg Config) (http.Handler, error) {
	if cfg.CoreBaseURL == "" {
		return nil, fmt.Errorf("CORE_BASE_URL required")
	}
	core := coreclient.NewHTTPClient(cfg.CoreBaseURL)
	tenant, err := core.GetTenantBySlug(context.Background(), cfg.DefaultTenantSlug)
	if err != nil {
		return nil, fmt.Errorf("resolve default tenant %q: %w", cfg.DefaultTenantSlug, err)
	}
	facade := &application.Facade{Core: core}
	h := &public.Handler{Facade: facade, Tenant: tenant}
	return public.NewRouter(h), nil
}
