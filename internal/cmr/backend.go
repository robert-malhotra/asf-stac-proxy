package cmr

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/robert-malhotra/asf-stac-proxy/internal/backend"
	"github.com/robert-malhotra/asf-stac-proxy/internal/config"
	"github.com/robert-malhotra/asf-stac-proxy/internal/stac"
)

// CMRBackend implements backend.SearchBackend for NASA's CMR API.
type CMRBackend struct {
	client      *Client
	collections *config.CollectionRegistry
	cfg         *config.Config
	logger      *slog.Logger
}

// NewCMRBackend creates a new CMR backend.
func NewCMRBackend(
	client *Client,
	collections *config.CollectionRegistry,
	cfg *config.Config,
	logger *slog.Logger,
) *CMRBackend {
	return &CMRBackend{
		client:      client,
		collections: collections,
		cfg:         cfg,
		logger:      logger,
	}
}

// Name returns the backend name.
func (b *CMRBackend) Name() string {
	return "cmr"
}

// SupportsPagination returns false to use unified cursor-based pagination.
// While CMR supports native pagination via CMR-Search-After header, we use
// the same client-side cursor-based pagination as ASF for consistency.
func (b *CMRBackend) SupportsPagination() bool {
	return false
}

// Search executes a search against the CMR API.
func (b *CMRBackend) Search(ctx context.Context, params *backend.SearchParams) (*backend.SearchResult, error) {
	// Convert backend params to CMR params
	cmrParams, err := b.toCMRParams(params)
	if err != nil {
		return nil, fmt.Errorf("failed to convert search params: %w", err)
	}

	// Execute search
	result, err := b.client.Search(ctx, cmrParams)
	if err != nil {
		return nil, fmt.Errorf("CMR search failed: %w", err)
	}

	// Convert CMR granules to STAC items
	items := make([]*stac.Item, 0, len(result.Granules))
	for _, granule := range result.Granules {
		// Determine collection ID from granule
		collectionID := b.determineCollection(&granule)
		item, err := TranslateGranuleToItem(&granule, collectionID, b.cfg.STAC.BaseURL, b.cfg.STAC.Version)
		if err != nil {
			b.logger.Warn("failed to translate CMR granule",
				slog.String("granule_ur", granule.GranuleUR),
				slog.String("error", err.Error()),
			)
			continue
		}
		items = append(items, item)
	}

	return &backend.SearchResult{
		Items:      items,
		// NextCursor is not set - we use unified client-side cursor pagination
		TotalCount: &result.Hits,
	}, nil
}

// GetItem retrieves a single item from CMR.
func (b *CMRBackend) GetItem(ctx context.Context, collection, itemID string) (*stac.Item, error) {
	// Verify collection exists
	if !b.collections.Has(collection) {
		return nil, fmt.Errorf("collection %q not found", collection)
	}

	// Fetch from CMR
	granule, err := b.client.GetGranule(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch granule: %w", err)
	}

	// Convert to STAC item
	item, err := TranslateGranuleToItem(granule, collection, b.cfg.STAC.BaseURL, b.cfg.STAC.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to translate granule: %w", err)
	}

	return item, nil
}

// toCMRParams converts backend SearchParams to CMR-specific SearchParams.
func (b *CMRBackend) toCMRParams(params *backend.SearchParams) (*SearchParams, error) {
	cmrParams := &SearchParams{}

	// Map collections to CMR short names or concept IDs
	if len(params.Collections) > 0 {
		for _, collID := range params.Collections {
			coll := b.collections.Get(collID)
			if coll == nil {
				return nil, fmt.Errorf("collection %q not found", collID)
			}

			// Use CMR concept IDs if available, otherwise use short names
			if coll.CMR != nil && len(coll.CMR.ConceptIDs) > 0 {
				cmrParams.ConceptID = append(cmrParams.ConceptID, coll.CMR.ConceptIDs...)
			} else if coll.CMR != nil && len(coll.CMR.ShortNames) > 0 {
				cmrParams.ShortName = append(cmrParams.ShortName, coll.CMR.ShortNames...)
			} else {
				// Fallback: use ASF datasets as short names
				cmrParams.ShortName = append(cmrParams.ShortName, coll.ASFDatasets...)
			}
		}
	}

	// Map IDs to granule URs
	if len(params.IDs) > 0 {
		cmrParams.GranuleUR = params.IDs
	}

	// Map spatial filters
	if len(params.BBox) >= 4 {
		// CMR expects: west,south,east,north
		cmrParams.BoundingBox = fmt.Sprintf("%f,%f,%f,%f",
			params.BBox[0], params.BBox[1], params.BBox[2], params.BBox[3])
	}

	if len(params.Intersects) > 0 {
		// Convert GeoJSON to CMR polygon format
		polygon, err := geojsonToPolygon(params.Intersects)
		if err != nil {
			return nil, fmt.Errorf("failed to convert geometry: %w", err)
		}
		if polygon != "" {
			cmrParams.Polygon = polygon
		}
	}

	// Map temporal filters
	if params.Start != nil || params.End != nil {
		temporal := ""
		if params.Start != nil {
			temporal = params.Start.Format("2006-01-02T15:04:05Z")
		}
		temporal += ","
		if params.End != nil {
			temporal += params.End.Format("2006-01-02T15:04:05Z")
		}
		cmrParams.Temporal = temporal
	}

	// Map SAR-specific filters
	cmrParams.Polarization = params.Polarization
	cmrParams.BeamMode = params.BeamMode
	cmrParams.FlightDirection = params.FlightDirection
	cmrParams.RelativeOrbit = params.RelativeOrbit
	cmrParams.ProcessingLevel = params.ProcessingLevel

	// Map pagination
	if params.Limit > 0 {
		cmrParams.PageSize = params.Limit
	}
	// Note: We don't pass params.Cursor to CMR - we use unified client-side
	// cursor pagination that works the same way as ASF backend

	// Map sort
	if params.SortField != "" {
		dir := stac.SortAsc
		if params.SortDirection == "desc" {
			dir = stac.SortDesc
		}
		cmrParams.SortKey = stac.MapSTACFieldToCMRSort(params.SortField, dir)
	}

	return cmrParams, nil
}

