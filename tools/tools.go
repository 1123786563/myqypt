//go:build tools

// Package tools pins development tool dependencies in go.mod so that
// go:generate directives can invoke them with `go run` and no @version
// suffix, keeping the toolchain reproducible.
package tools

import (
	_ "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen"
)
