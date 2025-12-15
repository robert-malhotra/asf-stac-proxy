package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/robert-malhotra/asf-stac-proxy/internal/config"
	"github.com/robert-malhotra/asf-stac-proxy/internal/stac"
	"github.com/robert-malhotra/asf-stac-proxy/internal/translate"
)

// createQueryablesTestCollections creates a collection registry with multiple collections
// that have different summaries for testing queryables
func createQueryablesTestCollections() *config.CollectionRegistry {
	registry := config.NewCollectionRegistry()

	// Sentinel-1 SLC collection
	sentinel1SLC := &config.CollectionConfig{
		ID:          "sentinel-1-slc",
		Title:       "Sentinel-1 SLC",
		Description: "Sentinel-1 Single Look Complex",
		License:     "proprietary",
		ASFDatasets: []string{"SENTINEL-1"},
		Extent: config.Extent{
			Spatial: config.SpatialExtent{
				BBox: [][]float64{{-180, -90, 180, 90}},
			},
			Temporal: config.TemporalExtent{
				Interval: [][]interface{}{{"2014-04-03T00:00:00Z", nil}},
			},
		},
		Summaries: map[string]interface{}{
			"platform":            []interface{}{"sentinel-1a", "sentinel-1b", "sentinel-1c"},
			"instruments":         []interface{}{"c-sar"},
			"constellation":       []interface{}{"sentinel-1"},
			"sar:instrument_mode": []interface{}{"IW", "EW", "SM", "WV"},
			"sar:frequency_band":  []interface{}{"C"},
			"sar:polarizations":   []interface{}{[]interface{}{"VV"}, []interface{}{"VH"}, []interface{}{"VV", "VH"}, []interface{}{"HH"}, []interface{}{"HV"}, []interface{}{"HH", "HV"}},
			"sar:product_type":    []interface{}{"SLC"},
			"sat:orbit_state":     []interface{}{"ascending", "descending"},
			"processing:level":    []interface{}{"L1"},
		},
	}

	// ALOS PALSAR collection
	alosPalsar := &config.CollectionConfig{
		ID:          "alos-palsar-l1-0",
		Title:       "ALOS PALSAR L1.0",
		Description: "ALOS PALSAR Level 1.0",
		License:     "proprietary",
		ASFDatasets: []string{"ALOS"},
		Extent: config.Extent{
			Spatial: config.SpatialExtent{
				BBox: [][]float64{{-180, -90, 180, 90}},
			},
			Temporal: config.TemporalExtent{
				Interval: [][]interface{}{{"2006-05-16T00:00:00Z", "2011-04-22T00:00:00Z"}},
			},
		},
		Summaries: map[string]interface{}{
			"platform":            []interface{}{"alos"},
			"instruments":         []interface{}{"palsar"},
			"sar:instrument_mode": []interface{}{"FBS", "FBD", "PLR", "WB1", "WB2"},
			"sar:frequency_band":  []interface{}{"L"},
			"sar:polarizations":   []interface{}{[]interface{}{"HH"}, []interface{}{"HV"}, []interface{}{"VV"}, []interface{}{"VH"}, []interface{}{"HH", "HV"}, []interface{}{"VV", "VH"}},
			"sar:product_type":    []interface{}{"L1.0"},
			"sat:orbit_state":     []interface{}{"ascending", "descending"},
			"processing:level":    []interface{}{"L1"},
		},
	}

	// ERS collection
	ers := &config.CollectionConfig{
		ID:          "ers-l0",
		Title:       "ERS L0",
		Description: "ERS Level 0",
		License:     "proprietary",
		ASFDatasets: []string{"ERS-1", "ERS-2"},
		Extent: config.Extent{
			Spatial: config.SpatialExtent{
				BBox: [][]float64{{-180, -90, 180, 90}},
			},
			Temporal: config.TemporalExtent{
				Interval: [][]interface{}{{"1991-07-17T00:00:00Z", "2011-07-04T00:00:00Z"}},
			},
		},
		Summaries: map[string]interface{}{
			"platform":            []interface{}{"ers-1", "ers-2"},
			"instruments":         []interface{}{"sar"},
			"sar:instrument_mode": []interface{}{"STD"},
			"sar:frequency_band":  []interface{}{"C"},
			"sar:polarizations":   []interface{}{[]interface{}{"VV"}},
			"sar:product_type":    []interface{}{"L0"},
			"sat:orbit_state":     []interface{}{"ascending", "descending"},
			"processing:level":    []interface{}{"L0"},
		},
	}

	_ = registry.Add(sentinel1SLC)
	_ = registry.Add(alosPalsar)
	_ = registry.Add(ers)
	return registry
}

