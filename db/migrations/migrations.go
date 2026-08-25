// Package migrations embeds the goose SQL migrations for the platform
// database. Migrations are append-only: every version pairs an Up and a Down
// section and is applied exclusively through goose's version table.
package migrations

import "embed"

// FS holds every SQL migration file in this directory.
//
//go:embed *.sql
var FS embed.FS
