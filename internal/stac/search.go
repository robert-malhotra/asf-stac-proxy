package stac

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// SortbyItem represents a single sort criterion
type SortbyItem struct {
	Field     string `json:"field"`
	Direction string `json:"direction"` // "asc" or "desc"
}

// SearchRequest represents a STAC search request.
// Standard STAC query parameters are supported directly.
// Extension-specific filters (sar:*, sat:*, etc.) should use the CQL2 filter.
type SearchRequest struct {
	// Core STAC search parameters
	BBox        []float64       `json:"bbox,omitempty"`
	DateTime    string          `json:"datetime,omitempty"`
	Intersects  json.RawMessage `json:"intersects,omitempty"`
	IDs         []string        `json:"ids,omitempty"`
	Collections []string        `json:"collections,omitempty"`
	Limit       int             `json:"limit,omitempty"`

	// Cursor-based pagination (preferred for ASF backend)
	Cursor string `json:"cursor,omitempty"` // Base64-encoded cursor for pagination

	// Sortby extension
	Sortby []SortbyItem `json:"sortby,omitempty"`

	// Filter extension - use CQL2-JSON for extension properties like sar:*, sat:*
	Filter     any    `json:"filter,omitempty"`
	FilterLang string `json:"filter-lang,omitempty"`
	FilterCRS  string `json:"filter-crs,omitempty"`
}

// ParseSearchRequest parses a STAC search request from GET query parameters
func ParseSearchRequest(r *http.Request) (*SearchRequest, error) {
	query := r.URL.Query()
	req := &SearchRequest{}

	// Parse bbox parameter
	if bboxStr := query.Get("bbox"); bboxStr != "" {
		bboxParts := strings.Split(bboxStr, ",")
		if len(bboxParts) != 4 && len(bboxParts) != 6 {
			return nil, fmt.Errorf("bbox must have 4 or 6 coordinates, got %d", len(bboxParts))
		}

		bbox := make([]float64, len(bboxParts))
		for i, part := range bboxParts {
			val, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
			if err != nil {
				return nil, fmt.Errorf("invalid bbox coordinate at position %d: %w", i, err)
			}
			bbox[i] = val
		}
		req.BBox = bbox
	}

	// Parse datetime parameter
	if datetime := query.Get("datetime"); datetime != "" {
		req.DateTime = datetime
	}

	// Parse intersects parameter (GeoJSON geometry as URL-encoded JSON)
	if intersects := query.Get("intersects"); intersects != "" {
		if !json.Valid([]byte(intersects)) {
			return nil, fmt.Errorf("intersects must be valid GeoJSON geometry")
		}
		req.Intersects = json.RawMessage(intersects)
	}

	// Parse ids parameter (comma-separated list)
	if ids := query.Get("ids"); ids != "" {
		req.IDs = strings.Split(ids, ",")
		for i := range req.IDs {
			req.IDs[i] = strings.TrimSpace(req.IDs[i])
		}
	}

	// Parse collections parameter (comma-separated list)
	if collections := query.Get("collections"); collections != "" {
		req.Collections = strings.Split(collections, ",")
		for i := range req.Collections {
			req.Collections[i] = strings.TrimSpace(req.Collections[i])
		}
	}

	// Parse limit parameter
	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return nil, fmt.Errorf("invalid limit parameter: %w", err)
		}
		if limit < 0 {
			return nil, fmt.Errorf("limit must be non-negative, got %d", limit)
		}
		req.Limit = limit
	}

	// Parse cursor parameter (for pagination)
	if cursor := query.Get("cursor"); cursor != "" {
		req.Cursor = cursor
	}

	// Parse sortby parameter
	if sortbyStr := query.Get("sortby"); sortbyStr != "" {
		sortbyItems, err := parseSortbyParam(sortbyStr)
		if err != nil {
			return nil, fmt.Errorf("invalid sortby parameter: %w", err)
		}
		req.Sortby = sortbyItems
	}

	// Parse filter parameters
	if filter := query.Get("filter"); filter != "" {
		// Try to parse as JSON (CQL2-JSON format) if filter-lang is cql2-json or auto-detect
		filterLang := query.Get("filter-lang")
		if filterLang == "cql2-json" || (filterLang == "" && strings.HasPrefix(strings.TrimSpace(filter), "{")) {
			// Parse as JSON
			var filterObj interface{}
			if err := json.Unmarshal([]byte(filter), &filterObj); err == nil {
				req.Filter = filterObj
			} else {
				// Fall back to storing as string if JSON parsing fails
				req.Filter = filter
			}
		} else {
			// Store as-is for CQL2-Text format
			req.Filter = filter
		}
	}
	if filterLang := query.Get("filter-lang"); filterLang != "" {
		req.FilterLang = filterLang
	}
	if filterCRS := query.Get("filter-crs"); filterCRS != "" {
		req.FilterCRS = filterCRS
	}

	return req, nil
}

