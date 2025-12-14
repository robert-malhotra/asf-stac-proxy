// Package translate provides translation between ASF and STAC formats.
package translate

import (
	"encoding/json"
	"log/slog"

	"github.com/rkm/asf-stac-proxy/internal/asf"
	"github.com/rkm/asf-stac-proxy/internal/config"
	"github.com/rkm/asf-stac-proxy/internal/stac"
	"github.com/rkm/asf-stac-proxy/pkg/geojson"
)

// Geometry is an alias for geojson.Geometry for convenience
type Geometry = geojson.Geometry

// Translator handles conversion between STAC and ASF formats.
type Translator struct {
	cfg         *config.Config
	collections *config.CollectionRegistry
	logger      *slog.Logger
}

// NewTranslator creates a new translator instance.
func NewTranslator(cfg *config.Config, collections *config.CollectionRegistry, logger *slog.Logger) *Translator {
	return &Translator{
		cfg:         cfg,
		collections: collections,
		logger:      logger,
	}
}

// TranslateSTACSearchToASF converts a STAC search request to ASF search parameters.
// If collectionID is not empty, it filters to that collection's ASF datasets.
func (t *Translator) TranslateSTACSearchToASF(req *stac.SearchRequest, collectionID string) (*asf.SearchParams, error) {
	params := &asf.SearchParams{
		Output: "geojson",
	}

	// Map collections to ASF datasets
	if collectionID != "" {
		// Single collection specified (from /collections/{id}/items)
		datasets := t.collections.GetASFDatasets(collectionID)
		if datasets == nil {
			return nil, ErrCollectionNotFound
		}
		params.Dataset = datasets
	} else if len(req.Collections) > 0 {
		// Collections from search request
		for _, collID := range req.Collections {
			datasets := t.collections.GetASFDatasets(collID)
			if datasets == nil {
				return nil, ErrCollectionNotFound
			}
			params.Dataset = append(params.Dataset, datasets...)
		}
	}

	// Map IDs to granule list
	if len(req.IDs) > 0 {
		params.GranuleList = req.IDs
	}

	// Map spatial filters
	if len(req.BBox) > 0 {
		// Convert bbox to WKT polygon
		wkt, err := BBoxToWKT(req.BBox)
		if err != nil {
			t.logger.Error("failed to convert bbox to WKT", "error", err)
			return nil, ErrInvalidGeometry
		}
		params.IntersectsWith = wkt
	} else if len(req.Intersects) > 0 {
		// Parse intersects GeoJSON and convert to WKT
		var geom Geometry
		if err := json.Unmarshal(req.Intersects, &geom); err != nil {
			t.logger.Error("failed to parse intersects geometry", "error", err)
			return nil, ErrInvalidGeometry
		}
		wkt, err := IntersectsToWKT(&geom)
		if err != nil {
			t.logger.Error("failed to convert intersects to WKT", "error", err)
			return nil, ErrInvalidGeometry
		}
		params.IntersectsWith = wkt
	}

	// Map temporal filters
	if req.DateTime != "" {
		start, end, err := ParseDateTimeInterval(req.DateTime)
		if err != nil {
			t.logger.Error("failed to parse datetime", "error", err)
			return nil, ErrInvalidDateTime
		}
		params.Start = start
		params.End = end
	}

	// Apply cursor-based pagination (modifies end time to get next page)
	if req.Cursor != "" {
		cursor, err := stac.DecodeCursor(req.Cursor)
		if err != nil {
			t.logger.Error("failed to decode cursor", "error", err)
			return nil, ErrInvalidCursor
		}
		if cursor != nil {
			params.End = stac.ApplyCursorToDatetime(cursor, params.End)
			t.logger.Debug("applied cursor to datetime filter",
				slog.String("cursor_start_time", cursor.StartTime),
				slog.Any("new_end", params.End),
			)
		}
	}

	// Map limit
	if req.Limit > 0 {
		params.MaxResults = req.Limit
	}

	// Note: Page parameter is not passed to ASF as ASF API doesn't support pagination.
	// Pagination is handled by the proxy via next/prev links.

	// Map sortby to ASF sort parameters
	// Note: ASF API doesn't support sort direction, results are always descending
	if len(req.Sortby) > 0 {
		// ASF only supports single field sorting, use the first one
		sortby := req.Sortby[0]
		asfSort, err := stac.MapSTACFieldToASFSort(sortby.Field)
		if err != nil {
			t.logger.Warn("unsupported sort field, ignoring", "field", sortby.Field)
		} else {
			params.Sort = asfSort
			// ASF doesn't support sort direction, log if user requested ascending
			if sortby.Direction == "asc" {
				t.logger.Warn("ASF API doesn't support ascending sort, using default descending order")
			}
		}
	}

	// Process CQL2 filter if present
	// Extension properties (sar:*, sat:*, processing:*, etc.) should be filtered via CQL2-JSON
	if req.Filter != nil {
		if err := TranslateCQL2Filter(req.Filter, params); err != nil {
			t.logger.Error("failed to translate CQL2 filter", "error", err)
			return nil, err
		}
	}

	return params, nil
}


// TranslateASFFeatureToSTACItem converts an ASF feature to a STAC item.
func (t *Translator) TranslateASFFeatureToSTACItem(feature *asf.ASFFeature, collectionID string) (*stac.Item, error) {
	// Use the detailed translation function from item.go
	return TranslateASFFeatureToItem(feature, collectionID, t.cfg.STAC.BaseURL, t.cfg.STAC.Version)
}

// TranslateASFResponseToItemCollection converts an ASF response to a STAC ItemCollection.
func (t *Translator) TranslateASFResponseToItemCollection(
	resp *asf.ASFGeoJSONResponse,
	req *stac.SearchRequest,
	collectionID string,
) (*stac.ItemCollection, error) {
	items := make([]*stac.Item, 0, len(resp.Features))

	for _, feature := range resp.Features {
		item, err := t.TranslateASFFeatureToSTACItem(&feature, collectionID)
		if err != nil {
			t.logger.Warn("failed to translate feature",
				slog.String("feature_id", feature.Properties.FileID),
				slog.String("error", err.Error()),
			)
			continue
		}
		items = append(items, item)
	}

	itemCollection := stac.NewItemCollection(items)

	// Set context with pagination metadata
	limit := req.Limit
	if limit == 0 {
		limit = t.cfg.Features.DefaultLimit
	}
	itemCollection.SetContext(len(items), limit, resp.TotalCount)

	// Add links
	baseURL := t.cfg.STAC.BaseURL
	if baseURL != "" {
		if collectionID != "" {
			// Link to the collection items endpoint
			itemCollection.AddLink("self", baseURL+"/collections/"+collectionID+"/items", "application/geo+json")
		} else {
			// Link to the search endpoint
			itemCollection.AddLink("self", baseURL+"/search", "application/geo+json")
		}
		// Root link
		itemCollection.AddLink("root", baseURL, "application/json")
	}

	return itemCollection, nil
}
