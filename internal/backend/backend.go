// Package backend provides an abstraction layer for search backends (ASF, CMR).
package backend

import (
	"context"
	"time"

	"github.com/rkm/asf-stac-proxy/internal/stac"
)

// SearchBackend defines the interface for search backends.
// Both ASF and CMR backends implement this interface.
type SearchBackend interface {
	// Search executes a search query and returns STAC items.
	Search(ctx context.Context, params *SearchParams) (*SearchResult, error)

	// GetItem retrieves a single item by ID.
	GetItem(ctx context.Context, collection, itemID string) (*stac.Item, error)

	// Name returns the backend name (e.g., "asf", "cmr").
	Name() string

	// SupportsPagination returns true if the backend supports native pagination.
	// CMR supports native pagination via CMR-Search-After header.
	// ASF does not support pagination, requiring client-side cursor management.
	SupportsPagination() bool
}

// SearchParams contains parameters for search queries.
// These are backend-agnostic and will be translated to backend-specific formats.
type SearchParams struct {
	// Collections to search (STAC collection IDs)
	Collections []string

	// Spatial filters
	BBox       []float64 // [west, south, east, north] or 6 element 3D bbox
	Intersects []byte    // GeoJSON geometry as raw JSON

	// Temporal filters
	Start *time.Time
	End   *time.Time

	// Item identification
	IDs []string

	// Pagination
	Limit  int
	Cursor string // Opaque cursor from previous response

	// SAR-specific filters
	BeamMode        []string
	Polarization    []string
	FlightDirection string
	RelativeOrbit   []int
	AbsoluteOrbit   []int
	ProcessingLevel []string
	Platform        []string // Platform names (e.g., "Sentinel-1A", "Sentinel-1B")

	// Sorting
	SortField     string // Field to sort by
	SortDirection string // "asc" or "desc"
}

// SearchResult contains the results of a search query.
type SearchResult struct {
	// Items are the STAC items returned by the search
	Items []*stac.Item

	// NextCursor is the opaque cursor for the next page (empty if no more results)
	// For CMR: This is the CMR-Search-After header value
	// For ASF: This is our custom cursor encoding
	NextCursor string

	// TotalCount is the total number of matching items (nil if unknown)
	TotalCount *int
}

// DatetimeRange represents a temporal range for filtering.
type DatetimeRange struct {
	Start *time.Time
	End   *time.Time
}
