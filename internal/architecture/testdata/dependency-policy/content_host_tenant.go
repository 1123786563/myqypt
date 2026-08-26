package fixtures

// Fixture for ARCH-HOST-TENANT: host-derived tenant resolution is forbidden
// in production sources (Issue #105 implementation plan, design ruling 4);
// the local identifier below matches the rule's identifier pattern.
func fixtureHostEcho(host string) string {
	tenantFromHost := host
	return tenantFromHost
}
