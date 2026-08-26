package oidc

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1123786563/myqypt/internal/application/identity"
)

const testAudience = "platform-api"

// mustRSAKey generates a fresh 2048-bit RSA signing key.
func mustRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	return key
}

func b64(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// b64JSON marshals v and returns the base64url JWT segment for it.
func b64JSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return b64(raw)
}

// signJWT returns a compact-serialized RS256 JWT over the given
// base64url header and claims segments.
func signJWT(t *testing.T, key *rsa.PrivateKey, header, claims string) string {
	t.Helper()
	signingInput := header + "." + claims
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return signingInput + "." + b64(sig)
}

func rs256Header(t *testing.T, kid string) string {
	t.Helper()
	return b64JSON(t, map[string]any{"alg": "RS256", "typ": "JWT", "kid": kid})
}

// rsaJWK returns the signing JWK for key under kid.
func rsaJWK(t *testing.T, kid string, key *rsa.PrivateKey) map[string]any {
	t.Helper()
	return map[string]any{
		"kty": "RSA",
		"kid": kid,
		"use": "sig",
		"alg": "RS256",
		"n":   b64(key.PublicKey.N.Bytes()),
		"e":   b64(big.NewInt(int64(key.PublicKey.E)).Bytes()),
	}
}

func jwksDoc(t *testing.T, keys ...map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"keys": keys})
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}
	return string(raw)
}

// startIdP runs an issuer that serves discovery bound to its own URL
// and delegates /jwks to jwks.
func startIdP(t *testing.T, jwks http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			fmt.Fprintf(w, `{"issuer":%q,"jwks_uri":%q}`, "http://"+r.Host, "http://"+r.Host+"/jwks")
		case "/jwks":
			jwks(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func serveDoc(doc string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, doc)
	}
}

// validClaims returns claims accepted for testAudience at issuer with a
// comfortable validity window.
func validClaims(issuer string) map[string]any {
	return map[string]any{
		"iss": issuer,
		"sub": "subject-1",
		"aud": testAudience,
		"exp": time.Now().Add(time.Hour).Unix(),
	}
}

func mustVerifyInvalid(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, identity.ErrInvalidToken) {
		t.Fatalf("error %v does not match ErrInvalidToken", err)
	}
}

func mustVerifyUnavailable(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, identity.ErrProviderUnavailable) {
		t.Fatalf("error %v does not match ErrProviderUnavailable", err)
	}
}

// TestVerifyAcceptsValidRS256Token covers matrix 1: a correctly signed
// token verifies and returns the attested identity. Optional-claim
// absence and permissive shapes stay in scope of the same ruling.
func TestVerifyAcceptsValidRS256Token(t *testing.T) {
	key := mustRSAKey(t)
	const kid = "key-1"
	srv := startIdP(t, serveDoc(jwksDoc(t, rsaJWK(t, kid, key))))
	verifier := NewVerifier(srv.URL, testAudience)

	t.Run("string audience", func(t *testing.T) {
		token := signJWT(t, key, rs256Header(t, kid), b64JSON(t, validClaims(srv.URL)))
		got, err := verifier.Verify(context.Background(), token)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if got.Issuer != srv.URL {
			t.Errorf("Issuer = %q, want %q", got.Issuer, srv.URL)
		}
		if got.Subject != "subject-1" {
			t.Errorf("Subject = %q, want %q", got.Subject, "subject-1")
		}
	})

	t.Run("array audience containing configured", func(t *testing.T) {
		claims := validClaims(srv.URL)
		claims["aud"] = []string{"account", testAudience}
		token := signJWT(t, key, rs256Header(t, kid), b64JSON(t, claims))
		if _, err := verifier.Verify(context.Background(), token); err != nil {
			t.Fatalf("Verify: %v", err)
		}
	})

	t.Run("minimal claims without optional ones", func(t *testing.T) {
		claims := map[string]any{"iss": srv.URL, "sub": "subject-1"}
		token := signJWT(t, key, rs256Header(t, kid), b64JSON(t, claims))
		if _, err := verifier.Verify(context.Background(), token); err != nil {
			t.Fatalf("Verify: %v", err)
		}
	})

	t.Run("past nbf passes", func(t *testing.T) {
		claims := validClaims(srv.URL)
		claims["nbf"] = time.Now().Add(-time.Minute).Unix()
		token := signJWT(t, key, rs256Header(t, kid), b64JSON(t, claims))
		if _, err := verifier.Verify(context.Background(), token); err != nil {
			t.Fatalf("Verify: %v", err)
		}
	})
}

