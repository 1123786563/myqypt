package httptransport_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	httptransport "github.com/1123786563/myqypt/internal/transport/http"
	"github.com/1123786563/myqypt/internal/transport/http/api"
	"github.com/getkin/kin-openapi/openapi3"
)

// systemStatusResponseSchema loads the embedded OpenAPI document and returns
// the response schema for GET /api/v1/system/status 200 application/json.
// Navigation goes through Paths rather than operationId because the embedded
// document is the source of truth for the wire contract, independent of how
// the generator normalizes identifiers.
func systemStatusResponseSchema(t *testing.T) *openapi3.Schema {
	t.Helper()
	swagger, err := api.GetSwagger()
	if err != nil {
		t.Fatalf("load embedded spec: %v", err)
	}
	pathItem := swagger.Paths.Find("/api/v1/system/status")
	if pathItem == nil || pathItem.Get == nil {
		t.Fatalf("embedded spec missing GET /api/v1/system/status")
	}
	response := pathItem.Get.Responses.Value("200")
	if response == nil || response.Value == nil {
		t.Fatalf("embedded spec missing 200 response for system status")
	}
	mediaType := response.Value.Content.Get("application/json")
	if mediaType == nil || mediaType.Schema == nil {
		t.Fatalf("embedded 200 response missing application/json schema")
	}
	return mediaType.Schema.Value
}

// validateAgainstContract decodes payload as generic JSON — the same shape a
// real client parses — and validates it against an embedded-contract schema.
// The returned error is exactly what the divergence detector reports.
func validateAgainstContract(t *testing.T, schema *openapi3.Schema, payload string) error {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(payload), &value); err != nil {
		t.Fatalf("decode payload %q: %v", payload, err)
	}
	return schema.VisitJSON(value)
}

// TestContractSystemStatusResponseMatchesEmbeddedSpec serves a real request
// through the production router and validates the response body against the
// SystemStatus schema taken from the embedded spec: existence, types,
// required fields, additionalProperties and the status const.
func TestContractSystemStatusResponseMatchesEmbeddedSpec(t *testing.T) {
	useTestGinMode(t)
	router := httptransport.NewRouter(httptransport.Dependencies{Version: "be4cc10"})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	schema := systemStatusResponseSchema(t)
	if err := validateAgainstContract(t, schema, response.Body.String()); err != nil {
		t.Fatalf("real response violates embedded contract: %v", err)
	}
}

// TestContractDetectorRejectsDivergentPayloads is the negative control for
// the divergence detector: hand-built payloads that a drifted implementation
// could plausibly emit must be rejected by the very same validation that
// accepted the real response. If any of these passed, the detector would be
// blind to implementation/contract drift.
func TestContractDetectorRejectsDivergentPayloads(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{"missing required version", `{"status":"available"}`},
		{"unknown extra field", `{"status":"available","version":"be4cc10","request_id":"abc"}`},
		{"status violates const", `{"status":"dead","version":"be4cc10"}`},
		{"empty version violates minLength", `{"status":"available","version":""}`},
	}
	schema := systemStatusResponseSchema(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateAgainstContract(t, schema, tc.payload); err == nil {
				t.Fatalf("divergent payload %q accepted by contract validation", tc.payload)
			}
		})
	}

	// Control for the control: a payload that conforms to the schema must be
	// accepted, so the rejections above detect divergence, not everything.
	t.Run("conforming payload is accepted", func(t *testing.T) {
		if err := validateAgainstContract(t, schema, `{"status":"available","version":"be4cc10"}`); err != nil {
			t.Fatalf("conforming payload rejected: %v", err)
		}
	})
}
