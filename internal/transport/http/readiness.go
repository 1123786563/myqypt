package httptransport

import (
	"encoding/json"
	"net/http"

	"github.com/1123786563/myqypt/internal/application/readiness"
	"github.com/gin-gonic/gin"
)

// readinessBody is the exact response shape of /readyz: check names and
// their states, nothing else. Error text, DSNs, and hostnames never appear.
type readinessBody struct {
	Checks map[string]string `json:"checks"`
}

// failClosedReadinessBody is served when no readiness service is wired: a
// process whose dependency state is unknown must never report ready.
const failClosedReadinessBody = `{"checks":{}}`

// readinessHandler serves GET /readyz, an operational sibling of /livez
// that stays outside the OpenAPI contract. The HTTP status carries the
// ready semantics (200 ready, 503 not ready); the body carries only the
// per-check states. A nil service (zero Dependencies) fails closed.
func readinessHandler(service *readiness.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		ready := false
		// Non-nil from the start: a missing service marshals as an empty
		// checks object rather than null.
		checks := map[string]string{}
		if service != nil {
			result := service.Check(c.Request.Context())
			ready = result.Ready
			checks = result.Checks
		}

		status := http.StatusServiceUnavailable
		if ready {
			status = http.StatusOK
		}

		body, err := json.Marshal(readinessBody{Checks: checks})
		if err != nil {
			// A map[string]string cannot fail to marshal; fail closed anyway.
			c.Header("Content-Type", "application/json")
			c.String(http.StatusServiceUnavailable, failClosedReadinessBody)
			return
		}
		c.Header("Content-Type", "application/json")
		c.String(status, string(body))
	}
}