// TestVerifyRejectsTamperedPayload covers matrix 2.
func TestVerifyRejectsTamperedPayload(t *testing.T) {
	key := mustRSAKey(t)
	const kid = "key-1"
	srv := startIdP(t, serveDoc(jwksDoc(t, rsaJWK(t, kid, key))))
	verifier := NewVerifier(srv.URL, testAudience)

	token := signJWT(t, key, rs256Header(t, kid), b64JSON(t, validClaims(srv.URL)))
	parts := strings.Split(token, ".")
	escalated := validClaims(srv.URL)
	escalated["sub"] = "admin"
	parts[1] = b64JSON(t, escalated)

	_, err := verifier.Verify(context.Background(), strings.Join(parts, "."))
	mustVerifyInvalid(t, err)
}

// TestVerifyRejectsTamperedSignature covers matrix 3.
func TestVerifyRejectsTamperedSignature(t *testing.T) {
	key := mustRSAKey(t)
	const kid = "key-1"
	srv := startIdP(t, serveDoc(jwksDoc(t, rsaJWK(t, kid, key))))
	verifier := NewVerifier(srv.URL, testAudience)

	token := signJWT(t, key, rs256Header(t, kid), b64JSON(t, validClaims(srv.URL)))
	parts := strings.Split(token, ".")
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	sig[0] ^= 0xff
	parts[2] = b64(sig)

	_, err = verifier.Verify(context.Background(), strings.Join(parts, "."))
	mustVerifyInvalid(t, err)
}

// TestVerifyRejectsAlgNone covers matrix 4.
func TestVerifyRejectsAlgNone(t *testing.T) {
	key := mustRSAKey(t)
	const kid = "key-1"
	srv := startIdP(t, serveDoc(jwksDoc(t, rsaJWK(t, kid, key))))
	verifier := NewVerifier(srv.URL, testAudience)

	header := b64JSON(t, map[string]any{"alg": "none", "typ": "JWT"})
	claims := b64JSON(t, validClaims(srv.URL))

	_, err := verifier.Verify(context.Background(), header+"."+claims+".")
	mustVerifyInvalid(t, err)
}

// TestVerifyRejectsHMAC covers matrix 5: even a correctly computed
// HMAC is rejected because the algorithm whitelist is RS256 only.
func TestVerifyRejectsHMAC(t *testing.T) {
	key := mustRSAKey(t)
	const kid = "key-1"
	srv := startIdP(t, serveDoc(jwksDoc(t, rsaJWK(t, kid, key))))
	verifier := NewVerifier(srv.URL, testAudience)

	header := b64JSON(t, map[string]any{"alg": "HS256", "typ": "JWT", "kid": kid})
	signingInput := header + "." + b64JSON(t, validClaims(srv.URL))
	mac := hmac.New(sha256.New, []byte("shared-secret"))
	mac.Write([]byte(signingInput))

	_, err := verifier.Verify(context.Background(), signingInput+"."+b64(mac.Sum(nil)))
	mustVerifyInvalid(t, err)
}

// TestVerifyRejectsWrongIssuer covers matrix 6.
func TestVerifyRejectsWrongIssuer(t *testing.T) {
	key := mustRSAKey(t)
	const kid = "key-1"
	srv := startIdP(t, serveDoc(jwksDoc(t, rsaJWK(t, kid, key))))
	verifier := NewVerifier(srv.URL, testAudience)

	claims := validClaims("https://attacker.example/realms/main")
	token := signJWT(t, key, rs256Header(t, kid), b64JSON(t, claims))

	_, err := verifier.Verify(context.Background(), token)
	mustVerifyInvalid(t, err)
}

