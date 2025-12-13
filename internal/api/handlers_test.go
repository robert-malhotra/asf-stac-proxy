package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	gostac "github.com/planetlabs/go-stac"
	"github.com/rkm/asf-stac-proxy/internal/backend"
	"github.com/rkm/asf-stac-proxy/internal/config"
	"github.com/rkm/asf-stac-proxy/internal/stac"
	"github.com/rkm/asf-stac-proxy/internal/translate"
)

// mockBackend is a test backend that returns configurable results
type mockBackend struct {
	items              []*gostac.Item
	supportsPagination bool
	searchCalls        []backend.SearchParams // Record of search calls for verification
}

func (m *mockBackend) Search(ctx context.Context, params *backend.SearchParams) (*backend.SearchResult, error) {
	// Record the call
	m.searchCalls = append(m.searchCalls, *params)

	// Return up to params.Limit items
	end := params.Limit
	if end > len(m.items) {
		end = len(m.items)
	}

	return &backend.SearchResult{
		Items: m.items[:end],
	}, nil
}

func (m *mockBackend) GetItem(ctx context.Context, collection, itemID string) (*gostac.Item, error) {
	for _, item := range m.items {
		if item.Id == itemID {
			return item, nil
		}
	}
	return nil, fmt.Errorf("item not found: %s", itemID)
}

func (m *mockBackend) Name() string {
	return "mock"
}

func (m *mockBackend) SupportsPagination() bool {
	return m.supportsPagination
}

// createTestItem creates a STAC item for testing
func createTestItem(id string, startTime time.Time) *gostac.Item {
	geom := map[string]interface{}{
		"type":        "Point",
		"coordinates": []float64{0, 0},
	}
	geomJSON, _ := json.Marshal(geom)

	return &gostac.Item{
		Version:    "1.0.0",
		Id:         id,
		Collection: "sentinel-1",
		Geometry:   geomJSON,
		Bbox:       []float64{-1, -1, 1, 1},
		Properties: map[string]interface{}{
			"datetime":       startTime.Format(time.RFC3339),
			"start_datetime": startTime.Format(time.RFC3339),
			"end_datetime":   startTime.Add(time.Hour).Format(time.RFC3339),
		},
		Assets: map[string]*gostac.Asset{},
		Links:  []*gostac.Link{},
	}
}

// createTestConfig creates a config for testing
func createTestConfig() *config.Config {
	return &config.Config{
		STAC: config.STACConfig{
			BaseURL: "http://test.example.com",
		},
		Features: config.FeatureConfig{
			DefaultLimit: 10,
			MaxLimit:     250,
		},
	}
}

// createTestCollections creates a minimal collection registry for testing
func createTestCollections() *config.CollectionRegistry {
	registry := config.NewCollectionRegistry()

	coll := &config.CollectionConfig{
		ID:          "sentinel-1",
		Title:       "Sentinel-1",
		Description: "Test collection",
		License:     "proprietary",
		ASFDatasets: []string{"SENTINEL-1"},
		Extent: config.Extent{
			Spatial: config.SpatialExtent{
				BBox: [][]float64{{-180, -90, 180, 90}},
			},
		},
	}

	_ = registry.Add(coll)
	return registry
}

