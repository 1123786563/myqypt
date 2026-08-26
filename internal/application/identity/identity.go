// Package identity binds verified external identities to platform users.
//
// An identity is verified when a trusted issuer asserts a non-empty
// subject; mutable profile claims such as email, phone, or username are
// never identity keys (ADR 0024).
package identity

import (
	"context"
	"errors"
)

// VerifiedIdentity is an identity whose issuer and subject were verified
// end to end. An empty Issuer or Subject means the verification never
// happened.
type VerifiedIdentity struct {
	Issuer  string
	Subject string
}

// User is a platform user keyed by its immutable platform-side identifier.
type User struct {
	ID string
}

// ErrUnverifiedIdentity reports a Bind attempt with an identity whose
// issuer or subject is missing.
var ErrUnverifiedIdentity = errors.New("identity: unverified identity")

// ErrInvalidToken reports a bearer token that is malformed, uses a
// disallowed algorithm, fails cryptographic verification, or carries
// claims the configured verification policy rejects.
var ErrInvalidToken = errors.New("identity: invalid token")

// ErrProviderUnavailable reports that the identity provider could not
// be reached or served an unusable discovery or key document.
var ErrProviderUnavailable = errors.New("identity: provider unavailable")

// Repository is the persistence port for identity binding: it returns the
// platform user already bound to (identityProvider, subject), or creates
// the user, its binding, and the new user's complete personal-tenant
// bundle — exactly one personal tenant, its 1:1 billing customer, and the
// active owner membership — atomically on first delivery. The created
// flag reports which path ran: true exactly when this call inserted a new
// user with its tenant bundle, false when an existing user was loaded.
type Repository interface {
	BindOrLoad(ctx context.Context, identityProvider, subject string) (User, bool, error)
}

// Verifier is the verification port for bearer tokens issued by the
// configured identity provider: Verify returns the identity the token
// attests, ErrInvalidToken for tokens that fail validation, and
// ErrProviderUnavailable when the provider cannot serve its keys.
type Verifier interface {
	Verify(ctx context.Context, rawToken string) (VerifiedIdentity, error)
}

// verifiedIdentityKey is the unexported context key type, so values set by
// WithVerifiedIdentity cannot collide with unrelated context keys.
type verifiedIdentityKey struct{}

// WithVerifiedIdentity returns a copy of ctx carrying verified for
// downstream consumers in the same request.
func WithVerifiedIdentity(ctx context.Context, verified VerifiedIdentity) context.Context {
	return context.WithValue(ctx, verifiedIdentityKey{}, verified)
}

// VerifiedIdentityFromContext returns the verified identity attached with
// WithVerifiedIdentity, reporting whether one is present.
func VerifiedIdentityFromContext(ctx context.Context) (VerifiedIdentity, bool) {
	verified, ok := ctx.Value(verifiedIdentityKey{}).(VerifiedIdentity)
	return verified, ok
}