// TestVerifyRejectsWrongAudience covers matrix 7 in both JSON shapes.
func TestVerifyRejectsWrongAudience(t *testing.T) {
	key := mustRSAKey(t)
	const kid = "key-1"
	srv := startIdP(t, serveDoc(jwksDoc(t, rsaJWK(t, kid, key))))
	verifier := NewVerifier(srv.URL, testAudience)

	t.Run("string audience", func(t *testing.T) {
		claims := validClaims(srv.URL)
		claims["aud"] = "other-api"
		token := signJWT(t, key, rs256Header(t, kid), b64JSON(t, claims))
		_, err := verifier.Verify(context.Background(), token)
		mustVerifyInvalid(t, err)
	})

	t.Run("array audience without configured", func(t *testing.T) {
		claims := validClaims(srv.URL)
		claims["aud"] = []string{"account", "other-api"}
		token := signJWT(t, key, rs256Header(t, kid), b64JSON(t, claims))
		_, err := verifier.Verify(context.Background(), token)
		mustVerifyInvalid(t, err)
	})
}

// TestVerifyRejectsExpiredToken covers matrix 8 with zero leeway.
func TestVerifyRejectsExpiredToken(t *testing.T) {
	key := mustRSAKey(t)
	const kid = "key-1"
	srv := startIdP(t, serveDoc(jwksDoc(t, rsaJWK(t, kid, key))))
	verifier := NewVerifier(srv.URL, testAudience)

	t.Run("expired a minute ago", func(t *testing.T) {
		claims := validClaims(srv.URL)
		claims["exp"] = time.Now().Add(-time.Minute).Unix()
		token := signJWT(t, key, rs256Header(t, kid), b64JSON(t, claims))
		_, err := verifier.Verify(context.Background(), token)
		mustVerifyInvalid(t, err)
	})

	t.Run("expired exactly now", func(t *testing.T) {
		claims := validClaims(srv.URL)
		claims["exp"] = time.Now().Unix()
		token := signJWT(t, key, rs256Header(t, kid), b64JSON(t, claims))
		_, err := verifier.Verify(context.Background(), token)
		mustVerifyInvalid(t, err)
	})
}

// TestVerifyRejectsNotYetValidToken covers matrix 9.
func TestVerifyRejectsNotYetValidToken(t *testing.T) {
	key := mustRSAKey(t)
	const kid = "key-1"
	srv := startIdP(t, serveDoc(jwksDoc(t, rsaJWK(t, kid, key))))
	verifier := NewVerifier(srv.URL, testAudience)

	claims := validClaims(srv.URL)
	claims["nbf"] = time.Now().Add(time.Hour).Unix()
	token := signJWT(t, key, rs256Header(t, kid), b64JSON(t, claims))

	_, err := verifier.Verify(context.Background(), token)
	mustVerifyInvalid(t, err)
}

// TestVerifyRejectsEmptySubject covers matrix 10.
func TestVerifyRejectsEmptySubject(t *testing.T) {
	key := mustRSAKey(t)
	const kid = "key-1"
	srv := startIdP(t, serveDoc(jwksDoc(t, rsaJWK(t, kid, key))))
	verifier := NewVerifier(srv.URL, testAudience)

	claims := validClaims(srv.URL)
	claims["sub"] = ""
	token := signJWT(t, key, rs256Header(t, kid), b64JSON(t, claims))

	_, err := verifier.Verify(context.Background(), token)
	mustVerifyInvalid(t, err)
}

