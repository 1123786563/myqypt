package fixtures

// Fixture for ARCH-DEFAULT-CREDENTIALS: seeded default credentials are
// forbidden in production sources (Issue #105 implementation plan, design
// ruling 4). This line matches both the credential substring rule and the
// default-credential identifier rule.
const DefaultPassword = "admin123"
