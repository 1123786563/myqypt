package httptransport

import (
	"context"

	"github.com/1123786563/myqypt/internal/transport/http/api"
)

// StatusHandler serves the system status endpoint defined by the OpenAPI
// contract. It has no business side effects. Since the contract grew the
// tenant-context operations, the full api.StrictServerInterface is
// implemented by the aggregate registered in NewRouter (contractAPI),
// which embeds this handler.
type StatusHandler struct {
	Version string
}

func (h *StatusHandler) GetSystemStatus(ctx context.Context, request api.GetSystemStatusRequestObject) (api.GetSystemStatusResponseObject, error) {
	return api.GetSystemStatus200JSONResponse{
		Status:  api.Available,
		Version: h.Version,
	}, nil
}