// TestVerifyNoncePolicy covers matrix 11: nonce is optional, but when
// present it must be a non-empty string.
func TestVerifyNoncePolicy(t *testing.T) {
	key := mustRSAKey(t)
	const kid = "key-1"
	srv := startIdP(t, serveDoc(jwksDoc(t, rsaJWK(t, kid, key))))
	verifier := NewVerifier(srv.URL, testAudience)

	t.Run("empty nonce rejected", func(t *testing.T) {
		claims := validClaims(srv.URL)
		claims["nonce"] = ""
		token := signJWT(t, key, rs256Header(t, kid), b64JSON(t, claims))
		_, err := verifier.Verify(context.Background(), token)
		mustVerifyInvalid(t, err)
	})

	t.Run("non-empty nonce accepted", func(t *testing.T) {
		claims := validClaims(srv.URL)
		claims["nonce"] = "nonce-1"
		token := signJWT(t, key, rs256Header(t, kid), b64JSON(t, claims))
		if _, err := verifier.Verify(context.Background(), token); err != nil {
			t.Fatalf("Verify: %v", err)
		}
	})

	t.Run("absent nonce accepted", func(t *testing.T) {
		token := signJWT(t, key, rs256Header(t, kid), b64JSON(t, validClaims(srv.URL)))
		if _, err := verifier.Verify(context.Background(), token); err != nil {
			t.Fatalf("Verify: %v", err)
		}
	})
}

// TestVerifyRotatesKeysWithSingleRefetch covers matrix 12: an unknown
// kid triggers exactly one JWKS refetch, which picks up the rotated key.
func TestVerifyRotatesKeysWithSingleRefetch(t *testing.T) {
	keyA := mustRSAKey(t)
	keyB := mustRSAKey(t)
	var jwksGETs atomic.Int32
	srv := startIdP(t, func(w http.ResponseWriter, r *http.Request) {
		if jwksGETs.Add(1) == 1 {
			serveDoc(jwksDoc(t, rsaJWK(t, "kid-a", keyA)))(w, r)
			return
		}
		serveDoc(jwksDoc(t, rsaJWK(t, "kid-b", keyB)))(w, r)
	})
	verifier := NewVerifier(srv.URL, testAudience)

	token := signJWT(t, keyB, rs256Header(t, "kid-b"), b64JSON(t, validClaims(srv.URL)))
	if _, err := verifier.Verify(context.Background(), token); err != nil {
		t.Fatalf("Verify after rotation: %v", err)
	}
	if got := jwksGETs.Load(); got != 2 {
		t.Fatalf("jwks requests = %d, want 2 (initial fetch + exactly one refetch)", got)
	}
}

// TestVerifyUnknownKidRefetchesOnceThenFails covers matrix 12: a
// refetch that still misses the kid fails without a retry loop.
func TestVerifyUnknownKidRefetchesOnceThenFails(t *testing.T) {
	keyA := mustRSAKey(t)
	var jwksGETs atomic.Int32
	srv := startIdP(t, func(w http.ResponseWriter, r *http.Request) {
		jwksGETs.Add(1)
		serveDoc(jwksDoc(t, rsaJWK(t, "kid-a", keyA)))(w, r)
	})
	verifier := NewVerifier(srv.URL, testAudience)

	token := signJWT(t, keyA, rs256Header(t, "kid-ghost"), b64JSON(t, validClaims(srv.URL)))
	_, err := verifier.Verify(context.Background(), token)
	mustVerifyInvalid(t, err)
	if got := jwksGETs.Load(); got != 2 {
		t.Fatalf("jwks requests = %d, want 2 (single refetch, no retry loop)", got)
	}
}

// TestVerifyReportsUnavailableWhenDiscoveryUnreachable covers matrix 13.
func TestVerifyReportsUnavailableWhenDiscoveryUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	issuer := srv.URL
	srv.Close()

	key := mustRSAKey(t)
	verifier := NewVerifier(issuer, testAudience)
	token := signJWT(t, key, rs256Header(t, "key-1"), b64JSON(t, validClaims(issuer)))

	_, err := verifier.Verify(context.Background(), token)
	mustVerifyUnavailable(t, err)
}

