// Package oidc verifies OIDC bearer tokens issued by a Keycloak
// identity provider, implementing the application identity Verifier
// port with the Go standard library only.
package oidc

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/1123786563/myqypt/internal/application/identity"
)

const (
	// requestTimeout bounds every discovery and JWKS request.
	requestTimeout = 5 * time.Second
	// discoveryPath is appended to the configured issuer URL.
	discoveryPath = "/.well-known/openid-configuration"
)

// Verifier verifies RS256 bearer tokens from one configured issuer.
type Verifier struct {
	issuer   string
	audience string
	client   *http.Client

	// mu guards the lazy discovery state and the kid key cache; it is
	// held across provider requests so concurrent Verifies serialize
	// instead of racing duplicate fetches.
	mu      sync.Mutex
	jwksURI string
	keys    map[string]*rsa.PublicKey
}

var _ identity.Verifier = (*Verifier)(nil)

// NewVerifier returns a Verifier for tokens issued by issuer for
// audience. It performs no network I/O: discovery and keys are fetched
// lazily on first Verify, so startup never depends on identity
// provider availability.
func NewVerifier(issuer, audience string) *Verifier {
	return &Verifier{
		issuer:   issuer,
		audience: audience,
		client:   &http.Client{Timeout: requestTimeout},
	}
}

// Verify verifies rawToken as a compact-serialized JWT and returns the
// identity it attests. Tokens that are malformed, signed with a
// disallowed algorithm, cryptographically invalid, or rejected by the
// claims policy fail with identity.ErrInvalidToken; a provider that
// cannot be reached or serves an unusable document fails with
// identity.ErrProviderUnavailable.
func (v *Verifier) Verify(ctx context.Context, rawToken string) (identity.VerifiedIdentity, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return identity.VerifiedIdentity{}, invalidToken("token must have three dot-separated segments")
	}
	headerJSON, err := decodeSegment(parts[0])
	if err != nil {
		return identity.VerifiedIdentity{}, invalidToken("decode header")
	}
	claimsJSON, err := decodeSegment(parts[1])
	if err != nil {
		return identity.VerifiedIdentity{}, invalidToken("decode claims")
	}
	sig, err := decodeSegment(parts[2])
	if err != nil {
		return identity.VerifiedIdentity{}, invalidToken("decode signature")
	}
	var header tokenHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return identity.VerifiedIdentity{}, invalidToken("parse header")
	}
	if header.Alg != "RS256" {
		return identity.VerifiedIdentity{}, invalidToken(fmt.Sprintf("alg %q is not allowed", header.Alg))
	}
	if header.Kid == "" {
		return identity.VerifiedIdentity{}, invalidToken("header carries no kid")
	}

	key, err := v.publicKey(ctx, header.Kid)
	if err != nil {
		return identity.VerifiedIdentity{}, err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return identity.VerifiedIdentity{}, invalidToken("signature does not verify")
	}

	var claims tokenClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return identity.VerifiedIdentity{}, invalidToken("parse claims")
	}
	if err := claims.validate(v.issuer, v.audience, time.Now()); err != nil {
		return identity.VerifiedIdentity{}, err
	}
	return identity.VerifiedIdentity{Issuer: claims.Issuer, Subject: claims.Subject}, nil
}

// publicKey returns the cached signing key for kid, fetching discovery
// and JWKS on first use and refreshing the key set exactly once when
// the kid is unknown (key rotation).
func (v *Verifier) publicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.jwksURI == "" {
		if err := v.discover(ctx); err != nil {
			return nil, err
		}
	}
	if v.keys == nil {
		if err := v.fetchJWKS(ctx); err != nil {
			return nil, err
		}
	}
	if key, ok := v.keys[kid]; ok {
		return key, nil
	}
	if err := v.fetchJWKS(ctx); err != nil {
		return nil, err
	}
	if key, ok := v.keys[kid]; ok {
		return key, nil
	}
	return nil, invalidToken(fmt.Sprintf("key set carries no usable key for kid %q", kid))
}

// discover fetches the issuer's OIDC discovery document and caches its
// jwks_uri. The caller must hold v.mu. A document whose issuer field
// names another issuer is unusable, per the OIDC discovery binding.
func (v *Verifier) discover(ctx context.Context) error {
	uri := strings.TrimSuffix(v.issuer, "/") + discoveryPath
	body, err := v.get(ctx, uri)
	if err != nil {
		return providerUnavailable(fmt.Sprintf("discovery: %v", err))
	}
	var doc struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return providerUnavailable("discovery: document is not valid json")
	}
	if doc.Issuer != "" && doc.Issuer != v.issuer {
		return providerUnavailable("discovery: document names a different issuer")
	}
	parsed, err := url.Parse(doc.JWKSURI)
	if doc.JWKSURI == "" || err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return providerUnavailable("discovery: document carries no usable jwks_uri")
	}
	v.jwksURI = doc.JWKSURI
	return nil
}

