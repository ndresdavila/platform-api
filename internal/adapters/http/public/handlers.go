package public

import (
	"encoding/json"
	"net/http"

	"github.com/ndresdavila/platform-api/internal/application"
	"github.com/ndresdavila/platform-api/internal/domain/ports"
)

type Handler struct {
	Facade    *application.Facade
	Tenant    *ports.Tenant
	UploadDir string
}

func NewRouter(h *Handler) http.Handler {
	return newBFFRouter(h)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
