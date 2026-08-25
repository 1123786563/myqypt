package observability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/1123786563/myqypt/internal/platform/observability"
	httptransport "github.com/1123786563/myqypt/internal/transport/http"
	"github.com/gin-gonic/gin"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Credential markers sent as real header and body values on every test
// request; every artifact the pipeline produces (log records, span
// attributes, event names) must keep them out.
const (
	bearerMarker = "Bearer marker-auth-XYZ"
	cookieMarker = "session=marker-cookie-XYZ"
	bodyMarker   = "payload=marker-body-XYZ"

	bearerSecret = "marker-auth-XYZ"
	cookieSecret = "marker-cookie-XYZ"
	bodySecret   = "marker-body-XYZ"
)

func useTestGinMode(t *testing.T) {
	t.Helper()
	original := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		gin.SetMode(original)
	})
}

func testConfig(writer *bytes.Buffer) observability.Config {
	return observability.Config{
		ServiceName:           "platform-api-test",
		ServiceVersion:        "vtest",
		DeploymentEnvironment: "test",
		OTLPEndpoint:          "",
		LogWriter:             writer,
	}
}

// assertNoCredentialMarkers fails when any credential marker value appears
// in the produced text.
func assertNoCredentialMarkers(t *testing.T, what, text string) {
	t.Helper()
	for _, secret := range []string{bearerSecret, cookieSecret, bodySecret} {
		if strings.Contains(text, secret) {
			t.Fatalf("%s leaked credential marker %q: %q", what, secret, text)
		}
	}
}

// TestObservabilityNoEndpointIsNoop covers the default path: without an OTLP
// endpoint, New still returns a usable JSON logger and real SDK providers
// with no exporters, and Shutdown is idempotent — a second call is side
// effect free (guarded by sync.Once inside the implementation).
func TestObservabilityNoEndpointIsNoop(t *testing.T) {
	var buf bytes.Buffer
	resources, err := observability.New(context.Background(), testConfig(&buf))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if resources.Logger == nil || resources.TracerProvider == nil || resources.MeterProvider == nil || resources.Shutdown == nil {
		t.Fatalf("New() returned nil resource(s): logger=%v tracer=%v meter=%v shutdown=%t",
			resources.Logger, resources.TracerProvider, resources.MeterProvider, resources.Shutdown == nil)
	}

	// The logger must be a JSON handler writing to the injected writer.
	resources.Logger.Info("probe", "key", "value")
	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("logger output is not one JSON record: %v\noutput=%q", err, buf.String())
	}
	if record["msg"] != "probe" || record["key"] != "value" {
		t.Fatalf("logger JSON record = %v", record)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := resources.Shutdown(ctx); err != nil {
		t.Fatalf("first Shutdown() error = %v", err)
	}
	if err := resources.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
}

