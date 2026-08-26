package identity_test

import (
	"context"
	"errors"
	"testing"

	"github.com/1123786563/myqypt/internal/application/identity"
)

// errRepositoryFailed is the sentinel error the stub repository returns in
// error-propagation cases.
var errRepositoryFailed = errors.New("repository failed")

// countingStub records every BindOrLoad call so tests can assert both how
// often and with which arguments the repository port was touched.
type countingStub struct {
	calls        int
	lastProvider string
	lastSubject  string
	user         identity.User
	err          error
}

func (s *countingStub) BindOrLoad(_ context.Context, identityProvider, subject string) (identity.User, error) {
	s.calls++
	s.lastProvider = identityProvider
	s.lastSubject = subject
	return s.user, s.err
}

func TestBindRejectsUnverifiedIdentity(t *testing.T) {
	cases := []struct {
		name     string
		identity identity.VerifiedIdentity
	}{
		{
			name:     "empty issuer",
			identity: identity.VerifiedIdentity{Issuer: "", Subject: "subject-1"},
		},
		{
			name:     "empty subject",
			identity: identity.VerifiedIdentity{Issuer: "https://issuer.example.test", Subject: ""},
		},
		{
			name:     "empty issuer and subject",
			identity: identity.VerifiedIdentity{Issuer: "", Subject: ""},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &countingStub{user: identity.User{ID: "must-not-be-used"}}
			service := identity.NewService(stub)

			user, err := service.Bind(context.Background(), tc.identity)

			if !errors.Is(err, identity.ErrUnverifiedIdentity) {
				t.Fatalf("Bind(...) err = %v, want ErrUnverifiedIdentity", err)
			}
			if user != (identity.User{}) {
				t.Fatalf("Bind(...) user = %+v, want the zero user", user)
			}
			if stub.calls != 0 {
				t.Fatalf("repository calls = %d, want 0 for an unverified identity", stub.calls)
			}
		})
	}
}

func TestBindDelegatesVerifiedIdentityToRepository(t *testing.T) {
	verified := identity.VerifiedIdentity{
		Issuer:  "https://issuer.example.test",
		Subject: "subject-1",
	}

	t.Run("returns the bound user", func(t *testing.T) {
		wantUser := identity.User{ID: "0199cd8e-6c9f-7cc0-9d34-6a2b5f01e88a"}
		stub := &countingStub{user: wantUser}
		service := identity.NewService(stub)

		user, err := service.Bind(context.Background(), verified)

		if err != nil {
			t.Fatalf("Bind(...) err = %v, want nil", err)
		}
		if user != wantUser {
			t.Fatalf("Bind(...) user = %+v, want %+v", user, wantUser)
		}
		if stub.calls != 1 {
			t.Fatalf("repository calls = %d, want 1", stub.calls)
		}
		if stub.lastProvider != verified.Issuer || stub.lastSubject != verified.Subject {
			t.Fatalf(
				"repository got (%q, %q), want (%q, %q)",
				stub.lastProvider, stub.lastSubject, verified.Issuer, verified.Subject,
			)
		}
	})

	t.Run("propagates the repository error", func(t *testing.T) {
		stub := &countingStub{err: errRepositoryFailed}
		service := identity.NewService(stub)

		user, err := service.Bind(context.Background(), verified)

		if !errors.Is(err, errRepositoryFailed) {
			t.Fatalf("Bind(...) err = %v, want errRepositoryFailed", err)
		}
		if user != (identity.User{}) {
			t.Fatalf("Bind(...) user = %+v, want the zero user", user)
		}
		if stub.calls != 1 {
			t.Fatalf("repository calls = %d, want 1", stub.calls)
		}
	})
}

func TestVerifiedIdentityContextRoundTrip(t *testing.T) {
	verified := identity.VerifiedIdentity{
		Issuer:  "https://issuer.example.test",
		Subject: "subject-1",
	}

	ctx := identity.WithVerifiedIdentity(context.Background(), verified)

	got, ok := identity.VerifiedIdentityFromContext(ctx)
	if !ok {
		t.Fatal("VerifiedIdentityFromContext(...) ok = false, want true after WithVerifiedIdentity")
	}
	if got != verified {
		t.Fatalf("VerifiedIdentityFromContext(...) = %+v, want %+v", got, verified)
	}

	if _, ok := identity.VerifiedIdentityFromContext(context.Background()); ok {
		t.Fatal("VerifiedIdentityFromContext(context.Background()) ok = true, want false")
	}
}
