package httptransport

import (
	"context"

	"github.com/1123786563/myqypt/internal/transport/http/api"
)

// StatusHandler serves the system status endpoint defined by the OpenAPI
// contract. It has no business side effects.
type StatusHandler struct {
	Version string
}

var _ api.StrictServerInterface = (*StatusHandler)(nil)

func (h *StatusHandler) GetSystemStatus(ctx context.Context, request api.GetSystemStatusRequestObject) (api.GetSystemStatusResponseObject, error) {
	return api.GetSystemStatus200JSONResponse{
		Status:  api.Available,
		Version: h.Version,
	}, nil
}