// TestObservabilityResourceAttributes asserts the identity resource carries
// the injected Config values verbatim.
func TestObservabilityResourceAttributes(t *testing.T) {
	resources, err := observability.New(context.Background(), testConfig(&bytes.Buffer{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	assertResourceAttributes(t, resources, map[string]string{
		"service.name":           "platform-api-test",
		"service.version":        "vtest",
		"deployment.environment": "test",
	})
}

// TestObservabilityEmptyConfigPassesThrough asserts the package injects no
// identity defaults of its own: an empty Config yields empty attributes,
// because the platform defaults (service name, deployment environment) are
// the composition root's responsibility — see the cmd/platform-api config
// tests — keeping this package environment-free per design ruling 10.
func TestObservabilityEmptyConfigPassesThrough(t *testing.T) {
	resources, err := observability.New(context.Background(), observability.Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	assertResourceAttributes(t, resources, map[string]string{
		"service.name":           "",
		"service.version":        "",
		"deployment.environment": "",
	})
}

func assertResourceAttributes(t *testing.T, resources observability.Resources, want map[string]string) {
	t.Helper()
	attrs := map[string]string{}
	for _, kv := range resources.Resource.Attributes() {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}
	for key, wantValue := range want {
		if got := attrs[key]; got != wantValue {
			t.Fatalf("resource attribute %s = %q, want %q (all: %v)", key, got, wantValue, attrs)
		}
	}
}

// TestObservabilityOTLPEndpointConstructs covers the endpoint path: New must
// construct the OTLP gRPC exporters without contacting a collector (none is
// running on the endpoint), and the shutdown path must stay callable and
// idempotent — a second call returns the exact same result as the first,
// proving the shutdown body ran only once.
func TestObservabilityOTLPEndpointConstructs(t *testing.T) {
	config := testConfig(&bytes.Buffer{})
	config.OTLPEndpoint = "127.0.0.1:19001"
	resources, err := observability.New(context.Background(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if resources.Logger == nil || resources.TracerProvider == nil || resources.MeterProvider == nil {
		t.Fatalf("New() returned nil resource(s)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	firstErr := resources.Shutdown(ctx)
	secondErr := resources.Shutdown(ctx)
	t.Logf("shutdown with endpoint and no collector: first=%v second=%v", firstErr, secondErr)
	if secondErr != firstErr {
		t.Fatalf("Shutdown() not idempotent: first=%v second=%v", firstErr, secondErr)
	}
}

// TestObservabilityAccessLogFieldsViaRouter drives one request carrying all
// three credential markers through the full NewRouter assembly wired with
// observability resources and asserts the access-log JSON record carries
// the complete allowlisted field set — including trace_id — while no marker
// value ever appears.
func TestObservabilityAccessLogFieldsViaRouter(t *testing.T) {
	useTestGinMode(t)
	var buf bytes.Buffer
	resources, err := observability.New(context.Background(), testConfig(&buf))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	router := httptransport.NewRouter(httptransport.Dependencies{
		Version:        "test",
		Logger:         resources.Logger,
		TracerProvider: resources.TracerProvider,
	})

	request := httptest.NewRequest(http.MethodGet, "/livez", strings.NewReader(bodyMarker))
	request.Header.Set("X-Request-ID", "obs-log-9")
	request.Header.Set("Authorization", bearerMarker)
	request.Header.Set("Cookie", cookieMarker)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	assertNoCredentialMarkers(t, "access log", buf.String())

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record); err != nil {
		t.Fatalf("access log is not one JSON record: %v\nlog=%q", err, buf.String())
	}
	if got := record["method"]; got != http.MethodGet {
		t.Fatalf("method = %v, want GET (record=%v)", got, record)
	}
	if got := record["path"]; got != "/livez" {
		t.Fatalf("path = %v, want /livez (record=%v)", got, record)
	}
	if got := record["status"]; got != float64(http.StatusOK) {
		t.Fatalf("status = %v, want 200 (record=%v)", got, record)
	}
	if got, ok := record["duration_ms"].(float64); !ok || got < 0 {
		t.Fatalf("duration_ms = %v, want a non-negative number (record=%v)", record["duration_ms"], record)
	}
	if got := record["request_id"]; got != "obs-log-9" {
		t.Fatalf("request_id = %v, want obs-log-9 (record=%v)", got, record)
	}
	traceID, ok := record["trace_id"].(string)
	if !ok || len(traceID) != 32 {
		t.Fatalf("trace_id = %v, want a 32-char trace ID (record=%v)", record["trace_id"], record)
	}
}

// firstAttr returns the value of the first present key among candidates.
func firstAttr(attrs map[string]string, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := attrs[key]; ok {
			return value, true
		}
	}
	return "", false
}

// TestObservabilitySpanAttributesAndCorrelation asserts the tracing side: a
// request through the router produces a span carrying the HTTP method, route
// and status attributes with no credential values in attributes or event
// names, and the access-log record's trace_id equals that span's trace ID —
// correlating request ID, trace and log line.
func TestObservabilitySpanAttributesAndCorrelation(t *testing.T) {
	useTestGinMode(t)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	recorder := tracetest.NewSpanRecorder()
	router := httptransport.NewRouter(httptransport.Dependencies{
		Version: "test",
		Routes: func(e *gin.Engine) {
			e.GET("/obs-span", func(c *gin.Context) { c.Status(http.StatusTeapot) })
		},
		Logger:         logger,
		TracerProvider: sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)),
	})

	request := httptest.NewRequest(http.MethodGet, "/obs-span", nil)
	request.Header.Set("X-Request-ID", "obs-span-4")
	request.Header.Set("Authorization", bearerMarker)
	request.Header.Set("Cookie", cookieMarker)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusTeapot {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}

	ended := recorder.Ended()
	if len(ended) == 0 {
		t.Fatal("no span recorded for the request")
	}
	span := ended[len(ended)-1]
	attrs := map[string]string{}
	for _, kv := range span.Attributes() {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}
	t.Logf("span name=%q attributes=%v", span.Name(), attrs)
	// otelgin emits semconv HTTP attributes; accept either the current or
	// the legacy spelling of method and status.
	if got, ok := firstAttr(attrs, "http.request.method", "http.method"); !ok || got != http.MethodGet {
		t.Fatalf("span http method attribute = %q, want GET (attrs=%v)", got, attrs)
	}
	if got, ok := attrs["http.route"]; !ok || got != "/obs-span" {
		t.Fatalf("span http.route = %q, want /obs-span (attrs=%v)", attrs["http.route"], attrs)
	}
	if got, ok := firstAttr(attrs, "http.response.status_code", "http.status_code"); !ok || got != "418" {
		t.Fatalf("span http status attribute = %q, want 418 (attrs=%v)", got, attrs)
	}
	for key, value := range attrs {
		if strings.Contains(value, bearerSecret) || strings.Contains(value, cookieSecret) {
			t.Fatalf("span attribute %s leaks credential marker: %q", key, value)
		}
	}
	for _, event := range span.Events() {
		if strings.Contains(event.Name, bearerSecret) || strings.Contains(event.Name, cookieSecret) {
			t.Fatalf("span event name leaks credential marker: %q", event.Name)
		}
	}

	// Correlation: the access-log record pairs the request ID with the span
	// trace ID.
	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record); err != nil {
		t.Fatalf("access log is not one JSON record: %v\nlog=%q", err, buf.String())
	}
	if record["request_id"] != "obs-span-4" {
		t.Fatalf("access log request_id = %v (record=%v)", record["request_id"], record)
	}
	if record["trace_id"] != span.SpanContext().TraceID().String() {
		t.Fatalf("access log trace_id = %v, want span trace ID %s",
			record["trace_id"], span.SpanContext().TraceID().String())
	}
}
