package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/planetlabs/go-ogc/filter"
	"github.com/planetlabs/go-stac"
	"github.com/robert-malhotra/asf-stac-proxy/internal/backend"
	"github.com/robert-malhotra/asf-stac-proxy/internal/config"
	intstac "github.com/robert-malhotra/asf-stac-proxy/internal/stac"
	"github.com/robert-malhotra/asf-stac-proxy/internal/translate"
)

// Handlers contains all HTTP handlers for the STAC API.
type Handlers struct {
	cfg         *config.Config
	backend     backend.SearchBackend
	translator  *translate.Translator
	collections *config.CollectionRegistry
	cursorStore intstac.CursorStore
	logger      *slog.Logger
}

// NewHandlers creates a new Handlers instance with the given dependencies.
func NewHandlers(
	cfg *config.Config,
	searchBackend backend.SearchBackend,
	translator *translate.Translator,
	collections *config.CollectionRegistry,
	logger *slog.Logger,
) *Handlers {
	return &Handlers{
		cfg:         cfg,
		backend:     searchBackend,
		translator:  translator,
		collections: collections,
		logger:      logger,
	}
}

// WithCursorStore sets the cursor store for server-side cursor storage.
func (h *Handlers) WithCursorStore(store intstac.CursorStore) *Handlers {
	h.cursorStore = store
	return h
}

// LandingPage returns the STAC API landing page (root catalog).
// GET /
func (h *Handlers) LandingPage(w http.ResponseWriter, r *http.Request) {
	baseURL := h.cfg.STAC.BaseURL

	// Create landing page
	landing := intstac.NewLandingPage(
		"asf-stac-root",
		h.cfg.STAC.Title,
		h.cfg.STAC.Description,
		h.cfg.STAC.Version,
		intstac.DefaultConformance(),
	)

	// Add links
	landing.AddLink("self", baseURL+"/", "application/json")
	landing.AddLink("root", baseURL+"/", "application/json")
	landing.AddLink("conformance", baseURL+"/conformance", "application/json")
	landing.AddLink("data", baseURL+"/collections", "application/json")

	if h.cfg.Features.EnableSearch {
		landing.Links = append(landing.Links, &stac.Link{
			Rel:    "search",
			Href:   baseURL + "/search",
			Type:   "application/geo+json",
			Method: "GET",
		})
		landing.Links = append(landing.Links, &stac.Link{
			Rel:    "search",
			Href:   baseURL + "/search",
			Type:   "application/geo+json",
			Method: "POST",
		})
	}

	landing.AddLink("service-desc", baseURL+"/api", "application/vnd.oai.openapi+json;version=3.0")
	landing.AddLink("service-doc", baseURL+"/api.html", "text/html")

	WriteJSON(w, http.StatusOK, landing)
}

// Conformance returns the conformance classes supported by this API.
// GET /conformance
func (h *Handlers) Conformance(w http.ResponseWriter, r *http.Request) {
	conformance := &intstac.Conformance{
		ConformsTo: intstac.DefaultConformance(),
	}

	WriteJSON(w, http.StatusOK, conformance)
}

// Collections returns the list of all available collections.
// GET /collections
func (h *Handlers) Collections(w http.ResponseWriter, r *http.Request) {
	baseURL := h.cfg.STAC.BaseURL

	// Get all collections from registry
	collectionConfigs := h.collections.All()
	h.logger.Info("building collections response", "count", len(collectionConfigs))
	collections := make([]*stac.Collection, 0, len(collectionConfigs))

	for i, cfg := range collectionConfigs {
		h.logger.Info("building collection", "index", i, "id", cfg.ID)
		collection := h.buildSTACCollection(cfg, baseURL)
		collections = append(collections, collection)
		h.logger.Info("built collection", "index", i, "id", cfg.ID)
	}

	// Build response
	response := intstac.NewCollectionsList(collections)
	response.Links = append(response.Links, &stac.Link{
		Rel:  "self",
		Href: baseURL + "/collections",
		Type: "application/json",
	})
	response.Links = append(response.Links, &stac.Link{
		Rel:  "root",
		Href: baseURL + "/",
		Type: "application/json",
	})

	WriteJSON(w, http.StatusOK, response)
}