func TestHandlers_Items_OverFetchWithSeenIDs(t *testing.T) {
	// Test that when a cursor has SeenIDs, we request extra items from backend
	// to compensate for filtering

	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create 150 items all with same timestamp
	items := make([]*gostac.Item, 150)
	for i := 0; i < 150; i++ {
		items[i] = createTestItem(fmt.Sprintf("item-%03d", i), baseTime)
	}

	mock := &mockBackend{
		items:              items,
		supportsPagination: false,
	}

	cfg := createTestConfig()
	cfg.Features.DefaultLimit = 100
	collections := createTestCollections()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	translator := translate.NewTranslator(cfg, collections, logger)
	cursorStore := stac.NewMemoryCursorStore(time.Hour, 5*time.Minute)
	defer cursorStore.Stop()

	handlers := NewHandlers(cfg, mock, translator, collections, logger).WithCursorStore(cursorStore)

	// Create a cursor with 20 SeenIDs (simulating second page)
	cursor := &stac.Cursor{
		StartTime: baseTime.Format(time.RFC3339),
		Direction: "next",
		SeenIDs:   make([]string, 20),
	}
	for i := 0; i < 20; i++ {
		cursor.SeenIDs[i] = fmt.Sprintf("item-%03d", i)
	}
	encodedCursor := stac.EncodeCursor(cursor)

	// Make request with cursor
	req := httptest.NewRequest("GET", "/collections/sentinel-1/items?limit=100&cursor="+encodedCursor, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("collectionId", "sentinel-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handlers.Items(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify backend was called with over-fetch limit
	if len(mock.searchCalls) != 1 {
		t.Fatalf("Expected 1 search call, got %d", len(mock.searchCalls))
	}

	// Should have requested 100 + 20 = 120 items (over-fetch)
	expectedLimit := 120
	if mock.searchCalls[0].Limit != expectedLimit {
		t.Errorf("Expected backend limit %d (over-fetch), got %d", expectedLimit, mock.searchCalls[0].Limit)
	}
}

func TestHandlers_Items_TrimsToRequestedLimit(t *testing.T) {
	// Test that after over-fetching and filtering, results are trimmed to requested limit

	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create 150 items
	items := make([]*gostac.Item, 150)
	for i := 0; i < 150; i++ {
		items[i] = createTestItem(fmt.Sprintf("item-%03d", i), baseTime)
	}

	mock := &mockBackend{
		items:              items,
		supportsPagination: false,
	}

	cfg := createTestConfig()
	cfg.Features.DefaultLimit = 100
	collections := createTestCollections()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	translator := translate.NewTranslator(cfg, collections, logger)
	cursorStore := stac.NewMemoryCursorStore(time.Hour, 5*time.Minute)
	defer cursorStore.Stop()

	handlers := NewHandlers(cfg, mock, translator, collections, logger).WithCursorStore(cursorStore)

	// Create cursor with 10 SeenIDs
	cursor := &stac.Cursor{
		StartTime: baseTime.Format(time.RFC3339),
		Direction: "next",
		SeenIDs:   []string{"item-000", "item-001", "item-002", "item-003", "item-004", "item-005", "item-006", "item-007", "item-008", "item-009"},
	}
	encodedCursor := stac.EncodeCursor(cursor)

	req := httptest.NewRequest("GET", "/collections/sentinel-1/items?limit=100&cursor="+encodedCursor, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("collectionId", "sentinel-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handlers.Items(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Parse response
	var result stac.ItemCollection
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should return exactly 100 items (the requested limit), not 110 (over-fetch minus 10 filtered)
	if len(result.Features) != 100 {
		t.Errorf("Expected 100 items (requested limit), got %d", len(result.Features))
	}

	// Verify the first items are NOT in SeenIDs (were filtered)
	for _, item := range result.Features {
		for _, seenID := range cursor.SeenIDs {
			if item.Id == seenID {
				t.Errorf("Item %s should have been filtered out (in SeenIDs)", item.Id)
			}
		}
	}
}

func TestHandlers_Items_NextLinkWithBackendHasMoreData(t *testing.T) {
	// Test that "next" link is generated when backend has more data,
	// even if filtered count is less than limit

	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create exactly 110 items (over-fetch amount for limit=100 with 10 SeenIDs)
	items := make([]*gostac.Item, 110)
	for i := 0; i < 110; i++ {
		items[i] = createTestItem(fmt.Sprintf("item-%03d", i), baseTime)
	}

	mock := &mockBackend{
		items:              items,
		supportsPagination: false,
	}

	cfg := createTestConfig()
	cfg.Features.DefaultLimit = 100
	collections := createTestCollections()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	translator := translate.NewTranslator(cfg, collections, logger)
	cursorStore := stac.NewMemoryCursorStore(time.Hour, 5*time.Minute)
	defer cursorStore.Stop()

	handlers := NewHandlers(cfg, mock, translator, collections, logger).WithCursorStore(cursorStore)

	// Create cursor with 10 SeenIDs
	cursor := &stac.Cursor{
		StartTime: baseTime.Format(time.RFC3339),
		Direction: "next",
		SeenIDs:   []string{"item-000", "item-001", "item-002", "item-003", "item-004", "item-005", "item-006", "item-007", "item-008", "item-009"},
	}
	encodedCursor := stac.EncodeCursor(cursor)

	req := httptest.NewRequest("GET", "/collections/sentinel-1/items?limit=100&cursor="+encodedCursor, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("collectionId", "sentinel-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handlers.Items(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Parse response
	var result stac.ItemCollection
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Check for "next" link - backend returned full 110 items (over-fetch amount),
	// so there should be a next link
	hasNextLink := false
	for _, link := range result.Links {
		if link.Rel == "next" {
			hasNextLink = true
			break
		}
	}

	if !hasNextLink {
		t.Error("Expected 'next' link when backend returned full over-fetch amount")
	}
}

func TestHandlers_Items_NoNextLinkOnLastPage(t *testing.T) {
	// Test that no "next" link is generated when backend returns less than requested

	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create only 50 items (less than limit of 100)
	items := make([]*gostac.Item, 50)
	for i := 0; i < 50; i++ {
		items[i] = createTestItem(fmt.Sprintf("item-%03d", i), baseTime)
	}

	mock := &mockBackend{
		items:              items,
		supportsPagination: false,
	}

	cfg := createTestConfig()
	cfg.Features.DefaultLimit = 100
	collections := createTestCollections()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	translator := translate.NewTranslator(cfg, collections, logger)
	cursorStore := stac.NewMemoryCursorStore(time.Hour, 5*time.Minute)
	defer cursorStore.Stop()

	handlers := NewHandlers(cfg, mock, translator, collections, logger).WithCursorStore(cursorStore)

	// First page request (no cursor)
	req := httptest.NewRequest("GET", "/collections/sentinel-1/items?limit=100", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("collectionId", "sentinel-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handlers.Items(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result stac.ItemCollection
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should NOT have "next" link since backend returned less than limit
	for _, link := range result.Links {
		if link.Rel == "next" {
			t.Error("Should not have 'next' link when backend returned less than limit")
		}
	}
}

func TestHandlers_Search_OverFetchWithSeenIDs(t *testing.T) {
	// Same test as Items but for the Search endpoint

	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	items := make([]*gostac.Item, 150)
	for i := 0; i < 150; i++ {
		items[i] = createTestItem(fmt.Sprintf("item-%03d", i), baseTime)
	}

	mock := &mockBackend{
		items:              items,
		supportsPagination: false,
	}

	cfg := createTestConfig()
	cfg.Features.DefaultLimit = 100
	cfg.Features.EnableSearch = true // Enable search endpoint
	collections := createTestCollections()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	translator := translate.NewTranslator(cfg, collections, logger)
	cursorStore := stac.NewMemoryCursorStore(time.Hour, 5*time.Minute)
	defer cursorStore.Stop()

	handlers := NewHandlers(cfg, mock, translator, collections, logger).WithCursorStore(cursorStore)

	// Create cursor with 30 SeenIDs
	cursor := &stac.Cursor{
		StartTime: baseTime.Format(time.RFC3339),
		Direction: "next",
		SeenIDs:   make([]string, 30),
	}
	for i := 0; i < 30; i++ {
		cursor.SeenIDs[i] = fmt.Sprintf("item-%03d", i)
	}
	encodedCursor := stac.EncodeCursor(cursor)

	// POST search with cursor
	body := fmt.Sprintf(`{"collections":["sentinel-1"],"limit":100,"cursor":"%s"}`, encodedCursor)
	req := httptest.NewRequest("POST", "/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handlers.Search(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify backend was called with over-fetch limit
	if len(mock.searchCalls) != 1 {
		t.Fatalf("Expected 1 search call, got %d", len(mock.searchCalls))
	}

	// Should have requested 100 + 30 = 130 items
	expectedLimit := 130
	if mock.searchCalls[0].Limit != expectedLimit {
		t.Errorf("Expected backend limit %d (over-fetch), got %d", expectedLimit, mock.searchCalls[0].Limit)
	}
}

func TestHandlers_Items_OverFetchCappedAtMaxLimit(t *testing.T) {
	// Test that over-fetch doesn't exceed MaxLimit

	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	items := make([]*gostac.Item, 300)
	for i := 0; i < 300; i++ {
		items[i] = createTestItem(fmt.Sprintf("item-%03d", i), baseTime)
	}

	mock := &mockBackend{
		items:              items,
		supportsPagination: false,
	}

	cfg := createTestConfig()
	cfg.Features.DefaultLimit = 100
	cfg.Features.MaxLimit = 250 // Set max limit

	collections := createTestCollections()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	translator := translate.NewTranslator(cfg, collections, logger)
	cursorStore := stac.NewMemoryCursorStore(time.Hour, 5*time.Minute)
	defer cursorStore.Stop()

	handlers := NewHandlers(cfg, mock, translator, collections, logger).WithCursorStore(cursorStore)

	// Create cursor with 100 SeenIDs - would push over-fetch to 350, but should cap at 250
	cursor := &stac.Cursor{
		StartTime: baseTime.Format(time.RFC3339),
		Direction: "next",
		SeenIDs:   make([]string, 100),
	}
	for i := 0; i < 100; i++ {
		cursor.SeenIDs[i] = fmt.Sprintf("item-%03d", i)
	}
	encodedCursor := stac.EncodeCursor(cursor)

	req := httptest.NewRequest("GET", "/collections/sentinel-1/items?limit=250&cursor="+url.QueryEscape(encodedCursor), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("collectionId", "sentinel-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handlers.Items(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify backend limit was capped at MaxLimit
	if len(mock.searchCalls) != 1 {
		t.Fatalf("Expected 1 search call, got %d", len(mock.searchCalls))
	}

	if mock.searchCalls[0].Limit != 250 {
		t.Errorf("Expected backend limit to be capped at MaxLimit (250), got %d", mock.searchCalls[0].Limit)
	}
}
