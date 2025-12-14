package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

func TestRecovery_PanicWithError(t *testing.T) {
	// Test that Recovery middleware properly handles panic with error type
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	handler := Recovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp["code"] != "ServerError" {
		t.Errorf("Expected code 'ServerError', got %v", resp["code"])
	}
}

func TestRecovery_PanicWithString(t *testing.T) {
	// Test that Recovery middleware properly handles panic with string type
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	handler := Recovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went wrong")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}

func TestRecovery_PanicWithInt(t *testing.T) {
	// Test that Recovery middleware properly handles panic with arbitrary type (int)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	handler := Recovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(42)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}

func TestRecovery_NoPanic(t *testing.T) {
	// Test that Recovery middleware passes through normal requests
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	handler := Recovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "OK" {
		t.Errorf("Expected body 'OK', got %s", w.Body.String())
	}
}

func TestRecovery_LogsError(t *testing.T) {
	// Test that Recovery middleware logs the panic
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	handler := Recovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic message")
	}))

	req := httptest.NewRequest("GET", "/test-path", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "panic recovered") {
		t.Errorf("Expected log to contain 'panic recovered', got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "test panic message") {
		t.Errorf("Expected log to contain panic message, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "/test-path") {
		t.Errorf("Expected log to contain path, got: %s", logOutput)
	}
}

func TestContentTypeJSON(t *testing.T) {
	handler := ContentTypeJSON(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got %s", contentType)
	}
}

func TestContentTypeJSON_CanBeOverridden(t *testing.T) {
	handler := ContentTypeJSON(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/geo+json" {
		t.Errorf("Expected Content-Type 'application/geo+json', got %s", contentType)
	}
}

func TestRequestLogger(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	handler := RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))

	req := httptest.NewRequest("POST", "/api/search?foo=bar", nil)
	req.Header.Set("User-Agent", "test-agent")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	logOutput := logBuf.String()

	// Check for expected log fields
	expectedFields := []string{
		"http request",
		"method=POST",
		"path=/api/search",
		"status=200",
		"user_agent=test-agent",
	}

	for _, field := range expectedFields {
		if !strings.Contains(logOutput, field) {
			t.Errorf("Expected log to contain '%s', got: %s", field, logOutput)
		}
	}
}

func TestRequestLogger_CapturesDuration(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	handler := RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "duration=") {
		t.Errorf("Expected log to contain duration field, got: %s", logOutput)
	}
}

func TestRequestLogger_CapturesStatusCode(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	handler := RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest("GET", "/not-found", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "status=404") {
		t.Errorf("Expected log to contain 'status=404', got: %s", logOutput)
	}
}

func TestRequestIDResponse(t *testing.T) {
	// Create a handler chain: RequestID -> RequestIDResponse -> handler
	handler := middleware.RequestID(RequestIDResponse(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Check that X-Request-ID header is set in response
	reqID := w.Header().Get("X-Request-ID")
	if reqID == "" {
		t.Error("Expected X-Request-ID header to be set in response")
	}
}

func TestRequestIDResponse_WithExistingHeader(t *testing.T) {
	// Create a handler chain: RequestID -> RequestIDResponse -> handler
	handler := middleware.RequestID(RequestIDResponse(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-Id", "custom-request-id-123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Check that the incoming request ID is used
	reqID := w.Header().Get("X-Request-ID")
	if reqID != "custom-request-id-123" {
		t.Errorf("Expected X-Request-ID to be 'custom-request-id-123', got %s", reqID)
	}
}

func TestGetRequestID_WithChiMiddleware(t *testing.T) {
	var capturedReqID string

	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReqID = GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if capturedReqID == "" {
		t.Error("Expected GetRequestID to return non-empty value when using chi middleware")
	}
}

func TestGetRequestID_EmptyContext(t *testing.T) {
	ctx := context.Background()
	reqID := GetRequestID(ctx)

	if reqID != "" {
		t.Errorf("Expected empty request ID for empty context, got %s", reqID)
	}
}

func TestRequestLogger_IncludesRequestID(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	// Chain: RequestID -> RequestLogger -> handler
	handler := middleware.RequestID(RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "request_id=") {
		t.Errorf("Expected log to contain 'request_id=', got: %s", logOutput)
	}
}

func TestRecovery_IncludesRequestIDInResponse(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Chain: RequestID -> Recovery -> handler that panics
	handler := middleware.RequestID(Recovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-Id", "test-req-123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var resp STACError
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.RequestID != "test-req-123" {
		t.Errorf("Expected request_id 'test-req-123' in error response, got %s", resp.RequestID)
	}
}
