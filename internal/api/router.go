package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// NewRouter creates and configures the HTTP router with all routes and middleware.
func NewRouter(h *Handlers, logger *slog.Logger) chi.Router {
	r := chi.NewRouter()

	// Add middleware stack
	r.Use(middleware.RequestID)
	r.Use(RequestIDResponse) // Add X-Request-ID to response headers
	r.Use(middleware.RealIP)
	r.Use(RequestLogger(logger))
	r.Use(Recovery(logger))
	r.Use(middleware.Compress(5)) // Gzip compression
	r.Use(ContentTypeJSON)

	// CORS configuration
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"}, // Allow all origins for STAC API
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "Content-Length"},
		ExposedHeaders:   []string{"Link", "X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300, // 5 minutes
	}))

	// Health check endpoint (before any other middleware)
	r.Get("/health", h.Health)

	// STAC API routes

	// Landing page
	r.Get("/", h.LandingPage)

	// Conformance
	r.Get("/conformance", h.Conformance)

	// Collections
	r.Get("/collections", h.Collections)
	r.Get("/collections/{collectionId}", h.Collection)

	// Items
	r.Get("/collections/{collectionId}/items", h.Items)
	r.Get("/collections/{collectionId}/items/{itemId}", h.Item)

	// Search endpoint
	r.Route("/search", func(r chi.Router) {
		r.Get("/", h.Search)
		r.Post("/", h.Search)
	})

	// Queryables (if enabled)
	if h.cfg.Features.EnableQueryables {
		r.Get("/queryables", h.Queryables)
		r.Get("/collections/{collectionId}/queryables", h.Queryables)
	}

	// 404 handler
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		WriteNotFound(w, "endpoint not found")
	})

	// 405 handler
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
	})

	return r
}

// Queryables returns the queryable properties for collections.
// GET /queryables
// GET /collections/{collectionId}/queryables
func (h *Handlers) Queryables(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "collectionId")

	title := "Queryables for ASF STAC API"
	id := h.cfg.STAC.BaseURL + "/queryables"

	if collectionID != "" {
		if !h.collections.Has(collectionID) {
			WriteNotFound(w, "collection not found")
			return
		}
		title = "Queryables for " + collectionID
		id = h.cfg.STAC.BaseURL + "/collections/" + collectionID + "/queryables"
	}

	// Build properties map with all supported queryables
	properties := map[string]interface{}{
		// Core STAC queryables
		"datetime": map[string]interface{}{
			"description": "Datetime or datetime range",
			"type":        "string",
			"format":      "date-time",
		},
		"bbox": map[string]interface{}{
			"description": "Bounding box [west, south, east, north]",
			"type":        "array",
			"minItems":    4,
			"maxItems":    6,
			"items":       map[string]interface{}{"type": "number"},
		},
		"intersects": map[string]interface{}{
			"description": "GeoJSON geometry to intersect",
			"$ref":        "https://geojson.org/schema/Geometry.json",
		},
		"ids": map[string]interface{}{
			"description": "Array of item IDs to return",
			"type":        "array",
			"items":       map[string]interface{}{"type": "string"},
		},
		"collections": map[string]interface{}{
			"description": "Array of collection IDs to search",
			"type":        "array",
			"items":       map[string]interface{}{"type": "string"},
		},

		// SAR extension queryables
		"sar:instrument_mode": map[string]interface{}{
			"description": "SAR instrument mode (e.g., IW, EW, SM, WV)",
			"type":        "string",
		},
		"sar:polarizations": map[string]interface{}{
			"description": "SAR polarization mode (e.g., VV, VH, HH, HV)",
			"type":        "array",
			"items":       map[string]interface{}{"type": "string"},
		},
		"sar:frequency_band": map[string]interface{}{
			"description": "SAR frequency band (e.g., C, L, X)",
			"type":        "string",
		},

		// Satellite extension queryables
		"sat:orbit_state": map[string]interface{}{
			"description": "Orbit state (ascending or descending)",
			"type":        "string",
			"enum":        []string{"ascending", "descending"},
		},
		"sat:relative_orbit": map[string]interface{}{
			"description": "Relative orbit number",
			"type":        "integer",
		},
		"sat:absolute_orbit": map[string]interface{}{
			"description": "Absolute orbit number",
			"type":        "integer",
		},

		// Processing extension queryables
		"processing:level": map[string]interface{}{
			"description": "Processing level (e.g., SLC, GRD_HD, RAW, OCN)",
			"type":        "string",
		},

		// Platform
		"platform": map[string]interface{}{
			"description": "Platform identifier (e.g., sentinel-1a, alos)",
			"type":        "string",
		},
	}

	// Add collection-specific enum values from summaries
	if collectionID != "" {
		coll := h.collections.Get(collectionID)
		if coll != nil && coll.Summaries != nil {
			if modes, ok := coll.Summaries["sar:instrument_mode"].([]interface{}); ok {
				properties["sar:instrument_mode"].(map[string]interface{})["enum"] = modes
			}
			if bands, ok := coll.Summaries["sar:frequency_band"].([]interface{}); ok {
				properties["sar:frequency_band"].(map[string]interface{})["enum"] = bands
			}
			if platforms, ok := coll.Summaries["platform"].([]interface{}); ok {
				properties["platform"].(map[string]interface{})["enum"] = platforms
			}
		}
		// Only add processing:level queryable for UAVSAR (which has multiple processing levels)
		// Other collections are already split by processing level, so filtering doesn't apply
		if collectionID == "uavsar" {
			properties["processing:level"] = map[string]interface{}{
				"type":        "string",
				"title":       "Processing Level",
				"description": "Product processing level",
			}
		}
	}

	queryables := map[string]interface{}{
		"$schema":              "https://json-schema.org/draft/2019-09/schema",
		"$id":                  id,
		"type":                 "object",
		"title":                title,
		"description":          "Queryable properties for STAC API search",
		"properties":           properties,
		"additionalProperties": true,
	}

	WriteJSON(w, http.StatusOK, queryables)
}
