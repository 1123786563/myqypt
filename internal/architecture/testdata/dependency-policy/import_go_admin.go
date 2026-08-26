package fixtures

// Fixture for ARCH-GO-ADMIN: importing any subpath of
// github.com/go-admin-team/go-admin — here the upstream common/global
// runtime package named by design ruling 3 — is forbidden.
import _ "github.com/go-admin-team/go-admin/common/global"
