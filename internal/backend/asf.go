package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/robert-malhotra/asf-stac-proxy/internal/asf"
	"github.com/robert-malhotra/asf-stac-proxy/internal/config"
	"github.com/robert-malhotra/asf-stac-proxy/internal/stac"
	"github.com/robert-malhotra/asf-stac-proxy/internal/translate"
)

// ASFBackend implements SearchBackend for the ASF Search API.
type ASFBackend struct {
	client      *asf.Client
	collections *config.CollectionRegistry
	translator  *translate.Translator
	cfg         *config.Config
	logger      *slog.Logger
}

// NewASFBackend creates a new ASF backend.
func NewASFBackend(
	client *asf.Client,
	collections *config.CollectionRegistry,
	translator *translate.Translator,
	cfg *config.Config,
	logger *slog.Logger,
) *ASFBackend {
	return &ASFBackend{
		client:      client,
		collections: collections,
		translator:  translator,
		cfg:         cfg,
		logger:      logger,
	}
}

// Name returns the backend name.
func (b *ASFBackend) Name() string {
	return "asf"
}

// SupportsPagination returns false because ASF API doesn't support native pagination.
func (b *ASFBackend) SupportsPagination() bool {
	return false
}

// Search executes a search against the ASF API.
func (b *ASFBackend) Search(ctx context.Context, params *SearchParams) (*SearchResult, error) {
	// Convert backend params to ASF params
	asfParams, err := b.toASFParams(params)
	if err != nil {
		return nil, fmt.Errorf("failed to convert search params: %w", err)
	}

	// Execute search
	resp, err := b.client.Search(ctx, *asfParams)
	if err != nil {
		return nil, fmt.Errorf("ASF search failed: %w", err)
	}

	// Convert ASF features to STAC items
	items := make([]*stac.Item, 0, len(resp.Features))
	for _, feature := range resp.Features {
		// Determine collection ID from feature
		collectionID := b.determineCollection(&feature)
		item, err := translate.TranslateASFFeatureToItem(&feature, collectionID, b.cfg.STAC.BaseURL, b.cfg.STAC.Version)
		if err != nil {
			b.logger.Warn("failed to translate ASF feature",
				slog.String("feature_id", feature.Properties.FileID),
				slog.String("error", err.Error()),
			)
			continue
		}
		items = append(items, item)
	}

	return &SearchResult{
		Items:      items,
		TotalCount: resp.TotalCount,
		// NextCursor is handled by the pagination layer, not the backend
	}, nil
}

// GetItem retrieves a single item from ASF.
func (b *ASFBackend) GetItem(ctx context.Context, collection, itemID string) (*stac.Item, error) {
	// Verify collection exists
	if !b.collections.Has(collection) {
		return nil, fmt.Errorf("collection %q not found", collection)
	}

	// Fetch from ASF
	feature, err := b.client.GetGranule(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch granule: %w", err)
	}

	// Convert to STAC item
	item, err := translate.TranslateASFFeatureToItem(feature, collection, b.cfg.STAC.BaseURL, b.cfg.STAC.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to translate feature: %w", err)
	}

	return item, nil
}

// toASFParams converts backend SearchParams to ASF-specific SearchParams.
func (b *ASFBackend) toASFParams(params *SearchParams) (*asf.SearchParams, error) {
	asfParams := &asf.SearchParams{
		Output: "geojson",
	}

	// Map IDs to granule list
	// IMPORTANT: ASF API does not allow combining granule_list with ANY other search params
	// (including maxResults), so when IDs are provided, we return early with only the granule list.
	if len(params.IDs) > 0 {
		asfParams.GranuleList = params.IDs
		return asfParams, nil
	}

	// Map collections to ASF datasets
	if len(params.Collections) > 0 {
		for _, collID := range params.Collections {
			datasets := b.collections.GetASFDatasets(collID)
			if datasets == nil {
				return nil, fmt.Errorf("collection %q not found", collID)
			}
			asfParams.Dataset = append(asfParams.Dataset, datasets...)
		}
	}

	// Map spatial filters
	if len(params.BBox) > 0 {
		wkt, err := translate.BBoxToWKT(params.BBox)
		if err != nil {
			return nil, fmt.Errorf("invalid bbox: %w", err)
		}
		asfParams.IntersectsWith = wkt
	} else if len(params.Intersects) > 0 {
		var geom translate.Geometry
		if err := json.Unmarshal(params.Intersects, &geom); err != nil {
			return nil, fmt.Errorf("invalid intersects geometry: %w", err)
		}
		wkt, err := translate.IntersectsToWKT(&geom)
		if err != nil {
			return nil, fmt.Errorf("failed to convert geometry to WKT: %w", err)
		}
		asfParams.IntersectsWith = wkt
	}

	// Map temporal filters
	asfParams.Start = params.Start
	asfParams.End = params.End

	// Map SAR-specific filters
	asfParams.BeamMode = params.BeamMode
	asfParams.Polarization = params.Polarization
	asfParams.FlightDirection = params.FlightDirection
	asfParams.RelativeOrbit = params.RelativeOrbit
	asfParams.AbsoluteOrbit = params.AbsoluteOrbit
	asfParams.Platform = params.Platform

	// Apply processing level filter
	// If user specified processing levels, use those
	// Otherwise, use collection-configured level (each collection now has at most one)
	if len(params.ProcessingLevel) > 0 {
		asfParams.ProcessingLevel = params.ProcessingLevel
	} else if len(params.Collections) > 0 {
		// Aggregate processing levels from all collections
		levelSet := make(map[string]bool)
		for _, collID := range params.Collections {
			level := b.collections.GetASFProcessingLevel(collID)
			if level != "" {
				levelSet[level] = true
			}
		}
		if len(levelSet) > 0 {
			for level := range levelSet {
				asfParams.ProcessingLevel = append(asfParams.ProcessingLevel, level)
			}
		}
	}

	// Map limit
	if params.Limit > 0 {
		asfParams.MaxResults = params.Limit
	}

	// Map sort
	if params.SortField != "" {
		asfSort, err := stac.MapSTACFieldToASFSort(params.SortField)
		if err == nil {
			asfParams.Sort = asfSort
		}
	}

	return asfParams, nil
}

// determineCollection determines the STAC collection ID for an ASF feature.
func (b *ASFBackend) determineCollection(feature *asf.ASFFeature) string {
	platform := feature.Properties.Platform
	processingLevel := feature.Properties.ProcessingLevel

	// Find collection matching platform AND processing level
	for _, coll := range b.collections.All() {
		platformMatch := false
		for _, p := range coll.ASFPlatforms {
			if p == platform {
				platformMatch = true
				break
			}
		}
		if platformMatch {
			// If collection has a specific processing level, it must match
			if coll.ASFProcessingLevel != "" {
				if coll.ASFProcessingLevel == processingLevel {
					return coll.ID
				}
			} else {
				// Collection has no processing level filter (like UAVSAR), accept any
				return coll.ID
			}
		}
	}

	// Fallback: return empty string (cross-collection search)
	return ""
}