// Collection returns a single collection by ID.
// GET /collections/{collectionId}
func (h *Handlers) Collection(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "collectionId")
	if collectionID == "" {
		WriteBadRequest(w, "collection ID is required")
		return
	}

	// Get collection from registry
	collectionConfig := h.collections.Get(collectionID)
	if collectionConfig == nil {
		WriteNotFound(w, fmt.Sprintf("collection %q not found", collectionID))
		return
	}

	// Build STAC collection
	collection := h.buildSTACCollection(collectionConfig, h.cfg.STAC.BaseURL)

	WriteJSON(w, http.StatusOK, collection)
}

// Items returns items from a specific collection.
// GET /collections/{collectionId}/items
func (h *Handlers) Items(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "collectionId")
	if collectionID == "" {
		WriteBadRequest(w, "collection ID is required")
		return
	}

	// Verify collection exists
	if !h.collections.Has(collectionID) {
		WriteNotFound(w, fmt.Sprintf("collection %q not found", collectionID))
		return
	}

	// Parse search request from query parameters
	searchReq, err := intstac.ParseSearchRequest(r)
	if err != nil {
		WriteInvalidParameter(w, fmt.Sprintf("invalid search parameters: %v", err))
		return
	}

	// Apply default limit if not specified
	if searchReq.Limit == 0 {
		searchReq.Limit = h.cfg.Features.DefaultLimit
	}

	// Enforce max limit
	if searchReq.Limit > h.cfg.Features.MaxLimit {
		searchReq.Limit = h.cfg.Features.MaxLimit
	}

	// Decode current cursor if present (needed for filtering duplicates on ASF backend)
	var currentCursor *intstac.Cursor
	if searchReq.Cursor != "" && !h.backend.SupportsPagination() {
		var cursorErr error
		currentCursor, cursorErr = intstac.DecodeCursorWithStore(searchReq.Cursor, h.cursorStore)
		if cursorErr != nil {
			h.logger.Warn("failed to decode cursor",
				slog.String("error", cursorErr.Error()),
				slog.String("cursor", searchReq.Cursor),
			)
			WriteBadRequest(w, fmt.Sprintf("invalid or expired cursor: %s", cursorErr.Error()))
			return
		}
	}

	// Build backend search params
	// For ASF backend with cursor, over-fetch to compensate for SeenIDs filtering
	backendParams := h.buildBackendParams(searchReq, collectionID)
	backendLimit := searchReq.Limit
	if !h.backend.SupportsPagination() && currentCursor != nil && len(currentCursor.SeenIDs) > 0 {
		// Request extra items to compensate for filtering
		backendLimit = searchReq.Limit + len(currentCursor.SeenIDs)
		if backendLimit > h.cfg.Features.MaxLimit {
			backendLimit = h.cfg.Features.MaxLimit
		}
		backendParams.Limit = backendLimit
	}

	// Execute search against backend
	ctx := r.Context()
	result, err := h.backend.Search(ctx, backendParams)
	if err != nil {
		h.logger.Error("backend search failed",
			slog.String("collection_id", collectionID),
			slog.String("backend", h.backend.Name()),
			slog.String("error", err.Error()),
		)
		WriteUpstreamError(w, "upstream search service error")
		return
	}

	// Track if backend returned a full page (used for pagination decision)
	backendReturnedFullPage := len(result.Items) >= backendLimit

	// Build STAC ItemCollection from backend results
	itemCollection := intstac.NewItemCollection(result.Items)

	// Filter out items that were already returned in the previous page (for ASF backend)
	if !h.backend.SupportsPagination() && currentCursor != nil {
		itemCollection.Features = intstac.FilterSeenItems(itemCollection.Features, func(item *intstac.Item) string {
			return item.Id
		}, currentCursor)
	}

	// Trim to requested limit if we over-fetched
	if len(itemCollection.Features) > searchReq.Limit {
		itemCollection.Features = itemCollection.Features[:searchReq.Limit]
	}

	// Set context with pagination metadata
	// For ASF backend: only report TotalCount on first page - subsequent pages have
	// modified queries that return remaining count, not total count
	// For CMR backend: always report TotalCount since it uses native pagination
	limit := searchReq.Limit
	totalCount := result.TotalCount
	if !h.backend.SupportsPagination() && currentCursor != nil {
		totalCount = nil // Don't report inaccurate count on paginated ASF requests
	}
	itemCollection.SetContext(len(itemCollection.Features), limit, totalCount)

	// Add standard links
	baseURL := h.cfg.STAC.BaseURL
	selfURL := fmt.Sprintf("%s/collections/%s/items", baseURL, collectionID)
	itemCollection.AddLink("self", selfURL, "application/geo+json")
	itemCollection.AddLink("root", baseURL+"/", "application/json")
	itemCollection.AddLink("parent", fmt.Sprintf("%s/collections/%s", baseURL, collectionID), "application/json")
	itemCollection.AddLink("collection", fmt.Sprintf("%s/collections/%s", baseURL, collectionID), "application/json")

	// Build pagination links based on backend type
	if h.backend.SupportsPagination() && result.NextCursor != "" {
		// CMR-style: use the cursor from the backend directly
		nextURL := buildNextURLWithCursor(selfURL, r.URL.Query(), result.NextCursor, searchReq.Limit)
		itemCollection.Links = append(itemCollection.Links, &stac.Link{
			Rel:  "next",
			Href: nextURL,
			Type: "application/geo+json",
		})
	} else if !h.backend.SupportsPagination() && len(itemCollection.Features) > 0 {
		// ASF-style: build cursor from item timestamps
		// Generate next link if backend returned a full page (more data likely exists)
		items := extractItemTimeInfos(itemCollection.Features)
		paginationInfo := intstac.CursorPaginationInfo{
			BaseURL:            selfURL,
			Limit:              searchReq.Limit,
			ReturnedCount:      len(itemCollection.Features),
			BackendHasMoreData: backendReturnedFullPage,
			QueryParams:        r.URL.Query(),
			Items:              items,
			CurrentCursor:      currentCursor,
			CursorStore:        h.cursorStore,
		}
		paginationLinks := intstac.BuildCursorPaginationLinks(paginationInfo)
		for _, link := range paginationLinks {
			itemCollection.Links = append(itemCollection.Links, link)
		}
	}

	WriteGeoJSON(w, http.StatusOK, itemCollection)
}

