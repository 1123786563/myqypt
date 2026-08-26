package httptransport

import (
	"errors"
	"net/http"
	"strings"

	"github.com/1123786563/myqypt/internal/application/identity"
	"github.com/gin-gonic/gin"
)

// identityCallbackPath is the internal identity callback route. Like /livez
// it stays outside the OpenAPI contract: it is machine-to-machine plumbing
// wired per deployment, never part of the public surface.
const identityCallbackPath = "/internal/v1/identity/callback"

// IdentityDependencies wires the identity callback endpoint: the Verifier
// port authenticates the bearer token, the Repository port binds the
// verified identity to a platform user. A nil *IdentityDependencies on
// Dependencies leaves the endpoint unregistered; a non-nil assembly with a
// nil port fails closed at request time (503) — the /readyz fail-closed
// precedent, applied per dependency.
type IdentityDependencies struct {
	Verifier   identity.Verifier
	Repository identity.Repository
}

// registerIdentityRoutes registers POST /internal/v1/identity/callback when
// the identity assembly is configured. Registration mirrors /livez: outside
// the OpenAPI validator group and before NoRoute, so the catch-all problem
// mapping can never shadow the route.
func registerIdentityRoutes(router *gin.Engine, deps *IdentityDependencies) {
	if deps == nil {
		return
	}
	router.POST(identityCallbackPath, identityBearerMiddleware(deps.Verifier), identityCallbackHandler(deps))
}

// identityBearerMiddleware authenticates a request through the Verifier
// port. Only the Authorization: Bearer header is read — the issuer and
// subject always come from the verified token, never from the request body
// or other headers. Status mapping (design ruling 6): missing or malformed
// credentials and ErrInvalidToken → 401 unauthorized, ErrProviderUnavailable
// → 503 dependency_unavailable, any other failure → 500 internal_error. A
// nil verifier fails closed with 503.
func identityBearerMiddleware(verifier identity.Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		if verifier == nil {
			WriteProblem(c, newProblem(http.StatusServiceUnavailable, CodeDependencyUnavailable))
			return
		}
		rawToken, ok := bearerToken(c.Request.Header.Get("Authorization"))
		if !ok {
			WriteProblem(c, newProblem(http.StatusUnauthorized, CodeUnauthorized))
			return
		}
		verified, err := verifier.Verify(c.Request.Context(), rawToken)
		if err != nil {
			switch {
			case errors.Is(err, identity.ErrInvalidToken):
				WriteProblem(c, newProblem(http.StatusUnauthorized, CodeUnauthorized))
			case errors.Is(err, identity.ErrProviderUnavailable):
				WriteProblem(c, newProblem(http.StatusServiceUnavailable, CodeDependencyUnavailable))
			default:
				WriteProblem(c, newProblem(http.StatusInternalServerError, CodeInternalError))
			}
			return
		}
		c.Request = c.Request.WithContext(identity.WithVerifiedIdentity(c.Request.Context(), verified))
		c.Next()
	}
}

// bearerToken extracts the raw bearer token from an Authorization header
// value. Only the exact "Bearer <token>" form counts as credentials: the
// case-sensitive scheme, one separating space, and a non-empty token with
// no interior whitespace. Every other shape is treated as missing
// credentials.
func bearerToken(header string) (string, bool) {
	const scheme = "Bearer "
	if !strings.HasPrefix(header, scheme) {
		return "", false
	}
	token := strings.TrimPrefix(header, scheme)
	if token == "" || strings.ContainsAny(token, " \t") {
		return "", false
	}
	return token, true
}

// identityCallbackBody is the exact response shape of the identity
// callback: the platform user id, nothing else — tokens and claims never
// appear in a response body.
type identityCallbackBody struct {
	UserID string `json:"user_id"`
}

// identityCallbackHandler binds the verified identity carried in the
// request context (never read from the request itself) to its platform
// user through the application Service. Status mapping (design ruling 6):
// first bind → 201, idempotent rebind → 200, both with the same
// {"user_id": ...} body; a missing verified identity → 401; an
// unconfigured repository and ErrProviderUnavailable → 503
// dependency_unavailable; any other error → 500 internal_error.
func identityCallbackHandler(deps *IdentityDependencies) gin.HandlerFunc {
	bind := identity.NewService(deps.Repository)
	return func(c *gin.Context) {
		if deps.Repository == nil {
			WriteProblem(c, newProblem(http.StatusServiceUnavailable, CodeDependencyUnavailable))
			return
		}
		verified, ok := identity.VerifiedIdentityFromContext(c.Request.Context())
		if !ok {
			WriteProblem(c, newProblem(http.StatusUnauthorized, CodeUnauthorized))
			return
		}
		user, created, err := bind.Bind(c.Request.Context(), verified)
		if err != nil {
			if errors.Is(err, identity.ErrProviderUnavailable) {
				WriteProblem(c, newProblem(http.StatusServiceUnavailable, CodeDependencyUnavailable))
			} else {
				WriteProblem(c, newProblem(http.StatusInternalServerError, CodeInternalError))
			}
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		c.JSON(status, identityCallbackBody{UserID: user.ID})
	}
}
