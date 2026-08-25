package main

import (
	"reflect"
	"testing"

	"github.com/1123786563/myqypt/internal/platform/observability"
	"github.com/1123786563/myqypt/internal/transport/http/middleware"
)

// TestObservabilityConfigDefaults covers the empty-environment path: unset
// variables fall back to the platform defaults (design ruling 10 — the
// defaults live in the composition root, never in the observability
// package), with no OTLP endpoint so a bare local run stays network-free.
func TestObservabilityConfigDefaults(t *testing.T) {
	t.Setenv("OTEL_SERVICE_NAME", "")
	t.Setenv("PLATFORM_DEPLOYMENT_ENVIRONMENT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	want := observability.Config{
		ServiceName:           "platform-api",
		ServiceVersion:        version,
		DeploymentEnvironment: "development",
		OTLPEndpoint:          "",
	}
	if got := observabilityConfig(); got != want {
		t.Fatalf("observabilityConfig() = %+v, want %+v", got, want)
	}
}

// TestObservabilityConfigFromEnvironment covers set values: every variable
// is passed through verbatim.
func TestObservabilityConfigFromEnvironment(t *testing.T) {
	t.Setenv("OTEL_SERVICE_NAME", "custom-service")
	t.Setenv("PLATFORM_DEPLOYMENT_ENVIRONMENT", "production")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "127.0.0.1:14317")

	want := observability.Config{
		ServiceName:           "custom-service",
		ServiceVersion:        version,
		DeploymentEnvironment: "production",
		OTLPEndpoint:          "127.0.0.1:14317",
	}
	if got := observabilityConfig(); got != want {
		t.Fatalf("observabilityConfig() = %+v, want %+v", got, want)
	}
}

// TestSecurityConfigDefaults covers the fail-closed default: an unset or
// blank PLATFORM_API_ALLOWED_ORIGINS yields an empty origin allowlist, so
// the security headers stay on and no origin receives a CORS grant.
func TestSecurityConfigDefaults(t *testing.T) {
	t.Setenv("PLATFORM_API_ALLOWED_ORIGINS", "")

	config := securityConfig()
	if config == nil {
		t.Fatal("securityConfig() = nil, want a non-nil config")
	}
	if len(config.AllowedOrigins) != 0 {
		t.Fatalf("securityConfig().AllowedOrigins = %v, want empty", config.AllowedOrigins)
	}
	if config.AllowCredentials {
		t.Fatal("securityConfig().AllowCredentials = true, want false")
	}
}

// TestSecurityConfigFromEnvironment covers the allowlist path: a set
// variable is parsed into the exact trimmed origin list.
func TestSecurityConfigFromEnvironment(t *testing.T) {
	t.Setenv("PLATFORM_API_ALLOWED_ORIGINS", "http://allowed.example")

	config := securityConfig()
	if !reflect.DeepEqual(config.AllowedOrigins, []string{"http://allowed.example"}) {
		t.Fatalf("securityConfig().AllowedOrigins = %v, want [http://allowed.example]", config.AllowedOrigins)
	}
}

func TestParseAllowedOrigins(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"blank", "   ", nil},
		{"single origin", "http://allowed.example", []string{"http://allowed.example"}},
		{"multiple origins", "http://a.example,https://b.example", []string{"http://a.example", "https://b.example"}},
		{"trims surrounding whitespace", " http://a.example , https://b.example ", []string{"http://a.example", "https://b.example"}},
		{"drops empty entries", ",http://a.example,,,", []string{"http://a.example"}},
		{"only separators", " , , ,", nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := parseAllowedOrigins(testCase.raw); !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("parseAllowedOrigins(%q) = %v, want %v", testCase.raw, got, testCase.want)
			}
		})
	}
}

// TestSecurityConfigMiddlewareCompatible is a wiring guard: the config the
// composition root builds must be accepted by the middleware constructor —
// in particular the default (empty allowlist) build never trips the
// wildcard-plus-credentials construction panic.
func TestSecurityConfigMiddlewareCompatible(t *testing.T) {
	t.Setenv("PLATFORM_API_ALLOWED_ORIGINS", "")
	config := securityConfig()
	handler := middleware.Security(*config)
	if handler == nil {
		t.Fatal("middleware.Security(securityConfig()) = nil")
	}
}