// Item returns a single item by ID from a collection.
// GET /collections/{collectionId}/items/{itemId}
func (h *Handlers) Item(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "collectionId")
	itemID := chi.URLParam(r, "itemId")

	if collectionID == "" {
		WriteBadRequest(w, "collection ID is required")
		return
	}

	if itemID == "" {
		WriteBadRequest(w, "item ID is required")
		return
	}

	// Verify collection exists
	if !h.collections.Has(collectionID) {
		WriteNotFound(w, fmt.Sprintf("collection %q not found", collectionID))
		return
	}

	// Fetch item from backend
	ctx := r.Context()
	item, err := h.backend.GetItem(ctx, collectionID, itemID)
	if err != nil {
		h.logger.Error("failed to fetch item",
			slog.String("collection_id", collectionID),
			slog.String("item_id", itemID),
			slog.String("backend", h.backend.Name()),
			slog.String("error", err.Error()),
		)

		if strings.Contains(err.Error(), "not found") {
			WriteNotFound(w, fmt.Sprintf("item %q not found", itemID))
		} else {
			WriteUpstreamError(w, "upstream service error")
		}
		return
	}

	// Add links
	baseURL := h.cfg.STAC.BaseURL
	item.Links = append(item.Links,
		&stac.Link{
			Rel:  "self",
			Href: fmt.Sprintf("%s/collections/%s/items/%s", baseURL, collectionID, itemID),
			Type: "application/geo+json",
		},
		&stac.Link{
			Rel:  "root",
			Href: baseURL + "/",
			Type: "application/json",
		},
		&stac.Link{
			Rel:  "parent",
			Href: fmt.Sprintf("%s/collections/%s", baseURL, collectionID),
			Type: "application/json",
		},
		&stac.Link{
			Rel:  "collection",
			Href: fmt.Sprintf("%s/collections/%s", baseURL, collectionID),
			Type: "application/json",
		},
	)

	WriteGeoJSON(w, http.StatusOK, item)
}

