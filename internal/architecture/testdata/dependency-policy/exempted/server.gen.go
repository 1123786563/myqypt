package exempted

// Exemption fixture: this file imports gorm.io/gorm but lives at a path that
// scans pass as an explicit exemption entry (the shape of
// internal/transport/http/api/server.gen.go, design ruling 5). With the
// exemption list containing "exempted/server.gen.go" it must not be flagged;
// without the exemption it must be flagged like any other file.
import _ "gorm.io/gorm"
