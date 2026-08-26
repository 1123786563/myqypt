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

// Repository is the persistence port for identity binding: it returns the
// platform user already bound to (identityProvider, subject), or creates
// the user and its binding atomically on first delivery.
type Repository interface {
	BindOrLoad(ctx context.Context, identityProvider, subject string) (User, error)
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
