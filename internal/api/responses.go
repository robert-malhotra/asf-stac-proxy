// Package api provides HTTP handlers and routing for the ASF-STAC proxy service.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// STACError represents a STAC-compliant error response.
type STACError struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

// Standard STAC error codes.
const (
	ErrCodeBadRequest       = "BadRequest"
	ErrCodeNotFound         = "NotFound"
	ErrCodeInvalidParameter = "InvalidParameterValue"
	ErrCodeServerError      = "ServerError"
	ErrCodeUpstreamError    = "UpstreamServiceError"
)

// WriteJSON writes a JSON response with the given status code and value.
// If encoding fails, it logs the error and returns an internal server error.
func WriteJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response",
			slog.String("error", err.Error()),
		)
		return err
	}

	return nil
}

// WriteGeoJSON writes a GeoJSON response with the given status code and value.
// GeoJSON responses use the application/geo+json media type.
func WriteGeoJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/geo+json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode GeoJSON response",
			slog.String("error", err.Error()),
		)
		return err
	}

	return nil
}

// WriteError writes a STAC-compliant error response.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	errResp := STACError{
		Code:        code,
		Description: message,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(errResp); err != nil {
		slog.Error("failed to encode error response",
			slog.String("error", err.Error()),
		)
	}
}

// WriteBadRequest writes a 400 Bad Request error response.
func WriteBadRequest(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadRequest, ErrCodeBadRequest, message)
}

// WriteNotFound writes a 404 Not Found error response.
func WriteNotFound(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusNotFound, ErrCodeNotFound, message)
}

// WriteInvalidParameter writes a 400 Bad Request error for invalid parameters.
func WriteInvalidParameter(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadRequest, ErrCodeInvalidParameter, message)
}

// WriteInternalError writes a 500 Internal Server Error response.
func WriteInternalError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusInternalServerError, ErrCodeServerError, message)
}

// WriteUpstreamError writes a 502 Bad Gateway error for upstream service failures.
func WriteUpstreamError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadGateway, ErrCodeUpstreamError, message)
}