// TestVerifyReportsUnavailableWhenJWKSUnreachable covers matrix 14.
func TestVerifyReportsUnavailableWhenJWKSUnreachable(t *testing.T) {
	jwks := httptest.NewServer(http.NotFoundHandler())
	jwksURI := jwks.URL + "/jwks"
	jwks.Close()

	disc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"issuer":%q,"jwks_uri":%q}`, "http://"+r.Host, jwksURI)
	}))
	t.Cleanup(disc.Close)

	key := mustRSAKey(t)
	verifier := NewVerifier(disc.URL, testAudience)
	token := signJWT(t, key, rs256Header(t, "key-1"), b64JSON(t, validClaims(disc.URL)))

	_, err := verifier.Verify(context.Background(), token)
	mustVerifyUnavailable(t, err)
}

// TestVerifyReportsUnavailableOnHTTP500 covers matrix 15 for both
// endpoints.
func TestVerifyReportsUnavailableOnHTTP500(t *testing.T) {
	key := mustRSAKey(t)

	t.Run("discovery 500", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)

		verifier := NewVerifier(srv.URL, testAudience)
		token := signJWT(t, key, rs256Header(t, "key-1"), b64JSON(t, validClaims(srv.URL)))
		_, err := verifier.Verify(context.Background(), token)
		mustVerifyUnavailable(t, err)
	})

	t.Run("jwks 500", func(t *testing.T) {
		srv := startIdP(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		})
		verifier := NewVerifier(srv.URL, testAudience)
		token := signJWT(t, key, rs256Header(t, "key-1"), b64JSON(t, validClaims(srv.URL)))
		_, err := verifier.Verify(context.Background(), token)
		mustVerifyUnavailable(t, err)
	})
}

// TestVerifyRejectsUnusableJWKSKey covers matrix 16: the matching kid
// resolves to a key this verifier cannot use for RS256 verification.
func TestVerifyRejectsUnusableJWKSKey(t *testing.T) {
	t.Run("elliptic kty for matching kid", func(t *testing.T) {
		key := mustRSAKey(t)
		jwk := rsaJWK(t, "key-1", key)
		jwk["kty"] = "EC"
		delete(jwk, "n")
		delete(jwk, "e")
		jwk["crv"] = "P-256"
		jwk["x"] = b64(make([]byte, 32))
		jwk["y"] = b64(make([]byte, 32))

		srv := startIdP(t, serveDoc(jwksDoc(t, jwk)))
		verifier := NewVerifier(srv.URL, testAudience)
		token := signJWT(t, key, rs256Header(t, "key-1"), b64JSON(t, validClaims(srv.URL)))

		_, err := verifier.Verify(context.Background(), token)
		mustVerifyInvalid(t, err)
	})

	t.Run("encryption use for matching kid", func(t *testing.T) {
		key := mustRSAKey(t)
		jwk := rsaJWK(t, "key-1", key)
		jwk["use"] = "enc"

		srv := startIdP(t, serveDoc(jwksDoc(t, jwk)))
		verifier := NewVerifier(srv.URL, testAudience)
		token := signJWT(t, key, rs256Header(t, "key-1"), b64JSON(t, validClaims(srv.URL)))

		_, err := verifier.Verify(context.Background(), token)
		mustVerifyInvalid(t, err)
	})
}

// TestNewVerifierDoesNoIO covers matrix 17: constructing the verifier
// against an unreachable issuer returns immediately.
func TestNewVerifierDoesNoIO(t *testing.T) {
	start := time.Now()
	verifier := NewVerifier("http://127.0.0.1:1", testAudience)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("NewVerifier blocked for %v; constructor must not perform I/O", elapsed)
	}
	_ = verifier
}

// TestVerifyIsConcurrencySafe covers matrix 18: parallel Verify calls
// on one verifier all return their expected outcome.
func TestVerifyIsConcurrencySafe(t *testing.T) {
	key := mustRSAKey(t)
	const kid = "key-1"
	srv := startIdP(t, serveDoc(jwksDoc(t, rsaJWK(t, kid, key))))
	verifier := NewVerifier(srv.URL, testAudience)

	good := signJWT(t, key, rs256Header(t, kid), b64JSON(t, validClaims(srv.URL)))
	badParts := strings.Split(good, ".")
	badParts[2] = b64([]byte("bogus-signature-bytes"))
	bad := strings.Join(badParts, ".")

	type outcome struct {
		err      error
		identity identity.VerifiedIdentity
		valid    bool
	}
	const workers = 8
	const iterations = 25
	results := make(chan outcome, workers*iterations)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				token, isValid := good, true
				if (worker+i)%2 == 1 {
					token, isValid = bad, false
				}
				verified, err := verifier.Verify(context.Background(), token)
				results <- outcome{err: err, identity: verified, valid: isValid}
			}
		}(w)
	}
	wg.Wait()
	close(results)

	for res := range results {
		if res.valid {
			if res.err != nil {
				t.Errorf("valid token: Verify error %v", res.err)
				continue
			}
			if res.identity.Issuer != srv.URL || res.identity.Subject != "subject-1" {
				t.Errorf("valid token: identity = %+v", res.identity)
			}
			continue
		}
		if !errors.Is(res.err, identity.ErrInvalidToken) {
			t.Errorf("tampered token: error %v does not match ErrInvalidToken", res.err)
		}
	}
}

// TestVerifyRejectsMalformedTokens covers the malformed-token class:
// wrong segment count, bad base64, and bad JSON all fail validation
// before any network access matters.
func TestVerifyRejectsMalformedTokens(t *testing.T) {
	key := mustRSAKey(t)
	const kid = "key-1"
	srv := startIdP(t, serveDoc(jwksDoc(t, rsaJWK(t, kid, key))))
	verifier := NewVerifier(srv.URL, testAudience)

	header := rs256Header(t, kid)
	claims := b64JSON(t, validClaims(srv.URL))
	token := signJWT(t, key, header, claims)
	sig := strings.Split(token, ".")[2]

	cases := []struct {
		name  string
		token string
	}{
		{"empty string", ""},
		{"two segments", strings.Join(strings.Split(token, ".")[:2], ".")},
		{"four segments", token + ".extra"},
		{"bad base64 in header", "!!!." + claims + "." + sig},
		{"bad base64 in payload", header + ".$$$." + sig},
		{"bad base64 in signature", header + "." + claims + ".!!!"},
		{"header not json", b64([]byte("not json")) + "." + claims + "." + sig},
		{"claims not json", header + "." + b64([]byte("not json")) + "." + sig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := verifier.Verify(context.Background(), tc.token)
			mustVerifyInvalid(t, err)
		})
	}
}

// TestVerifyRejectsTokenWithoutKid locks in the header policy: JWKS
// lookup keys on kid, so a token without one cannot be verified.
func TestVerifyRejectsTokenWithoutKid(t *testing.T) {
	key := mustRSAKey(t)
	const kid = "key-1"
	srv := startIdP(t, serveDoc(jwksDoc(t, rsaJWK(t, kid, key))))
	verifier := NewVerifier(srv.URL, testAudience)

	header := b64JSON(t, map[string]any{"alg": "RS256", "typ": "JWT"})
	token := signJWT(t, key, header, b64JSON(t, validClaims(srv.URL)))

	_, err := verifier.Verify(context.Background(), token)
	mustVerifyInvalid(t, err)
}

// TestVerifyBindsDiscoveryIssuer locks in the discovery hardening: a
// discovery document whose issuer field names a different issuer makes
// the provider document unusable.
func TestVerifyBindsDiscoveryIssuer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"issuer":"https://other-issuer.example","jwks_uri":%q}`, "http://"+r.Host+"/jwks")
	}))
	t.Cleanup(srv.Close)

	key := mustRSAKey(t)
	verifier := NewVerifier(srv.URL, testAudience)
	token := signJWT(t, key, rs256Header(t, "key-1"), b64JSON(t, validClaims(srv.URL)))

	_, err := verifier.Verify(context.Background(), token)
	mustVerifyUnavailable(t, err)
}

// TestVerifyReportsUnavailableWhenDiscoveryDocIncomplete covers the
// discovery document that carries no usable jwks_uri.
func TestVerifyReportsUnavailableWhenDiscoveryDocIncomplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"issuer":%q}`, "http://"+r.Host)
	}))
	t.Cleanup(srv.Close)

	key := mustRSAKey(t)
	verifier := NewVerifier(srv.URL, testAudience)
	token := signJWT(t, key, rs256Header(t, "key-1"), b64JSON(t, validClaims(srv.URL)))

	_, err := verifier.Verify(context.Background(), token)
	mustVerifyUnavailable(t, err)
}
