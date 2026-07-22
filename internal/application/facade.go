package application

import "github.com/ndresdavila/platform-api/internal/domain/ports"

// Facade holds shared deps for the public BFF (platform-core client).
type Facade struct {
	Core ports.CoreClient
}
