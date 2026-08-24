# T01.2 Keycloak Verified Subject 与 Identity Binding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 只接受经 Keycloak OIDC 验证的 issuer + subject，在 Platform PostgreSQL 幂等建立 User 与 Identity Binding，且不使用 email、phone 或 username 作为身份键。

**Architecture:** OIDC middleware verifies signature, issuer, audience, expiry, and nonce before constructing `VerifiedIdentity`; the Identity service never accepts issuer/subject from a client body or header. One PostgreSQL transaction upserts the immutable issuer/subject binding and its Platform User, while mutable profile claims remain non-key attributes.

**Tech Stack:** Go 1.26, Keycloak OIDC, PostgreSQL, black-box HTTP acceptance harness

**Spec:** [GitHub Issue #101](https://github.com/1123786563/myqypt/issues/101), `docs/adr/0024-separate-platform-users-from-keycloak-identities.md`, `CONTEXT.md`

## Global Constraints

- Only verified `issuer + subject` identifies a User; email, phone, and username cannot be unique cross-system keys.
- Duplicate callbacks are idempotent and return the same Platform User.
- Keycloak identity deletion or disablement cannot cascade-delete Platform history or Product data.
- Tokens, credentials, and mutable claims do not enter Audit or test evidence.

---

### Task 1: Persist and expose verified Identity Binding

**Files:**
- Create: `db/migrations/000001_identity_bindings.sql`
- Create: `internal/identity/binding.go`
- Create: `internal/identity/postgres_repository.go`
- Create: `internal/identity/binding_test.go`
- Create: `internal/identity/http_handler.go`
- Create: `internal/identity/http_handler_test.go`

**Interfaces:**
- Consumes: `platform.New`, PostgreSQL pool, and verified OIDC claims from #100.
- Produces: `Bind(ctx context.Context, identity VerifiedIdentity) (User, error)` and `POST /internal/v1/identity/callback` whose identity comes only from verified request context.

- [ ] **Step 1: Write the failing repository idempotency test**

```go
func TestBindReturnsOneUserForRepeatedIssuerSubject(t *testing.T) {
    identity := identity.VerifiedIdentity{Issuer: "http://keycloak:8080/realms/myqypt", Subject: "subject-t01"}
    first, err := service.Bind(context.Background(), identity)
    if err != nil { t.Fatal(err) }
    second, err := service.Bind(context.Background(), identity)
    if err != nil { t.Fatal(err) }
    if first.ID != second.ID { t.Fatalf("%s != %s", first.ID, second.ID) }
    assertBindingCount(t, db, identity, 1)
}
```

- [ ] **Step 2: Confirm red**

Run: `go test ./internal/identity -run TestBindReturnsOneUserForRepeatedIssuerSubject -count=1`

Expected: FAIL because the migration, repository, and `Bind` service do not exist.

- [ ] **Step 3: Add the immutable identity schema**

```sql
CREATE TABLE platform_users (
  id uuid PRIMARY KEY,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE identity_bindings (
  identity_provider text NOT NULL,
  subject text NOT NULL,
  platform_user_id uuid NOT NULL REFERENCES platform_users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (identity_provider, subject),
  UNIQUE (platform_user_id, identity_provider)
);
```

Do not add email, phone, username, `organization_id`, or a cascading foreign key to Keycloak.

- [ ] **Step 4: Implement transactional bind-or-load**

```go
type VerifiedIdentity struct { Issuer, Subject string }
type User struct { ID string }

func (s *Service) Bind(ctx context.Context, verified VerifiedIdentity) (User, error) {
    if verified.Issuer == "" || verified.Subject == "" {
        return User{}, ErrUnverifiedIdentity
    }
    return s.repository.BindOrLoad(ctx, verified.Issuer, verified.Subject)
}
```

`BindOrLoad` inserts a generated User and binding in one transaction, handles the `(identity_provider, subject)` conflict by loading the existing User, and never retries with a mutable claim.

- [ ] **Step 5: Add the verified-context-only HTTP handler**

```go
func (h Handler) Callback(w http.ResponseWriter, r *http.Request) {
    verified, ok := oidcidentity.FromContext(r.Context())
    if !ok {
        http.Error(w, "verified identity required", http.StatusUnauthorized)
        return
    }
    user, err := h.service.Bind(r.Context(), VerifiedIdentity{Issuer: verified.Issuer, Subject: verified.Subject})
    if err != nil { h.writeError(w, err); return }
    h.writeJSON(w, http.StatusCreated, map[string]string{"user_id": user.ID})
}
```

- [ ] **Step 6: Run focused and PostgreSQL integration tests**

Run: `go test ./internal/identity -count=1`

Expected: PASS for first bind, duplicate bind, same subject under different issuer, unverified request denial, and mutable profile change preserving the same User.

- [ ] **Step 7: Commit the binding slice**

```bash
git add db/migrations/000001_identity_bindings.sql internal/identity
git commit -m "feat(identity): bind verified Keycloak subjects"
```

## Self-Review Record

- Spec coverage: verified OIDC origin, stable issuer/subject key, idempotency, denial, and non-cascading lifecycle are explicit.
- Placeholder scan: schema, signatures, handler behavior, commands, and expected cases are concrete.
- Type consistency: `VerifiedIdentity`, `User`, `Bind`, repository key, and handler context use the same issuer/subject pair.
- Right-sizing: one persistence/API slice; shared scaffold and harness remain owned by #100.
