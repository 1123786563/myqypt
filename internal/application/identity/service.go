package identity

import "context"

// Service binds verified identities to platform users through the
// Repository port.
type Service struct {
	repository Repository
}

// NewService returns a Service that persists through r.
func NewService(r Repository) *Service {
	return &Service{repository: r}
}

// Bind binds a verified identity to its platform user, loading the
// existing user on repeated delivery. The created flag reports which path
// the repository took: true when this delivery bound a brand-new user,
// false when it loaded an existing one. An identity with an empty issuer
// or subject was never verified end to end and is rejected with
// ErrUnverifiedIdentity (created always false) without touching the
// repository.
func (s *Service) Bind(ctx context.Context, verified VerifiedIdentity) (User, bool, error) {
	if verified.Issuer == "" || verified.Subject == "" {
		return User{}, false, ErrUnverifiedIdentity
	}
	return s.repository.BindOrLoad(ctx, verified.Issuer, verified.Subject)
}