// parseSortbyParam parses the sortby query parameter
// Format: sortby=+datetime or sortby=-datetime (+ is asc, - is desc)
// Multiple sorts: sortby=+datetime,-platform
func parseSortbyParam(sortbyStr string) ([]SortbyItem, error) {
	if sortbyStr == "" {
		return nil, nil
	}

	fields := strings.Split(sortbyStr, ",")
	items := make([]SortbyItem, 0, len(fields))

	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}

		var direction string
		var fieldName string

		if strings.HasPrefix(field, "+") {
			direction = "asc"
			fieldName = field[1:]
		} else if strings.HasPrefix(field, "-") {
			direction = "desc"
			fieldName = field[1:]
		} else {
			direction = "asc"
			fieldName = field
		}

		if fieldName == "" {
			return nil, fmt.Errorf("empty field name in sortby")
		}

		items = append(items, SortbyItem{
			Field:     fieldName,
			Direction: direction,
		})
	}

	return items, nil
}

// ParseSearchRequestBody parses a STAC search request from POST JSON body
func ParseSearchRequestBody(body io.Reader) (*SearchRequest, error) {
	var req SearchRequest

	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&req); err != nil {
		return nil, fmt.Errorf("failed to parse search request body: %w", err)
	}

	return &req, nil
}

// ToQueryParams converts a SearchRequest to URL query parameters.
// This is used to preserve search parameters in pagination links for POST requests.
func (req *SearchRequest) ToQueryParams() url.Values {
	params := url.Values{}

	// BBox
	if len(req.BBox) >= 4 {
		bboxStrs := make([]string, len(req.BBox))
		for i, v := range req.BBox {
			bboxStrs[i] = strconv.FormatFloat(v, 'f', -1, 64)
		}
		params.Set("bbox", strings.Join(bboxStrs, ","))
	}

	// DateTime
	if req.DateTime != "" {
		params.Set("datetime", req.DateTime)
	}

	// Intersects (GeoJSON geometry)
	if len(req.Intersects) > 0 {
		params.Set("intersects", string(req.Intersects))
	}

	// IDs
	if len(req.IDs) > 0 {
		params.Set("ids", strings.Join(req.IDs, ","))
	}

	// Collections
	if len(req.Collections) > 0 {
		params.Set("collections", strings.Join(req.Collections, ","))
	}

	// Limit is handled separately in pagination link building

	// Sortby
	if len(req.Sortby) > 0 {
		var sortbyStrs []string
		for _, item := range req.Sortby {
			prefix := "+"
			if item.Direction == "desc" {
				prefix = "-"
			}
			sortbyStrs = append(sortbyStrs, prefix+item.Field)
		}
		params.Set("sortby", strings.Join(sortbyStrs, ","))
	}

	// Filter - convert to JSON string for query param
	if req.Filter != nil {
		filterBytes, err := json.Marshal(req.Filter)
		if err == nil && len(filterBytes) > 0 && string(filterBytes) != "null" {
			params.Set("filter", string(filterBytes))
		}
	}

	// Filter-lang
	if req.FilterLang != "" {
		params.Set("filter-lang", req.FilterLang)
	} else if req.Filter != nil {
		// Default to cql2-json for JSON filters
		params.Set("filter-lang", "cql2-json")
	}

	// Filter-crs
	if req.FilterCRS != "" {
		params.Set("filter-crs", req.FilterCRS)
	}

	return params
}