// Search performs a cross-collection search.
// GET/POST /search
func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.Features.EnableSearch {
		WriteError(w, http.StatusNotImplemented, "NotImplemented", "search endpoint is disabled")
		return
	}

	var searchReq *intstac.SearchRequest
	var err error

	// Parse request based on method
	if r.Method == http.MethodGet {
		searchReq, err = intstac.ParseSearchRequest(r)
	} else if r.Method == http.MethodPost {
		searchReq, err = intstac.ParseSearchRequestBody(r.Body)
		defer r.Body.Close()
	} else {
		WriteBadRequest(w, "method not allowed")
		return
	}

	if err != nil {
		WriteInvalidParameter(w, fmt.Sprintf("invalid search request: %v", err))
		return
	}

	// Apply default limit if not specified
	if searchReq.Limit == 0 {
		searchReq.Limit = h.cfg.Features.DefaultLimit
	}

	// Enforce max limit
	if searchReq.Limit > h.cfg.Features.MaxLimit {
		searchReq.Limit = h.cfg.Features.MaxLimit
	}

	// Decode current cursor if present (needed for filtering duplicates on ASF backend)
	var currentCursor *intstac.Cursor
	if searchReq.Cursor != "" && !h.backend.SupportsPagination() {
		var cursorErr error
		currentCursor, cursorErr = intstac.DecodeCursorWithStore(searchReq.Cursor, h.cursorStore)
		if cursorErr != nil {
			h.logger.Warn("failed to decode cursor",
				slog.String("error", cursorErr.Error()),
				slog.String("cursor", searchReq.Cursor),
			)
			WriteBadRequest(w, fmt.Sprintf("invalid or expired cursor: %s", cursorErr.Error()))
			return
		}
	}

	// Build backend search params
	// For ASF backend with cursor, over-fetch to compensate for SeenIDs filtering
	backendParams := h.buildBackendParams(searchReq, "")
	backendLimit := searchReq.Limit
	if !h.backend.SupportsPagination() && currentCursor != nil && len(currentCursor.SeenIDs) > 0 {
		backendLimit = searchReq.Limit + len(currentCursor.SeenIDs)
		if backendLimit > h.cfg.Features.MaxLimit {
			backendLimit = h.cfg.Features.MaxLimit
		}
		backendParams.Limit = backendLimit
	}

	// Validate collections exist
	for _, collID := range searchReq.Collections {
		if !h.collections.Has(collID) {
			WriteNotFound(w, fmt.Sprintf("collection %q not found", collID))
			return
		}
	}

	// Execute search against backend
	ctx := r.Context()
	result, err := h.backend.Search(ctx, backendParams)
	if err != nil {
		h.logger.Error("backend search failed",
			slog.String("backend", h.backend.Name()),
			slog.String("error", err.Error()),
		)

		if errors.Is(err, translate.ErrCollectionNotFound) {
			WriteNotFound(w, "one or more collections not found")
		} else {
			WriteUpstreamError(w, "upstream search service error")
		}
		return
	}

	// Track if backend returned a full page (used for pagination decision)
	backendReturnedFullPage := len(result.Items) >= backendLimit

	// Build STAC ItemCollection from backend results
	itemCollection := intstac.NewItemCollection(result.Items)

	// Filter out items that were already returned in the previous page (for ASF backend)
	if !h.backend.SupportsPagination() && currentCursor != nil {
		itemCollection.Features = intstac.FilterSeenItems(itemCollection.Features, func(item *intstac.Item) string {
			return item.Id
		}, currentCursor)
	}

	// Trim to requested limit if we over-fetched
	if len(itemCollection.Features) > searchReq.Limit {
		itemCollection.Features = itemCollection.Features[:searchReq.Limit]
	}

	// Set context with pagination metadata
	// For ASF backend: only report TotalCount on first page - subsequent pages have
	// modified queries that return remaining count, not total count
	// For CMR backend: always report TotalCount since it uses native pagination
	limit := searchReq.Limit
	totalCount := result.TotalCount
	if !h.backend.SupportsPagination() && currentCursor != nil {
		totalCount = nil // Don't report inaccurate count on paginated ASF requests
	}
	itemCollection.SetContext(len(itemCollection.Features), limit, totalCount)

	// Add standard links
	baseURL := h.cfg.STAC.BaseURL
	searchURL := baseURL + "/search"
	itemCollection.AddLink("self", searchURL, "application/geo+json")
	itemCollection.AddLink("root", baseURL+"/", "application/json")

	// For POST requests, query params in URL are empty (body contains params)
	// Convert search request to query params for pagination links
	var queryParams url.Values
	if r.Method == http.MethodPost {
		// Use search parameters from request body for pagination links
		queryParams = searchReq.ToQueryParams()
	} else {
		queryParams = r.URL.Query()
	}

	// Build pagination links based on backend type
	if h.backend.SupportsPagination() && result.NextCursor != "" {
		// CMR-style: use the cursor from the backend directly
		nextURL := buildNextURLWithCursor(searchURL, queryParams, result.NextCursor, searchReq.Limit)
		itemCollection.Links = append(itemCollection.Links, &stac.Link{
			Rel:  "next",
			Href: nextURL,
			Type: "application/geo+json",
		})
	} else if !h.backend.SupportsPagination() && len(itemCollection.Features) > 0 {
		// ASF-style: build cursor from item timestamps
		items := extractItemTimeInfos(itemCollection.Features)
		paginationInfo := intstac.CursorPaginationInfo{
			BaseURL:            searchURL,
			Limit:              searchReq.Limit,
			ReturnedCount:      len(itemCollection.Features),
			BackendHasMoreData: backendReturnedFullPage,
			QueryParams:        queryParams,
			Items:              items,
			CurrentCursor:      currentCursor,
			CursorStore:        h.cursorStore,
		}
		paginationLinks := intstac.BuildCursorPaginationLinks(paginationInfo)
		for _, link := range paginationLinks {
			itemCollection.Links = append(itemCollection.Links, link)
		}
	}

	WriteGeoJSON(w, http.StatusOK, itemCollection)
}

