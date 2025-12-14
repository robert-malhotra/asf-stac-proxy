package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// RequestIDHeader is the header name for request ID in responses.
const RequestIDHeader = "X-Request-ID"

// requestIDKey is the context key for storing request ID.
type requestIDKey struct{}

// GetRequestID returns the request ID from the context, or empty string if not present.
func GetRequestID(ctx context.Context) string {
	// First try chi's middleware
	if reqID := middleware.GetReqID(ctx); reqID != "" {
		return reqID
	}
	// Fallback to our own context key
	if reqID, ok := ctx.Value(requestIDKey{}).(string); ok {
		return reqID
	}
	return ""
}

// RequestIDResponse adds the X-Request-ID header to the response.
// This should be placed after chi's middleware.RequestID middleware.
func RequestIDResponse(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := middleware.GetReqID(r.Context())
		if reqID != "" {
			w.Header().Set(RequestIDHeader, reqID)
		}
		next.ServeHTTP(w, r)
	})
}

// RequestLogger creates a middleware that logs HTTP requests using structured logging.
func RequestLogger(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap the response writer to capture status code
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			// Process request
			next.ServeHTTP(ww, r)

			// Log after request completes
			duration := time.Since(start)

			// Get request ID from context
			reqID := GetRequestID(r.Context())

			logger.Info("http request",
				slog.String("request_id", reqID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("query", r.URL.RawQuery),
				slog.String("remote_addr", r.RemoteAddr),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", duration),
				slog.String("user_agent", r.UserAgent()),
			)
		})
	}
}

// ContentTypeJSON sets the Content-Type header to application/json for all responses.
// Individual handlers may override this if needed (e.g., for GeoJSON).
func ContentTypeJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set default content type, handlers can override
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// Recovery recovers from panics and returns a 500 error.
func Recovery(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					// Safely extract error message from recover() which can return any type
					var errStr string
					switch v := rec.(type) {
					case error:
						errStr = v.Error()
					case string:
						errStr = v
					default:
						errStr = fmt.Sprintf("%v", v)
					}

					reqID := GetRequestID(r.Context())

					logger.Error("panic recovered",
						slog.String("request_id", reqID),
						slog.String("error", errStr),
						slog.String("path", r.URL.Path),
						slog.String("method", r.Method),
					)

					WriteInternalErrorWithRequestID(w, "internal server error", reqID)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
