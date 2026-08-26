package fixtures

// Fixture for ARCH-GORM: importing the gorm.io/gorm module is forbidden
// (Issue #105 implementation plan, design ruling 3). testdata directories are
// never compiled by the Go toolchain; this file exists only as scanner input.
import _ "gorm.io/gorm"