// Health returns the health status of the service.
// GET /health
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	// TODO: Could add ASF API connectivity check here
	response := map[string]string{
		"status": "ok",
	}

	WriteJSON(w, http.StatusOK, response)
}

// buildSTACCollection converts a CollectionConfig to a STAC Collection.
func (h *Handlers) buildSTACCollection(cfg *config.CollectionConfig, baseURL string) *stac.Collection {
	collection := intstac.NewCollection(
		cfg.ID,
		cfg.Title,
		cfg.Description,
		h.cfg.STAC.Version,
	)

	collection.License = cfg.License

	// Add providers
	if len(cfg.Providers) > 0 {
		collection.Providers = make([]*stac.Provider, len(cfg.Providers))
		for i, p := range cfg.Providers {
			collection.Providers[i] = &stac.Provider{
				Name:        p.Name,
				Description: p.Description,
				Roles:       p.Roles,
				Url:         p.URL,
			}
		}
	}

	// Set extent
	collection.Extent = &stac.Extent{
		Spatial: &stac.SpatialExtent{
			Bbox: cfg.Extent.Spatial.BBox,
		},
		Temporal: &stac.TemporalExtent{
			Interval: cfg.Extent.Temporal.Interval,
		},
	}

	// Set summaries
	if cfg.Summaries != nil {
		collection.Summaries = cfg.Summaries
	}

	// Note: Extensions are stored as URIs in stac_extensions field via JSON marshaling
	// The go-stac library handles this automatically, so we don't need to set Extensions here

	// Add links
	collection.Links = append(collection.Links,
		&stac.Link{
			Rel:  "self",
			Href: fmt.Sprintf("%s/collections/%s", baseURL, cfg.ID),
			Type: "application/json",
		},
		&stac.Link{
			Rel:  "root",
			Href: baseURL + "/",
			Type: "application/json",
		},
		&stac.Link{
			Rel:  "parent",
			Href: baseURL + "/",
			Type: "application/json",
		},
		&stac.Link{
			Rel:   "items",
			Href:  fmt.Sprintf("%s/collections/%s/items", baseURL, cfg.ID),
			Type:  "application/geo+json",
			Title: "Items",
		},
	)

	return collection
}