// createQueryablesTestHandlers creates handlers for queryables testing
func createQueryablesTestHandlers() (*Handlers, *stac.MemoryCursorStore) {
	cfg := &config.Config{
		STAC: config.STACConfig{
			BaseURL: "http://test.example.com",
		},
		Features: config.FeatureConfig{
			DefaultLimit:     10,
			MaxLimit:         250,
			EnableQueryables: true,
		},
	}
	collections := createQueryablesTestCollections()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	translator := translate.NewTranslator(cfg, collections, logger)
	cursorStore := stac.NewMemoryCursorStore(time.Hour, 5*time.Minute)

	mock := &mockBackend{
		items:              nil,
		supportsPagination: false,
	}

	return NewHandlers(cfg, mock, translator, collections, logger).WithCursorStore(cursorStore), cursorStore
}

// Queryables response structure for testing
type QueryablesResponse struct {
	Schema               string                            `json:"$schema"`
	ID                   string                            `json:"$id"`
	Type                 string                            `json:"type"`
	Title                string                            `json:"title"`
	Description          string                            `json:"description"`
	Properties           map[string]map[string]interface{} `json:"properties"`
	AdditionalProperties bool                              `json:"additionalProperties"`
}

func TestHandlers_Queryables_CollectionSpecific_PlatformEnum(t *testing.T) {
	handlers, cursorStore := createQueryablesTestHandlers()
	defer cursorStore.Stop()

	// Test sentinel-1-slc collection - should have sentinel-1a, sentinel-1b, sentinel-1c
	req := httptest.NewRequest("GET", "/collections/sentinel-1-slc/queryables", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("collectionId", "sentinel-1-slc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handlers.Queryables(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result QueryablesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Verify platform enum
	platformProp := result.Properties["platform"]
	if platformProp == nil {
		t.Fatal("Expected platform property in queryables")
	}

	platformEnum, ok := platformProp["enum"].([]interface{})
	if !ok {
		t.Fatalf("Expected platform to have enum, got: %v", platformProp)
	}

	expectedPlatforms := []string{"sentinel-1a", "sentinel-1b", "sentinel-1c"}
	if len(platformEnum) != len(expectedPlatforms) {
		t.Errorf("Expected %d platform values, got %d: %v", len(expectedPlatforms), len(platformEnum), platformEnum)
	}

	for _, expected := range expectedPlatforms {
		found := false
		for _, actual := range platformEnum {
			if actual.(string) == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected platform enum to contain %q, got %v", expected, platformEnum)
		}
	}
}

func TestHandlers_Queryables_CollectionSpecific_InstrumentModeEnum(t *testing.T) {
	handlers, cursorStore := createQueryablesTestHandlers()
	defer cursorStore.Stop()

	req := httptest.NewRequest("GET", "/collections/sentinel-1-slc/queryables", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("collectionId", "sentinel-1-slc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handlers.Queryables(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var result QueryablesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	modeProp := result.Properties["sar:instrument_mode"]
	if modeProp == nil {
		t.Fatal("Expected sar:instrument_mode property")
	}

	modeEnum, ok := modeProp["enum"].([]interface{})
	if !ok {
		t.Fatalf("Expected sar:instrument_mode to have enum")
	}

	expectedModes := []string{"IW", "EW", "SM", "WV"}
	if len(modeEnum) != len(expectedModes) {
		t.Errorf("Expected %d instrument modes, got %d: %v", len(expectedModes), len(modeEnum), modeEnum)
	}
}

func TestHandlers_Queryables_CollectionSpecific_PolarizationsFlattened(t *testing.T) {
	handlers, cursorStore := createQueryablesTestHandlers()
	defer cursorStore.Stop()

	req := httptest.NewRequest("GET", "/collections/sentinel-1-slc/queryables", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("collectionId", "sentinel-1-slc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handlers.Queryables(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var result QueryablesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	polProp := result.Properties["sar:polarizations"]
	if polProp == nil {
		t.Fatal("Expected sar:polarizations property")
	}

	// The enum should be on items, not on the property itself
	items, ok := polProp["items"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected sar:polarizations to have items property")
	}

	polEnum, ok := items["enum"].([]interface{})
	if !ok {
		t.Fatalf("Expected sar:polarizations items to have enum, got: %v", items)
	}

	// Should be flattened to individual channels: VV, VH, HH, HV
	expectedChannels := map[string]bool{"VV": true, "VH": true, "HH": true, "HV": true}
	if len(polEnum) != len(expectedChannels) {
		t.Errorf("Expected %d polarization channels, got %d: %v", len(expectedChannels), len(polEnum), polEnum)
	}

	for _, ch := range polEnum {
		if !expectedChannels[ch.(string)] {
			t.Errorf("Unexpected polarization channel: %v", ch)
		}
	}
}

func TestHandlers_Queryables_Global_AggregatesPlatforms(t *testing.T) {
	handlers, cursorStore := createQueryablesTestHandlers()
	defer cursorStore.Stop()

	// Global queryables (no collection ID)
	req := httptest.NewRequest("GET", "/queryables", nil)
	w := httptest.NewRecorder()
	handlers.Queryables(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result QueryablesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	platformProp := result.Properties["platform"]
	if platformProp == nil {
		t.Fatal("Expected platform property in global queryables")
	}

	platformEnum, ok := platformProp["enum"].([]interface{})
	if !ok {
		t.Fatalf("Expected platform to have enum in global queryables")
	}

	// Should contain platforms from all collections: sentinel-1a, sentinel-1b, sentinel-1c, alos, ers-1, ers-2
	expectedPlatforms := map[string]bool{
		"sentinel-1a": true,
		"sentinel-1b": true,
		"sentinel-1c": true,
		"alos":        true,
		"ers-1":       true,
		"ers-2":       true,
	}

	if len(platformEnum) != len(expectedPlatforms) {
		t.Errorf("Expected %d platforms in global queryables, got %d: %v",
			len(expectedPlatforms), len(platformEnum), platformEnum)
	}

	for _, p := range platformEnum {
		if !expectedPlatforms[p.(string)] {
			t.Errorf("Unexpected platform in global queryables: %v", p)
		}
	}
}

func TestHandlers_Queryables_Global_AggregatesFrequencyBands(t *testing.T) {
	handlers, cursorStore := createQueryablesTestHandlers()
	defer cursorStore.Stop()

	req := httptest.NewRequest("GET", "/queryables", nil)
	w := httptest.NewRecorder()
	handlers.Queryables(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var result QueryablesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	bandProp := result.Properties["sar:frequency_band"]
	if bandProp == nil {
		t.Fatal("Expected sar:frequency_band property")
	}

	bandEnum, ok := bandProp["enum"].([]interface{})
	if !ok {
		t.Fatalf("Expected sar:frequency_band to have enum")
	}

	// Should have both C and L bands
	bands := make([]string, len(bandEnum))
	for i, b := range bandEnum {
		bands[i] = b.(string)
	}
	sort.Strings(bands)

	expected := []string{"C", "L"}
	sort.Strings(expected)

	if len(bands) != len(expected) {
		t.Errorf("Expected bands %v, got %v", expected, bands)
	}
}

func TestHandlers_Queryables_Global_AggregatesInstrumentModes(t *testing.T) {
	handlers, cursorStore := createQueryablesTestHandlers()
	defer cursorStore.Stop()

	req := httptest.NewRequest("GET", "/queryables", nil)
	w := httptest.NewRecorder()
	handlers.Queryables(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var result QueryablesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	modeProp := result.Properties["sar:instrument_mode"]
	if modeProp == nil {
		t.Fatal("Expected sar:instrument_mode property")
	}

	modeEnum, ok := modeProp["enum"].([]interface{})
	if !ok {
		t.Fatalf("Expected sar:instrument_mode to have enum")
	}

	// Should include modes from all collections
	expectedModes := map[string]bool{
		"IW": true, "EW": true, "SM": true, "WV": true, // Sentinel-1
		"FBS": true, "FBD": true, "PLR": true, "WB1": true, "WB2": true, // ALOS
		"STD": true, // ERS
	}

	if len(modeEnum) != len(expectedModes) {
		t.Errorf("Expected %d instrument modes, got %d: %v",
			len(expectedModes), len(modeEnum), modeEnum)
	}

	for _, m := range modeEnum {
		if !expectedModes[m.(string)] {
			t.Errorf("Unexpected instrument mode in global queryables: %v", m)
		}
	}
}

func TestHandlers_Queryables_Global_AggregatesPolarizations(t *testing.T) {
	handlers, cursorStore := createQueryablesTestHandlers()
	defer cursorStore.Stop()

	req := httptest.NewRequest("GET", "/queryables", nil)
	w := httptest.NewRecorder()
	handlers.Queryables(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var result QueryablesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	polProp := result.Properties["sar:polarizations"]
	if polProp == nil {
		t.Fatal("Expected sar:polarizations property")
	}

	items, ok := polProp["items"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected sar:polarizations to have items property")
	}

	polEnum, ok := items["enum"].([]interface{})
	if !ok {
		t.Fatalf("Expected sar:polarizations items to have enum")
	}

	// Should have all channels: VV, VH, HH, HV
	expectedChannels := map[string]bool{"VV": true, "VH": true, "HH": true, "HV": true}

	if len(polEnum) != len(expectedChannels) {
		t.Errorf("Expected %d polarization channels, got %d: %v",
			len(expectedChannels), len(polEnum), polEnum)
	}

	for _, ch := range polEnum {
		if !expectedChannels[ch.(string)] {
			t.Errorf("Unexpected polarization channel in global queryables: %v", ch)
		}
	}
}

func TestHandlers_Queryables_Global_AggregatesProductTypes(t *testing.T) {
	handlers, cursorStore := createQueryablesTestHandlers()
	defer cursorStore.Stop()

	req := httptest.NewRequest("GET", "/queryables", nil)
	w := httptest.NewRecorder()
	handlers.Queryables(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var result QueryablesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	productProp := result.Properties["sar:product_type"]
	if productProp == nil {
		t.Fatal("Expected sar:product_type property")
	}

	productEnum, ok := productProp["enum"].([]interface{})
	if !ok {
		t.Fatalf("Expected sar:product_type to have enum")
	}

	// Should include product types from all collections
	expectedProducts := map[string]bool{
		"SLC":  true, // Sentinel-1
		"L1.0": true, // ALOS
		"L0":   true, // ERS
	}

	if len(productEnum) != len(expectedProducts) {
		t.Errorf("Expected %d product types, got %d: %v",
			len(expectedProducts), len(productEnum), productEnum)
	}
}

func TestHandlers_Queryables_CollectionNotFound(t *testing.T) {
	handlers, cursorStore := createQueryablesTestHandlers()
	defer cursorStore.Stop()

	req := httptest.NewRequest("GET", "/collections/nonexistent/queryables", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("collectionId", "nonexistent")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handlers.Queryables(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for nonexistent collection, got %d", w.Code)
	}
}

func TestHandlers_Queryables_HasExpectedFields(t *testing.T) {
	handlers, cursorStore := createQueryablesTestHandlers()
	defer cursorStore.Stop()

	req := httptest.NewRequest("GET", "/queryables", nil)
	w := httptest.NewRecorder()
	handlers.Queryables(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var result QueryablesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Verify all expected fields are present
	expectedFields := []string{
		"datetime",
		"bbox",
		"intersects",
		"ids",
		"collections",
		"sar:instrument_mode",
		"sar:polarizations",
		"sar:frequency_band",
		"sar:product_type",
		"sat:orbit_state",
		"sat:relative_orbit",
		"sat:absolute_orbit",
		"processing:level",
		"platform",
		"constellation",
		"instruments",
	}

	for _, field := range expectedFields {
		if result.Properties[field] == nil {
			t.Errorf("Expected field %q in queryables", field)
		}
	}
}

func TestHandlers_Queryables_CollectionSpecific_DifferentPlatforms(t *testing.T) {
	handlers, cursorStore := createQueryablesTestHandlers()
	defer cursorStore.Stop()

	testCases := []struct {
		collectionID      string
		expectedPlatforms []string
	}{
		{
			collectionID:      "sentinel-1-slc",
			expectedPlatforms: []string{"sentinel-1a", "sentinel-1b", "sentinel-1c"},
		},
		{
			collectionID:      "alos-palsar-l1-0",
			expectedPlatforms: []string{"alos"},
		},
		{
			collectionID:      "ers-l0",
			expectedPlatforms: []string{"ers-1", "ers-2"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.collectionID, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/collections/"+tc.collectionID+"/queryables", nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("collectionId", tc.collectionID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()
			handlers.Queryables(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("Expected status 200, got %d", w.Code)
			}

			var result QueryablesResponse
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			platformProp := result.Properties["platform"]
			platformEnum, ok := platformProp["enum"].([]interface{})
			if !ok {
				t.Fatalf("Expected platform to have enum for %s", tc.collectionID)
			}

			if len(platformEnum) != len(tc.expectedPlatforms) {
				t.Errorf("Expected %d platforms for %s, got %d: %v",
					len(tc.expectedPlatforms), tc.collectionID, len(platformEnum), platformEnum)
			}

			for _, expected := range tc.expectedPlatforms {
				found := false
				for _, actual := range platformEnum {
					if actual.(string) == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected platform %q for %s, got %v",
						expected, tc.collectionID, platformEnum)
				}
			}
		})
	}
}

func TestHandlers_Queryables_ResponseFormat(t *testing.T) {
	handlers, cursorStore := createQueryablesTestHandlers()
	defer cursorStore.Stop()

	req := httptest.NewRequest("GET", "/queryables", nil)
	w := httptest.NewRecorder()
	handlers.Queryables(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var result QueryablesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Verify JSON Schema format
	if result.Schema != "https://json-schema.org/draft/2019-09/schema" {
		t.Errorf("Expected JSON Schema draft 2019-09, got %s", result.Schema)
	}

	if result.Type != "object" {
		t.Errorf("Expected type 'object', got %s", result.Type)
	}

	if result.ID != "http://test.example.com/queryables" {
		t.Errorf("Expected ID 'http://test.example.com/queryables', got %s", result.ID)
	}

	if !result.AdditionalProperties {
		t.Error("Expected additionalProperties to be true")
	}
}

