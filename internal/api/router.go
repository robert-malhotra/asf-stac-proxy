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
			"description": "SAR polarization combinations (e.g., [VV], [VV, VH])",
			"type":        "array",
			"items":       map[string]interface{}{"type": "string"},
		},
		"sar:frequency_band": map[string]interface{}{
			"description": "SAR frequency band (e.g., C, L, X)",
			"type":        "string",
		},
		"sar:product_type": map[string]interface{}{
			"description": "SAR product type identifier (e.g., SLC, GRD, RAW, OCN)",
			"type":        "string",
		},

		// Satellite extension queryables
		"sat:orbit_state": map[string]interface{}{
			"description": "Orbit state (ascending or descending)",
			"type":        "string",
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
			"description": "Processing level (e.g., L0, L1, L2)",
			"type":        "string",
		},

		// Platform and constellation
		"platform": map[string]interface{}{
			"description": "Platform identifier (e.g., sentinel-1a, alos)",
			"type":        "string",
		},
		"constellation": map[string]interface{}{
			"description": "Satellite constellation (e.g., sentinel-1)",
			"type":        "string",
		},
		"instruments": map[string]interface{}{
			"description": "Instrument identifiers (e.g., c-sar, palsar)",
			"type":        "array",
			"items":       map[string]interface{}{"type": "string"},
		},
	}

	// Add enum values based on whether this is global or collection-specific
	if collectionID != "" {
		// Collection-specific: use that collection's summaries
		coll := h.collections.Get(collectionID)
		if coll != nil && coll.Summaries != nil {
			addEnumFromSummary(properties, coll.Summaries, "platform")
			addEnumFromSummary(properties, coll.Summaries, "sar:instrument_mode")
			addEnumFromSummary(properties, coll.Summaries, "sar:frequency_band")
			addEnumFromSummary(properties, coll.Summaries, "sar:product_type")
			addEnumFromSummary(properties, coll.Summaries, "sat:orbit_state")
			addEnumFromSummary(properties, coll.Summaries, "constellation")
			addEnumFromSummary(properties, coll.Summaries, "processing:level")
			// Handle polarizations specially - flatten to channels as enum on items
			addPolarizationChannelsEnum(properties, coll.Summaries)
			// Handle instruments - set as enum on items
			addArrayItemsEnumFromSummary(properties, coll.Summaries, "instruments")
		}
	} else {
		// Global: aggregate enum values from all collections
		h.addGlobalEnums(properties)
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

// addEnumFromSummary adds enum values to a property from collection summaries
func addEnumFromSummary(properties map[string]interface{}, summaries map[string]interface{}, field string) {
	if values, ok := summaries[field].([]interface{}); ok && len(values) > 0 {
		if prop, ok := properties[field].(map[string]interface{}); ok {
			prop["enum"] = values
		}
	}
}

// addPolarizationChannelsEnum flattens polarization combinations and adds as enum on items
func addPolarizationChannelsEnum(properties map[string]interface{}, summaries map[string]interface{}) {
	polarizations, ok := summaries["sar:polarizations"].([]interface{})
	if !ok || len(polarizations) == 0 {
		return
	}

	// Flatten to unique channels
	seen := make(map[string]bool)
	var channels []string
	for _, p := range polarizations {
		if chArray, ok := p.([]interface{}); ok {
			for _, ch := range chArray {
				if s, ok := ch.(string); ok && !seen[s] {
					seen[s] = true
					channels = append(channels, s)
				}
			}
		}
	}

	if len(channels) > 0 {
		if prop, ok := properties["sar:polarizations"].(map[string]interface{}); ok {
			if items, ok := prop["items"].(map[string]interface{}); ok {
				items["enum"] = channels
			}
		}
	}
}

// addArrayItemsEnumFromSummary adds enum values to array items from collection summaries
func addArrayItemsEnumFromSummary(properties map[string]interface{}, summaries map[string]interface{}, field string) {
	values, ok := summaries[field].([]interface{})
	if !ok || len(values) == 0 {
		return
	}

	// Convert to string slice
	var strValues []string
	for _, v := range values {
		if s, ok := v.(string); ok {
			strValues = append(strValues, s)
		}
	}

	if len(strValues) > 0 {
		if prop, ok := properties[field].(map[string]interface{}); ok {
			if items, ok := prop["items"].(map[string]interface{}); ok {
				items["enum"] = strValues
			}
		}
	}
}

// addGlobalEnums aggregates enum values from all collections for global queryables
func (h *Handlers) addGlobalEnums(properties map[string]interface{}) {
	// Fields to aggregate
	stringFields := []string{
		"platform",
		"sar:instrument_mode",
		"sar:frequency_band",
		"sar:product_type",
		"sat:orbit_state",
		"constellation",
		"processing:level",
	}

	// Aggregate string fields
	for _, field := range stringFields {
		values := h.aggregateStringEnums(field)
		if len(values) > 0 {
			if prop, ok := properties[field].(map[string]interface{}); ok {
				prop["enum"] = values
			}
		}
	}

	// Handle sar:polarizations - flatten to unique channels and set as enum on items
	if polarizations := h.aggregatePolarizationChannels(); len(polarizations) > 0 {
		if prop, ok := properties["sar:polarizations"].(map[string]interface{}); ok {
			if items, ok := prop["items"].(map[string]interface{}); ok {
				items["enum"] = polarizations
			}
		}
	}

	// Handle instruments - set as enum on items
	if instruments := h.aggregateStringEnums("instruments"); len(instruments) > 0 {
		if prop, ok := properties["instruments"].(map[string]interface{}); ok {
			if items, ok := prop["items"].(map[string]interface{}); ok {
				items["enum"] = instruments
			}
		}
	}
}

// aggregateStringEnums collects unique string values from a field across all collections
func (h *Handlers) aggregateStringEnums(field string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, coll := range h.collections.All() {
		if coll.Summaries == nil {
			continue
		}
		if values, ok := coll.Summaries[field].([]interface{}); ok {
			for _, v := range values {
				if s, ok := v.(string); ok && !seen[s] {
					seen[s] = true
					result = append(result, s)
				}
			}
		}
	}

	return result
}

// aggregatePolarizationChannels flattens all polarization combinations into unique channels
func (h *Handlers) aggregatePolarizationChannels() []string {
	seen := make(map[string]bool)
	var result []string

	for _, coll := range h.collections.All() {
		if coll.Summaries == nil {
			continue
		}
		if polarizations, ok := coll.Summaries["sar:polarizations"].([]interface{}); ok {
			for _, p := range polarizations {
				// Each polarization can be an array of channels like ["VV", "VH"]
				if channels, ok := p.([]interface{}); ok {
					for _, ch := range channels {
						if s, ok := ch.(string); ok && !seen[s] {
							seen[s] = true
							result = append(result, s)
						}
					}
				}
			}
		}
	}

	return result
}