// fetchJWKS replaces the key cache with the provider's current key
// set. The caller must hold v.mu. Keys that cannot serve RS256
// verification (other kty, use, or alg) are skipped; a document that
// parses leaves the provider healthy even when no key matches a token.
func (v *Verifier) fetchJWKS(ctx context.Context) error {
	body, err := v.get(ctx, v.jwksURI)
	if err != nil {
		return providerUnavailable(fmt.Sprintf("jwks: %v", err))
	}
	var doc struct {
		Keys []jsonWebKey `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return providerUnavailable("jwks: document is not valid json")
	}
	keys := make(map[string]*rsa.PublicKey)
	for _, candidate := range doc.Keys {
		if key, ok := candidate.rsaPublicKey(); ok {
			keys[candidate.Kid] = key
		}
	}
	v.keys = keys
	return nil
}

// get performs a GET with the shared client and returns the response
// body for any 2xx status.
func (v *Verifier) get(ctx context.Context, uri string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// tokenHeader is the JWT header fields the verifier decides on.
type tokenHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

// tokenClaims is the JWT payload. Expiry and NotBefore are pointers so
// an absent claim stays distinguishable from a present one.
type tokenClaims struct {
	Issuer    string          `json:"iss"`
	Subject   string          `json:"sub"`
	Audience  json.RawMessage `json:"aud"`
	Expiry    *int64          `json:"exp"`
	NotBefore *int64          `json:"nbf"`
	Nonce     json.RawMessage `json:"nonce"`
}

// validate enforces the claims policy: the issuer must match the
// configuration exactly, the audience must contain it when present,
// the token must be within its validity window with zero leeway, the
// subject must be non-empty, and a nonce, when present, must be a
// non-empty string.
func (c *tokenClaims) validate(issuer, audience string, now time.Time) error {
	if c.Issuer != issuer {
		return invalidToken(fmt.Sprintf("issuer %q does not match the configured issuer", c.Issuer))
	}
	accepted, err := audienceAccepts(c.Audience, audience)
	if err != nil {
		return invalidToken("aud claim is neither a string nor an array of strings")
	}
	if !accepted {
		return invalidToken("aud claim does not include the configured audience")
	}
	if c.Expiry != nil && *c.Expiry <= now.Unix() {
		return invalidToken("token is expired")
	}
	if c.NotBefore != nil && *c.NotBefore > now.Unix() {
		return invalidToken("token is not yet valid")
	}
	if c.Subject == "" {
		return invalidToken("sub claim is empty")
	}
	if len(c.Nonce) != 0 {
		var nonce string
		if err := json.Unmarshal(c.Nonce, &nonce); err != nil || nonce == "" {
			return invalidToken("nonce claim must be a non-empty string when present")
		}
	}
	return nil
}

// audienceAccepts reports whether the raw aud claim admits audience. An
// absent claim accepts; a JSON string must equal it; a JSON array must
// contain it.
func audienceAccepts(raw json.RawMessage, audience string) (bool, error) {
	if len(raw) == 0 {
		return true, nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return single == audience, nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		for _, item := range list {
			if item == audience {
				return true, nil
			}
		}
		return false, nil
	}
	return false, errors.New("aud claim has an unusable shape")
}

// jsonWebKey is one JWK from a JWKS document.
type jsonWebKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// rsaPublicKey decodes the JWK into an RSA public key usable for
// RS256 verification, reporting whether the key qualifies.
func (k jsonWebKey) rsaPublicKey() (*rsa.PublicKey, bool) {
	if k.Kty != "RSA" || k.Kid == "" {
		return nil, false
	}
	if k.Use != "" && k.Use != "sig" {
		return nil, false
	}
	if k.Alg != "" && k.Alg != "RS256" {
		return nil, false
	}
	n, err := decodeSegment(k.N)
	if err != nil || len(n) == 0 {
		return nil, false
	}
	e, err := decodeSegment(k.E)
	if err != nil {
		return nil, false
	}
	exponent := new(big.Int).SetBytes(e)
	if !exponent.IsInt64() || exponent.Int64() <= 0 || exponent.Int64() > math.MaxInt32 {
		return nil, false
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(exponent.Int64())}, true
}

// decodeSegment decodes an unpadded base64url JWT segment.
func decodeSegment(seg string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(seg)
}

// invalidToken wraps identity.ErrInvalidToken so transport layers can
// map the failure with errors.Is.
func invalidToken(reason string) error {
	return fmt.Errorf("oidc: %w: %s", identity.ErrInvalidToken, reason)
}

// providerUnavailable wraps identity.ErrProviderUnavailable so
// transport layers can map the failure with errors.Is.
func providerUnavailable(reason string) error {
	return fmt.Errorf("oidc: %w: %s", identity.ErrProviderUnavailable, reason)
}