// extractItemTimeInfos extracts ID and start_datetime from all items for pagination.
// This is used for cursor-based pagination to track boundary items (items with the same start_datetime).
func extractItemTimeInfos(features []*intstac.Item) []intstac.ItemTimeInfo {
	if len(features) == 0 {
		return nil
	}

	result := make([]intstac.ItemTimeInfo, 0, len(features))
	for _, item := range features {
		if item.Properties == nil {
			continue
		}

		var startTime time.Time
		var found bool

		// Try to get start_datetime first (which maps to ASF's startTime)
		if startDT, ok := item.Properties["start_datetime"].(time.Time); ok {
			startTime = startDT
			found = true
		} else if startDT, ok := item.Properties["start_datetime"].(string); ok && startDT != "" {
			if t, err := time.Parse(time.RFC3339, startDT); err == nil {
				startTime = t
				found = true
			}
		}

		// Fall back to datetime
		if !found {
			if dt, ok := item.Properties["datetime"].(time.Time); ok {
				startTime = dt
				found = true
			} else if dt, ok := item.Properties["datetime"].(string); ok && dt != "" {
				if t, err := time.Parse(time.RFC3339, dt); err == nil {
					startTime = t
					found = true
				}
			}
		}

		if found {
			result = append(result, intstac.ItemTimeInfo{
				ID:        item.Id,
				StartTime: startTime,
			})
		}
	}

	return result
}

// buildBackendParams converts a STAC SearchRequest to backend.SearchParams.
func (h *Handlers) buildBackendParams(req *intstac.SearchRequest, collectionID string) *backend.SearchParams {
	params := &backend.SearchParams{
		Limit: req.Limit,
	}

	// Set collections
	if collectionID != "" {
		params.Collections = []string{collectionID}
	} else if len(req.Collections) > 0 {
		params.Collections = req.Collections
	}

	// Set IDs
	if len(req.IDs) > 0 {
		params.IDs = req.IDs
	}

	// Set spatial filters
	params.BBox = req.BBox
	if len(req.Intersects) > 0 {
		params.Intersects = req.Intersects
	}

	// Parse and set temporal filters
	if req.DateTime != "" {
		start, end, err := translate.ParseDateTimeInterval(req.DateTime)
		if err == nil {
			params.Start = start
			params.End = end
		}
	}

	// Handle cursor - for CMR backend, pass it directly
	// For ASF backend, the cursor is decoded and used to modify End time
	if req.Cursor != "" {
		if h.backend.SupportsPagination() {
			params.Cursor = req.Cursor
		} else {
			// ASF backend: decode cursor and apply to datetime
			cursor, err := intstac.DecodeCursorWithStore(req.Cursor, h.cursorStore)
			if err == nil && cursor != nil {
				params.End = intstac.ApplyCursorToDatetime(cursor, params.End)
			}
		}
	}

	// Set sort
	if len(req.Sortby) > 0 {
		params.SortField = req.Sortby[0].Field
		params.SortDirection = req.Sortby[0].Direction
	}

	// Parse CQL2 filter for SAR-specific parameters
	if req.Filter != nil {
		h.extractFilterParams(req.Filter, params)
	}

	return params
}

// extractFilterParams extracts SAR-specific parameters from CQL2 filter using go-ogc library.
func (h *Handlers) extractFilterParams(filterData interface{}, params *backend.SearchParams) {
	// Marshal the filter back to JSON so we can use go-ogc to parse it
	filterJSON, err := json.Marshal(filterData)
	if err != nil {
		h.logger.Debug("extractFilterParams: failed to marshal filter", slog.String("error", err.Error()))
		return
	}

	// Parse using go-ogc filter library
	var f filter.Filter
	if err := json.Unmarshal(filterJSON, &f); err != nil {
		h.logger.Debug("extractFilterParams: failed to parse CQL2 filter", slog.String("error", err.Error()))
		return
	}

	if f.Expression == nil {
		h.logger.Debug("extractFilterParams: filter has no expression")
		return
	}

	// Extract property values from the parsed filter
	propertyValues := make(map[string][]string)
	extractPropertiesFromExpression(f.Expression, propertyValues)

	h.logger.Debug("extractFilterParams: extracted properties", slog.Any("properties", propertyValues))

	// Map extracted properties to backend params
	if vals, ok := propertyValues["sar:polarizations"]; ok {
		params.Polarization = vals
	}
	if vals, ok := propertyValues["sar:instrument_mode"]; ok {
		params.BeamMode = vals
	}
	if vals, ok := propertyValues["sat:orbit_state"]; ok && len(vals) > 0 {
		params.FlightDirection = vals[0]
	}
	if vals, ok := propertyValues["processing:level"]; ok {
		params.ProcessingLevel = vals
	}
	// sar:product_type maps to processing level (e.g., SLC, GRD, RAW)
	if vals, ok := propertyValues["sar:product_type"]; ok {
		params.ProcessingLevel = vals
	}
	// platform filter (e.g., "sentinel-1a" -> "Sentinel-1A")
	if vals, ok := propertyValues["platform"]; ok {
		normalized := make([]string, len(vals))
		for i, v := range vals {
			normalized[i] = normalizePlatformForASF(v)
		}
		params.Platform = normalized
	}

	h.logger.Debug("extractFilterParams: final params",
		slog.Any("processingLevel", params.ProcessingLevel),
		slog.Any("platform", params.Platform))
}

