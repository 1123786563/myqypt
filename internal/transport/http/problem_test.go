package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/1123786563/myqypt/internal/transport/http/api"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
)

// wireProblem mirrors the public application/problem+json shape. It is
// declared locally so the json field names below are an independent source of
// truth, not the production struct tags.
type wireProblem struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Status  int    `json:"status"`
	Code    string `json:"code"`
	TraceID string `json:"trace_id"`
}

func useTestGinMode(t *testing.T) {
	t.Helper()
	original := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		gin.SetMode(original)
	})
}

func assertProblemResponse(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) wireProblem {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status=%d want=%d body=%q", rec.Code, wantStatus, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("content-type=%q want %q", got, "application/problem+json")
	}
	var p wireProblem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode problem body %q: %v", rec.Body.String(), err)
	}
	if p.Status != wantStatus {
		t.Fatalf("problem.status=%d want=%d body=%q", p.Status, wantStatus, rec.Body.String())
	}
	if p.Code != wantCode {
		t.Fatalf("problem.code=%q want=%q", p.Code, wantCode)
	}
	if wantType := "https://api.myqypt.dev/problems/" + wantCode; p.Type != wantType {
		t.Fatalf("problem.type=%q want=%q", p.Type, wantType)
	}
	if p.Title == "" {
		t.Fatalf("problem.title empty body=%q", rec.Body.String())
	}
	if p.TraceID == "" {
		t.Fatalf("problem.trace_id empty body=%q", rec.Body.String())
	}
	if got := rec.Header().Get("X-Request-ID"); got == "" || got != p.TraceID {
		t.Fatalf("X-Request-ID header=%q want to match trace_id=%q", got, p.TraceID)
	}
	return p
}

func TestProblemDetailsForMethodNotAllowed(t *testing.T) {
	useTestGinMode(t)
	router := NewRouter(Dependencies{Version: "test"})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/status", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assertProblemResponse(t, response, http.StatusMethodNotAllowed, "method_not_allowed")
}

func TestProblemDetailsForNotFound(t *testing.T) {
	useTestGinMode(t)
	router := NewRouter(Dependencies{Version: "test"})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assertProblemResponse(t, response, http.StatusNotFound, "not_found")
}

func TestProblemDetailsReuseInboundRequestID(t *testing.T) {
	useTestGinMode(t)
	router := NewRouter(Dependencies{Version: "test"})

	requests := []struct {
		name   string
		method string
		path   string
		status int
		code   string
	}{
		{"method not allowed", http.MethodPost, "/api/v1/system/status", http.StatusMethodNotAllowed, "method_not_allowed"},
		{"not found", http.MethodGet, "/api/v1/does-not-exist", http.StatusNotFound, "not_found"},
	}
	for _, tc := range requests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, nil)
			request.Header.Set("X-Request-ID", "test-trace-42")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			p := assertProblemResponse(t, response, tc.status, tc.code)
			if p.TraceID != "test-trace-42" {
				t.Fatalf("trace_id=%q want reused inbound %q", p.TraceID, "test-trace-42")
			}
			if got := response.Header().Get("X-Request-ID"); got != "test-trace-42" {
				t.Fatalf("X-Request-ID header=%q want echoed %q", got, "test-trace-42")
			}
		})
	}
}

// probeSpec is a tiny inline contract that — unlike the shipped contract —
// declares a required query parameter, so a genuinely invalid request can be
// sent through the production validator wiring.
const probeSpec = `openapi: 3.1.0
info:
  title: validator probe
  version: 1.0.0
paths:
  /probe:
    get:
      operationId: getProbe
      parameters:
        - name: region
          in: query
          required: true
          schema:
            type: string
      responses:
        "200":
          description: Probe acknowledged.
`

func TestValidatorRejectsInvalidRequestThroughProductionWiring(t *testing.T) {
	useTestGinMode(t)
	doc, err := openapi3.NewLoader().LoadFromIoReader(bytes.NewReader([]byte(probeSpec)))
	if err != nil {
		t.Fatalf("load probe spec: %v", err)
	}

	engine := gin.New()
	engine.Use(RequestID())
	probe := engine.Group("/", openAPIValidatorMiddleware(doc))
	probe.GET("/probe", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Control: a valid request passes the production validator untouched.
	valid := httptest.NewRequest(http.MethodGet, "/probe?region=eu-1", nil)
	validResponse := httptest.NewRecorder()
	engine.ServeHTTP(validResponse, valid)
	if validResponse.Code != http.StatusOK {
		t.Fatalf("valid request status=%d body=%q", validResponse.Code, validResponse.Body.String())
	}

	// Missing the required region query parameter must be rejected by the
	// same validator middleware the production router installs, mapped to a
	// 400 invalid_request problem.
	invalid := httptest.NewRequest(http.MethodGet, "/probe", nil)
	invalidResponse := httptest.NewRecorder()
	engine.ServeHTTP(invalidResponse, invalid)

	assertProblemResponse(t, invalidResponse, http.StatusBadRequest, "invalid_request")
}

func TestStrictRequestAndResponseErrorHooksWriteProblems(t *testing.T) {
	useTestGinMode(t)
	options := strictServerOptions()

	cases := []struct {
		name     string
		invoke   func(c *gin.Context, err error)
		status   int
		code     string
		secretEr string
	}{
		{
			name:     "request error hook",
			invoke:   options.RequestErrorHandlerFunc,
			status:   http.StatusBadRequest,
			code:     "invalid_request",
			secretEr: "secret bind failure",
		},
		{
			name:     "response error hook",
			invoke:   options.ResponseErrorHandlerFunc,
			status:   http.StatusInternalServerError,
			code:     "internal_error",
			secretEr: "secret serialization failure",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			// Mirror the two side effects of the RequestID middleware so the
			// hooks run in the environment the real handler chain provides.
			c.Set(requestIDContextKey, "hook-trace-7")
			c.Header(HeaderRequestID, "hook-trace-7")

			tc.invoke(c, errors.New(tc.secretEr))

			p := assertProblemResponse(t, response, tc.status, tc.code)
			if p.TraceID != "hook-trace-7" {
				t.Fatalf("trace_id=%q want %q", p.TraceID, "hook-trace-7")
			}
			if body := response.Body.String(); bytes.Contains([]byte(body), []byte(tc.secretEr)) {
				t.Fatalf("problem body leaks internal error %q: %s", tc.secretEr, body)
			}
		})
	}
}

// erroringStatusHandler implements api.StrictServerInterface by failing, to
// exercise the strict handler error hook end-to-end through the generated
// strict machinery.
type erroringStatusHandler struct {
	failure string
}

func (h erroringStatusHandler) GetSystemStatus(context.Context, api.GetSystemStatusRequestObject) (api.GetSystemStatusResponseObject, error) {
	return nil, errors.New(h.failure)
}

func TestStrictHandlerErrorWritesInternalErrorProblem(t *testing.T) {
	useTestGinMode(t)
	engine := gin.New()
	engine.Use(RequestID())
	api.RegisterHandlersWithOptions(
		engine,
		api.NewStrictHandlerWithOptions(erroringStatusHandler{failure: "secret internal failure"}, nil, strictServerOptions()),
		api.GinServerOptions{},
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	assertProblemResponse(t, response, http.StatusInternalServerError, "internal_error")
	if body := response.Body.String(); bytes.Contains([]byte(body), []byte("secret internal failure")) {
		t.Fatalf("problem body leaks internal error: %s", body)
	}
}