// determineCollection determines the STAC collection ID for a CMR granule.
func (b *CMRBackend) determineCollection(granule *UMMGranule) string {
	shortName := granule.CollectionReference.ShortName

	// Get processing level from granule (may be empty)
	processingLevel := ""
	if levels := granule.GetAdditionalAttribute("PROCESSING_LEVEL"); len(levels) > 0 {
		processingLevel = levels[0]
	} else if levels := granule.GetAdditionalAttribute("PROCESSING_TYPE"); len(levels) > 0 {
		processingLevel = levels[0]
	}

	// Check all collections for matching CMR short name AND processing level
	for _, coll := range b.collections.All() {
		shortNameMatch := false
		if coll.CMR != nil {
			for _, sn := range coll.CMR.ShortNames {
				if sn == shortName {
					shortNameMatch = true
					break
				}
			}
		}
		// Also check ASF datasets as fallback for short name matching
		if !shortNameMatch {
			for _, ds := range coll.ASFDatasets {
				if ds == shortName {
					shortNameMatch = true
					break
				}
			}
		}

		if shortNameMatch {
			// If collection has a specific processing level, it must match
			if coll.ASFProcessingLevel != "" {
				if coll.ASFProcessingLevel == processingLevel {
					return coll.ID
				}
			} else {
				// Collection has no processing level filter, accept any
				return coll.ID
			}
		}
	}

	// Fallback: try to find by short name only (no processing level match required)
	// This handles cases where processing level is not available in granule metadata
	for _, coll := range b.collections.All() {
		if coll.CMR != nil {
			for _, sn := range coll.CMR.ShortNames {
				if sn == shortName {
					return coll.ID
				}
			}
		}
	}

	// Try to infer from platform
	if len(granule.Platforms) > 0 {
		platform := strings.ToLower(granule.Platforms[0].ShortName)
		for _, coll := range b.collections.All() {
			for _, p := range coll.ASFPlatforms {
				if strings.ToLower(p) == platform {
					return coll.ID
				}
			}
		}
	}

	// Fallback: return empty string (cross-collection search)
	return ""
}


// geojsonToPolygon converts GeoJSON geometry to CMR polygon format.
func geojsonToPolygon(geojsonBytes []byte) (string, error) {
	var g struct {
		Type        string      `json:"type"`
		Coordinates interface{} `json:"coordinates"`
	}

	if err := json.Unmarshal(geojsonBytes, &g); err != nil {
		return "", err
	}

	switch g.Type {
	case "Polygon":
		coords, ok := g.Coordinates.([][][]float64)
		if !ok {
			// Try to parse as generic interface
			return parsePolygonCoords(g.Coordinates)
		}
		if len(coords) == 0 || len(coords[0]) == 0 {
			return "", nil
		}

		// CMR expects: lon1,lat1,lon2,lat2,...
		var parts []string
		for _, pt := range coords[0] {
			if len(pt) >= 2 {
				parts = append(parts, fmt.Sprintf("%f,%f", pt[0], pt[1]))
			}
		}
		return strings.Join(parts, ","), nil

	case "Point":
		coords, ok := g.Coordinates.([]float64)
		if !ok {
			return "", nil
		}
		if len(coords) >= 2 {
			return fmt.Sprintf("%f,%f", coords[0], coords[1]), nil
		}
	}

	return "", nil
}

// parsePolygonCoords parses polygon coordinates from generic interface.
func parsePolygonCoords(coords interface{}) (string, error) {
	rings, ok := coords.([]interface{})
	if !ok || len(rings) == 0 {
		return "", nil
	}

	ring, ok := rings[0].([]interface{})
	if !ok || len(ring) == 0 {
		return "", nil
	}

	var parts []string
	for _, pt := range ring {
		point, ok := pt.([]interface{})
		if !ok || len(point) < 2 {
			continue
		}

		lon, ok1 := point[0].(float64)
		lat, ok2 := point[1].(float64)
		if ok1 && ok2 {
			parts = append(parts, fmt.Sprintf("%f,%f", lon, lat))
		}
	}

	return strings.Join(parts, ","), nil
}