// extractPropertiesFromExpression recursively extracts property-value pairs from a CQL2 expression.
func extractPropertiesFromExpression(expr filter.Expression, result map[string][]string) {
	switch e := expr.(type) {
	case *filter.And:
		for _, arg := range e.Args {
			extractPropertiesFromExpression(arg, result)
		}
	case *filter.Or:
		for _, arg := range e.Args {
			extractPropertiesFromExpression(arg, result)
		}
	case *filter.Not:
		extractPropertiesFromExpression(e.Arg, result)
	case *filter.Comparison:
		// Handle equality comparisons: property = value
		if e.Name == filter.Equals || e.Name == "eq" {
			propName := getPropertyName(e.Left)
			if propName != "" {
				if val := getStringValue(e.Right); val != "" {
					result[propName] = append(result[propName], val)
				}
			}
		}
	case *filter.In:
		// Handle "in" operator: property IN [value1, value2, ...]
		propName := getPropertyName(e.Item)
		if propName != "" {
			for _, item := range e.List {
				if val := getStringValue(item); val != "" {
					result[propName] = append(result[propName], val)
				}
			}
		}
	}
}

// getPropertyName extracts the property name from a scalar expression.
func getPropertyName(expr filter.ScalarExpression) string {
	if prop, ok := expr.(*filter.Property); ok {
		return prop.Name
	}
	return ""
}

// getStringValue extracts a string value from a scalar expression.
func getStringValue(expr filter.ScalarExpression) string {
	if str, ok := expr.(*filter.String); ok {
		return str.Value
	}
	return ""
}

// normalizePlatformForASF converts STAC lowercase platform names to ASF format.
// STAC items display lowercase platforms (e.g., "sentinel-1a"), but ASF API
// expects specific capitalization (e.g., "Sentinel-1A").
func normalizePlatformForASF(platform string) string {
	lower := strings.ToLower(platform)

	// Known platform mappings (STAC lowercase -> ASF format)
	platformMap := map[string]string{
		"sentinel-1a": "Sentinel-1A",
		"sentinel-1b": "Sentinel-1B",
		"sentinel-1c": "Sentinel-1C",
		"alos":        "ALOS",
		"ers-1":       "ERS-1",
		"ers-2":       "ERS-2",
		"jers-1":      "JERS-1",
		"radarsat-1":  "RADARSAT-1",
		"seasat":      "SEASAT",
		"uavsar":      "UAVSAR",
		"airsar":      "AIRSAR",
	}

	if asfName, ok := platformMap[lower]; ok {
		return asfName
	}

	// If not in map, return original value
	return platform
}

// buildNextURLWithCursor constructs a URL with a cursor parameter for pagination.
func buildNextURLWithCursor(baseURL string, params url.Values, cursor string, limit int) string {
	newParams := url.Values{}
	for key, values := range params {
		if key == "cursor" || key == "page" {
			continue
		}
		for _, value := range values {
			newParams.Add(key, value)
		}
	}

	newParams.Set("cursor", cursor)
	if limit > 0 {
		newParams.Set("limit", fmt.Sprintf("%d", limit))
	}

	return baseURL + "?" + newParams.Encode()
}
