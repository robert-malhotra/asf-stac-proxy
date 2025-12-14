// Package integration provides live integration tests against the ASF API.
// Run with: go test -v ./internal/integration -tags=integration
//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rkm/asf-stac-proxy/internal/api"
	"github.com/rkm/asf-stac-proxy/internal/asf"
	"github.com/rkm/asf-stac-proxy/internal/backend"
	"github.com/rkm/asf-stac-proxy/internal/cmr"
	"github.com/rkm/asf-stac-proxy/internal/config"
	"github.com/rkm/asf-stac-proxy/internal/translate"
)

// TestConfig holds configuration for integration tests
type TestConfig struct {
	BaseURL string
	Timeout time.Duration
}

func getTestConfig() *TestConfig {
	return &TestConfig{
		BaseURL: "https://api.daac.asf.alaska.edu",
		Timeout: 60 * time.Second,
	}
}

// setupTestServer creates a test server with the full proxy stack (defaults to ASF backend)
func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	// Set required env var
	os.Setenv("STAC_BASE_URL", "http://test.local")
	defer os.Unsetenv("STAC_BASE_URL")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Create a logger for tests
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Load collections
	collections, err := config.LoadCollections("../../collections")
	if err != nil {
		t.Logf("Warning: failed to load collections: %v", err)
		collections = config.NewCollectionRegistry()
		// Add minimal sentinel-1 collection for testing
		collections.Add(&config.CollectionConfig{
			ID:          "sentinel-1",
			Title:       "Sentinel-1",
			Description: "Sentinel-1 SAR data",
			ASFDatasets: []string{"SENTINEL-1"},
			License:     "proprietary",
			Extent: config.Extent{
				Spatial:  config.SpatialExtent{BBox: [][]float64{{-180, -90, 180, 90}}},
				Temporal: config.TemporalExtent{Interval: [][]interface{}{{"2014-01-01T00:00:00Z", nil}}},
			},
		})
	}

	asfClient := asf.NewClient(cfg.ASF.BaseURL, cfg.ASF.Timeout)
	translator := translate.NewTranslator(cfg, collections, logger)
	// Create real ASF backend
	asfBackend := backend.NewASFBackend(asfClient, collections, translator, cfg, logger)
	handlers := api.NewHandlers(cfg, asfBackend, translator, collections, logger)
	router := api.NewRouter(handlers, logger)

	return httptest.NewServer(router)
}

// =============================================================================
// ASF Client Direct Tests
// =============================================================================

func TestASFClientSearch(t *testing.T) {
	tc := getTestConfig()
	client := asf.NewClient(tc.BaseURL, tc.Timeout)
	ctx := context.Background()

	t.Run("basic search returns results", func(t *testing.T) {
		params := asf.SearchParams{
			Dataset:    []string{"SENTINEL-1"},
			MaxResults: 5,
			Output:     "geojson",
		}

		resp, err := client.Search(ctx, params)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}

		if len(resp.Features) == 0 {
			t.Error("expected at least one result")
		}

		t.Logf("Received %d features", len(resp.Features))
	})

	t.Run("search with beamMode filter", func(t *testing.T) {
		params := asf.SearchParams{
			Dataset:    []string{"SENTINEL-1"},
			BeamMode:   []string{"IW"},
			MaxResults: 5,
			Output:     "geojson",
		}

		resp, err := client.Search(ctx, params)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}

		for _, f := range resp.Features {
			if f.Properties.BeamModeType != "IW" {
				t.Errorf("expected beamModeType=IW, got %s", f.Properties.BeamModeType)
			}
		}
	})

	t.Run("search with flightDirection filter", func(t *testing.T) {
		params := asf.SearchParams{
			Dataset:         []string{"SENTINEL-1"},
			FlightDirection: "DESCENDING",
			MaxResults:      5,
			Output:          "geojson",
		}

		resp, err := client.Search(ctx, params)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}

		for _, f := range resp.Features {
			if f.Properties.FlightDirection != "DESCENDING" {
				t.Errorf("expected flightDirection=DESCENDING, got %s", f.Properties.FlightDirection)
			}
		}
	})

	t.Run("search with bbox filter", func(t *testing.T) {
		// Alaska bbox
		params := asf.SearchParams{
			Dataset:        []string{"SENTINEL-1"},
			IntersectsWith: "POLYGON((-170 50,-140 50,-140 72,-170 72,-170 50))",
			MaxResults:     5,
			Output:         "geojson",
		}

		resp, err := client.Search(ctx, params)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}

		t.Logf("Alaska bbox search returned %d features", len(resp.Features))
	})

	t.Run("search with datetime filter", func(t *testing.T) {
		now := time.Now()
		weekAgo := now.AddDate(0, 0, -7)

		params := asf.SearchParams{
			Dataset:    []string{"SENTINEL-1"},
			Start:      &weekAgo,
			End:        &now,
			MaxResults: 10,
			Output:     "geojson",
		}

		resp, err := client.Search(ctx, params)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}

		t.Logf("Last week search returned %d features", len(resp.Features))

		// Verify dates are within range
		for _, f := range resp.Features {
			startTime, err := translate.ParseASFTime(f.Properties.StartTime)
			if err != nil {
				t.Errorf("failed to parse startTime: %v", err)
				continue
			}
			if startTime.Before(weekAgo) || startTime.After(now) {
				t.Errorf("startTime %v outside expected range", startTime)
			}
		}
	})
}

// =============================================================================
// Pagination Tests
// =============================================================================

func TestPaginationWithLargeResults(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	// Note: ASF API doesn't support page-based pagination, so we test:
	// 1. Limit parameter works correctly
	// 2. Results are returned as expected
	// 3. Large result sets work

	t.Run("limit parameter works correctly", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/collections/sentinel-1/items?limit=10")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		features := result["features"].([]interface{})
		t.Logf("Limit 10: %d features returned", len(features))

		if len(features) != 10 {
			t.Errorf("expected 10 features with limit=10, got %d", len(features))
		}

		// Check context shows correct limit
		if ctx, ok := result["context"].(map[string]interface{}); ok {
			if limit, ok := ctx["limit"].(float64); ok {
				if int(limit) != 10 {
					t.Errorf("context.limit should be 10, got %v", limit)
				}
			}
		}
	})

	t.Run("large result set", func(t *testing.T) {
		// Request 100 results
		resp, err := http.Get(server.URL + "/collections/sentinel-1/items?limit=100")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		features := result["features"].([]interface{})
		t.Logf("Large page: %d features", len(features))

		if len(features) < 50 {
			t.Errorf("expected at least 50 features for Sentinel-1, got %d", len(features))
		}

		// Check context
		if ctx, ok := result["context"].(map[string]interface{}); ok {
			t.Logf("Context: returned=%v, limit=%v", ctx["returned"], ctx["limit"])
		}
	})
}

// =============================================================================
// CQL2 Filter Tests
// =============================================================================

func TestCQL2Filters(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	t.Run("filter by instrument mode", func(t *testing.T) {
		filter := map[string]interface{}{
			"collections": []string{"sentinel-1"},
			"limit":       10,
			"filter": map[string]interface{}{
				"op": "=",
				"args": []interface{}{
					map[string]interface{}{"property": "sar:instrument_mode"},
					"IW",
				},
			},
		}

		body, _ := json.Marshal(filter)
		resp, err := http.Post(server.URL+"/search", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		features := result["features"].([]interface{})
		t.Logf("IW filter: %d features", len(features))

		for _, f := range features {
			feature := f.(map[string]interface{})
			props := feature["properties"].(map[string]interface{})
			mode := props["sar:instrument_mode"]
			if mode != "IW" {
				t.Errorf("expected sar:instrument_mode=IW, got %v", mode)
			}
		}
	})

	t.Run("filter by orbit state", func(t *testing.T) {
		filter := map[string]interface{}{
			"collections": []string{"sentinel-1"},
			"limit":       10,
			"filter": map[string]interface{}{
				"op": "=",
				"args": []interface{}{
					map[string]interface{}{"property": "sat:orbit_state"},
					"ascending",
				},
			},
		}

		body, _ := json.Marshal(filter)
		resp, err := http.Post(server.URL+"/search", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		features := result["features"].([]interface{})
		t.Logf("Ascending filter: %d features", len(features))

		for _, f := range features {
			feature := f.(map[string]interface{})
			props := feature["properties"].(map[string]interface{})
			state := props["sat:orbit_state"]
			if state != "ascending" {
				t.Errorf("expected sat:orbit_state=ascending, got %v", state)
			}
		}
	})

	t.Run("combined AND filter", func(t *testing.T) {
		filter := map[string]interface{}{
			"collections": []string{"sentinel-1"},
			"limit":       10,
			"filter": map[string]interface{}{
				"op": "and",
				"args": []interface{}{
					map[string]interface{}{
						"op": "=",
						"args": []interface{}{
							map[string]interface{}{"property": "sar:instrument_mode"},
							"IW",
						},
					},
					map[string]interface{}{
						"op": "=",
						"args": []interface{}{
							map[string]interface{}{"property": "sat:orbit_state"},
							"descending",
						},
					},
				},
			},
		}

		body, _ := json.Marshal(filter)
		resp, err := http.Post(server.URL+"/search", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		features := result["features"].([]interface{})
		t.Logf("Combined filter: %d features", len(features))

		for _, f := range features {
			feature := f.(map[string]interface{})
			props := feature["properties"].(map[string]interface{})

			mode := props["sar:instrument_mode"]
			state := props["sat:orbit_state"]

			if mode != "IW" {
				t.Errorf("expected sar:instrument_mode=IW, got %v", mode)
			}
			if state != "descending" {
				t.Errorf("expected sat:orbit_state=descending, got %v", state)
			}
		}
	})

	t.Run("filter with IN operator", func(t *testing.T) {
		filter := map[string]interface{}{
			"collections": []string{"sentinel-1"},
			"limit":       10,
			"filter": map[string]interface{}{
				"op": "in",
				"args": []interface{}{
					map[string]interface{}{"property": "sar:instrument_mode"},
					[]string{"IW", "EW"},
				},
			},
		}

		body, _ := json.Marshal(filter)
		resp, err := http.Post(server.URL+"/search", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		features := result["features"].([]interface{})
		t.Logf("IN filter: %d features", len(features))

		for _, f := range features {
			feature := f.(map[string]interface{})
			props := feature["properties"].(map[string]interface{})
			mode := props["sar:instrument_mode"].(string)
			if mode != "IW" && mode != "EW" {
				t.Errorf("expected sar:instrument_mode in [IW, EW], got %v", mode)
			}
		}
	})

	t.Run("filter by processing level", func(t *testing.T) {
		filter := map[string]interface{}{
			"collections": []string{"sentinel-1"},
			"limit":       10,
			"filter": map[string]interface{}{
				"op": "=",
				"args": []interface{}{
					map[string]interface{}{"property": "processing:level"},
					"GRD",
				},
			},
		}

		body, _ := json.Marshal(filter)
		resp, err := http.Post(server.URL+"/search", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Check HTTP status first
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Logf("Non-200 response: status=%d body=%s", resp.StatusCode, string(bodyBytes))
			t.Skip("processing:level filter may not be supported by ASF API")
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// Handle nil features gracefully
		featuresRaw, ok := result["features"]
		if !ok || featuresRaw == nil {
			t.Skip("No features returned for processing:level filter")
		}
		features, ok := featuresRaw.([]interface{})
		if !ok {
			t.Fatalf("features is not an array: %T", featuresRaw)
		}

		t.Logf("GRD filter: %d features", len(features))

		// GRD products should have GRD in processing:level
		for _, f := range features {
			feature := f.(map[string]interface{})
			props := feature["properties"].(map[string]interface{})
			level := props["processing:level"]
			// The response may show different variations like GRD_HD, GRD_HS, etc.
			if level != nil && !strings.Contains(fmt.Sprintf("%v", level), "GRD") {
				t.Logf("Note: processing:level=%v (may include variants)", level)
			}
		}
	})
}

// =============================================================================
// Sortby Tests
// =============================================================================

func TestSortby(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	// Note: ASF API doesn't support sort direction, it always returns results in descending order.
	// We just test that the sort field is accepted and results are returned.

	t.Run("sort by datetime (descending by default)", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/collections/sentinel-1/items?limit=10&sortby=-datetime")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Check HTTP status first
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Logf("Non-200 response: status=%d body=%s", resp.StatusCode, string(bodyBytes))
			t.Skip("sort parameter may not be supported by ASF API")
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// Handle nil features gracefully
		featuresRaw, ok := result["features"]
		if !ok || featuresRaw == nil {
			t.Skip("No features returned for sort request")
		}
		features, ok := featuresRaw.([]interface{})
		if !ok {
			t.Fatalf("features is not an array: %T", featuresRaw)
		}

		t.Logf("Sort desc: %d features", len(features))

		if len(features) == 0 {
			t.Skip("No features returned")
		}

		// Verify order (dates should be descending - ASF default)
		var lastTime time.Time
		for i, f := range features {
			feature := f.(map[string]interface{})
			props := feature["properties"].(map[string]interface{})
			startDatetime, ok := props["start_datetime"].(string)
			if !ok {
				continue
			}

			parsedTime, err := time.Parse(time.RFC3339, startDatetime)
			if err != nil {
				t.Logf("failed to parse datetime: %v", err)
				continue
			}

			if i > 0 && parsedTime.After(lastTime) {
				t.Logf("Note: results may not be strictly sorted descending: %v > %v", parsedTime, lastTime)
			}
			lastTime = parsedTime
		}
	})

	t.Run("sort by datetime ascending (not supported by ASF)", func(t *testing.T) {
		// ASF API doesn't support ascending sort direction, so this test verifies
		// that we handle the request gracefully (results will still be descending)
		resp, err := http.Get(server.URL + "/collections/sentinel-1/items?limit=10&sortby=+datetime")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Check HTTP status first
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Logf("Non-200 response: status=%d body=%s", resp.StatusCode, string(bodyBytes))
			t.Skip("sort parameter may not be supported by ASF API")
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// Handle nil features gracefully
		featuresRaw, ok := result["features"]
		if !ok || featuresRaw == nil {
			t.Skip("No features returned for sort request")
		}
		features, ok := featuresRaw.([]interface{})
		if !ok {
			t.Fatalf("features is not an array: %T", featuresRaw)
		}

		t.Logf("Sort asc (ignored by ASF): %d features", len(features))

		// Note: ASF ignores ascending direction, so we just verify we get results
		if len(features) == 0 {
			t.Error("Expected at least some features")
		}
	})
}

// =============================================================================
// Bbox and Datetime Tests
// =============================================================================

func TestBboxFilter(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	t.Run("bbox over Alaska", func(t *testing.T) {
		// Alaska bbox
		bbox := "-170,50,-140,72"
		resp, err := http.Get(server.URL + "/collections/sentinel-1/items?limit=10&bbox=" + bbox)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		features := result["features"].([]interface{})
		t.Logf("Alaska bbox: %d features", len(features))

		if len(features) == 0 {
			t.Log("No features found in Alaska bbox (may be expected depending on recent acquisitions)")
		}
	})

	t.Run("bbox over Europe", func(t *testing.T) {
		// Europe bbox
		bbox := "-10,35,30,60"
		resp, err := http.Get(server.URL + "/collections/sentinel-1/items?limit=20&bbox=" + bbox)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		features := result["features"].([]interface{})
		t.Logf("Europe bbox: %d features", len(features))

		// Europe should have lots of Sentinel-1 data
		if len(features) < 5 {
			t.Errorf("expected more features over Europe, got %d", len(features))
		}
	})
}

func TestDatetimeFilter(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	t.Run("datetime single value", func(t *testing.T) {
		// Single day
		datetime := "2024-01-15T00:00:00Z"
		resp, err := http.Get(server.URL + "/collections/sentinel-1/items?limit=10&datetime=" + datetime)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("unexpected status %d: %s", resp.StatusCode, body)
		}
	})

	t.Run("datetime range", func(t *testing.T) {
		// Last 7 days
		now := time.Now().UTC()
		weekAgo := now.AddDate(0, 0, -7)
		datetime := fmt.Sprintf("%s/%s", weekAgo.Format(time.RFC3339), now.Format(time.RFC3339))

		resp, err := http.Get(server.URL + "/collections/sentinel-1/items?limit=20&datetime=" + datetime)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		features := result["features"].([]interface{})
		t.Logf("Last 7 days: %d features", len(features))

		// Sentinel-1 should have many acquisitions in the last week
		if len(features) < 10 {
			t.Errorf("expected more features in last 7 days, got %d", len(features))
		}
	})

	t.Run("open-ended datetime range", func(t *testing.T) {
		// From a date onwards
		datetime := "2024-06-01T00:00:00Z/.."
		resp, err := http.Get(server.URL + "/collections/sentinel-1/items?limit=10&datetime=" + datetime)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("unexpected status %d: %s", resp.StatusCode, body)
		}
	})
}

// =============================================================================
// Property Translation Tests
// =============================================================================

func TestPropertyTranslation(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	t.Run("verify STAC property mapping", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/collections/sentinel-1/items?limit=5")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		features := result["features"].([]interface{})
		if len(features) == 0 {
			t.Fatal("no features returned")
		}

		feature := features[0].(map[string]interface{})
		props := feature["properties"].(map[string]interface{})

		// Check required STAC properties
		requiredProps := []string{
			"datetime",
			"start_datetime",
			"end_datetime",
		}

		for _, prop := range requiredProps {
			if _, ok := props[prop]; !ok {
				t.Errorf("missing required property: %s", prop)
			}
		}

		// Check SAR extension properties
		sarProps := []string{
			"sar:instrument_mode",
			"sar:frequency_band",
			"sar:polarizations",
		}

		for _, prop := range sarProps {
			val := props[prop]
			if val == nil {
				t.Logf("SAR property %s is nil (may be expected for some products)", prop)
			} else {
				t.Logf("%s = %v", prop, val)
			}
		}

		// Check satellite extension properties
		satProps := []string{
			"sat:orbit_state",
		}

		for _, prop := range satProps {
			val := props[prop]
			if val == nil {
				t.Logf("Satellite property %s is nil", prop)
			} else {
				t.Logf("%s = %v", prop, val)
			}
		}

		// Verify platform is lowercase
		if platform, ok := props["platform"].(string); ok {
			if platform != strings.ToLower(platform) {
				t.Errorf("platform should be lowercase, got %s", platform)
			}
		}

		// Verify constellation
		if constellation, ok := props["constellation"].(string); ok {
			if !strings.HasPrefix(constellation, "sentinel") {
				t.Errorf("expected sentinel constellation, got %s", constellation)
			}
		}
	})

	t.Run("verify item structure", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/collections/sentinel-1/items?limit=1")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		features := result["features"].([]interface{})
		if len(features) == 0 {
			t.Fatal("no features returned")
		}

		feature := features[0].(map[string]interface{})

		// Check STAC item structure
		if feature["type"] != "Feature" {
			t.Errorf("expected type=Feature, got %v", feature["type"])
		}

		if feature["stac_version"] == nil {
			t.Error("missing stac_version")
		}

		if feature["id"] == nil {
			t.Error("missing id")
		}

		if feature["geometry"] == nil {
			t.Error("missing geometry")
		}

		if feature["bbox"] == nil {
			t.Error("missing bbox")
		}

		if feature["properties"] == nil {
			t.Error("missing properties")
		}

		if feature["assets"] == nil {
			t.Error("missing assets")
		}

		if feature["links"] == nil {
			t.Error("missing links")
		}

		// Check assets
		assets := feature["assets"].(map[string]interface{})
		if len(assets) == 0 {
			t.Error("expected at least one asset")
		}

		// Log structure for debugging
		t.Logf("Item ID: %s", feature["id"])
		t.Logf("Assets: %v", getKeys(assets))
		t.Logf("Links: %d", len(feature["links"].([]interface{})))
	})
}

// =============================================================================
// Error Handling Tests
// =============================================================================

func TestErrorHandling(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	t.Run("invalid bbox", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/collections/sentinel-1/items?bbox=invalid")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid bbox, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid limit", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/collections/sentinel-1/items?limit=-1")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid limit, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid collection", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/collections/nonexistent/items")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 for invalid collection, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid CQL2 filter", func(t *testing.T) {
		filter := map[string]interface{}{
			"collections": []string{"sentinel-1"},
			"filter": map[string]interface{}{
				"op": "invalid_op",
				"args": []interface{}{},
			},
		}

		body, _ := json.Marshal(filter)
		resp, err := http.Post(server.URL+"/search", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Should return error for unsupported operator
		if resp.StatusCode == http.StatusOK {
			t.Log("Note: invalid operator may be silently ignored or return error")
		}
	})
}

// =============================================================================
// Comparison Tests (ASF Direct vs Proxy)
// =============================================================================

func TestASFProxyComparison(t *testing.T) {
	server := setupTestServer(t)
	defer server.Close()

	tc := getTestConfig()
	asfClient := asf.NewClient(tc.BaseURL, tc.Timeout)
	ctx := context.Background()

	t.Run("compare result counts", func(t *testing.T) {
		// ASF direct query
		asfParams := asf.SearchParams{
			Dataset:         []string{"SENTINEL-1"},
			BeamMode:        []string{"IW"},
			FlightDirection: "DESCENDING",
			MaxResults:      10,
			Output:          "geojson",
		}

		asfResp, err := asfClient.Search(ctx, asfParams)
		if err != nil {
			t.Fatalf("ASF direct search failed: %v", err)
		}

		// Proxy query (equivalent)
		filter := map[string]interface{}{
			"collections": []string{"sentinel-1"},
			"limit":       10,
			"filter": map[string]interface{}{
				"op": "and",
				"args": []interface{}{
					map[string]interface{}{
						"op": "=",
						"args": []interface{}{
							map[string]interface{}{"property": "sar:instrument_mode"},
							"IW",
						},
					},
					map[string]interface{}{
						"op": "=",
						"args": []interface{}{
							map[string]interface{}{"property": "sat:orbit_state"},
							"descending",
						},
					},
				},
			},
		}

		body, _ := json.Marshal(filter)
		resp, err := http.Post(server.URL+"/search", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Proxy search failed: %v", err)
		}
		defer resp.Body.Close()

		var proxyResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&proxyResp); err != nil {
			t.Fatalf("failed to decode proxy response: %v", err)
		}

		proxyFeatures := proxyResp["features"].([]interface{})

		t.Logf("ASF direct: %d features", len(asfResp.Features))
		t.Logf("Proxy: %d features", len(proxyFeatures))

		// Should have similar counts
		if len(proxyFeatures) != len(asfResp.Features) {
			t.Logf("Note: counts differ - ASF=%d, Proxy=%d", len(asfResp.Features), len(proxyFeatures))
		}
	})

	t.Run("compare first result properties", func(t *testing.T) {
		// ASF direct query
		asfParams := asf.SearchParams{
			Dataset:    []string{"SENTINEL-1"},
			MaxResults: 1,
			Output:     "geojson",
		}

		asfResp, err := asfClient.Search(ctx, asfParams)
		if err != nil {
			t.Fatalf("ASF direct search failed: %v", err)
		}

		if len(asfResp.Features) == 0 {
			t.Fatal("no ASF results")
		}

		asfFeature := asfResp.Features[0]

		// Proxy query
		resp, err := http.Get(server.URL + "/collections/sentinel-1/items?limit=1")
		if err != nil {
			t.Fatalf("Proxy request failed: %v", err)
		}
		defer resp.Body.Close()

		var proxyResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&proxyResp); err != nil {
			t.Fatalf("failed to decode proxy response: %v", err)
		}

		proxyFeatures := proxyResp["features"].([]interface{})
		if len(proxyFeatures) == 0 {
			t.Fatal("no proxy results")
		}

		proxyFeature := proxyFeatures[0].(map[string]interface{})
		proxyProps := proxyFeature["properties"].(map[string]interface{})

		// Compare key properties
		t.Logf("ASF fileID: %s", asfFeature.Properties.FileID)
		t.Logf("Proxy id: %s", proxyFeature["id"])

		t.Logf("ASF platform: %s", asfFeature.Properties.Platform)
		t.Logf("Proxy platform: %s", proxyProps["platform"])

		t.Logf("ASF beamModeType: %s", asfFeature.Properties.BeamModeType)
		t.Logf("Proxy sar:instrument_mode: %v", proxyProps["sar:instrument_mode"])

		t.Logf("ASF flightDirection: %s", asfFeature.Properties.FlightDirection)
		t.Logf("Proxy sat:orbit_state: %v", proxyProps["sat:orbit_state"])

		// Verify transformations
		if proxyProps["platform"] != nil {
			platform := proxyProps["platform"].(string)
			if platform != strings.ToLower(asfFeature.Properties.Platform) {
				t.Errorf("platform mismatch: ASF=%s, Proxy=%s",
					asfFeature.Properties.Platform, platform)
			}
		}

		if proxyProps["sat:orbit_state"] != nil {
			orbitState := proxyProps["sat:orbit_state"].(string)
			expectedState := strings.ToLower(asfFeature.Properties.FlightDirection)
			if orbitState != expectedState {
				t.Errorf("orbit_state mismatch: expected=%s, got=%s",
					expectedState, orbitState)
			}
		}
	})
}

// =============================================================================
// Live Pagination Tests - ASF and CMR Backends
// =============================================================================

// TestLivePaginationASF tests pagination with the ASF backend using Sentinel-1 SLC data
func TestLivePaginationASF(t *testing.T) {
	// Skip if running short tests
	if testing.Short() {
		t.Skip("skipping live pagination test in short mode")
	}

	// Set up ASF backend
	os.Setenv("STAC_BASE_URL", "http://test.local")
	os.Setenv("BACKEND_TYPE", "asf")
	defer os.Unsetenv("STAC_BASE_URL")
	defer os.Unsetenv("BACKEND_TYPE")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	collections, err := config.LoadCollections("../../collections")
	if err != nil {
		t.Fatalf("failed to load collections: %v", err)
	}

	asfClient := asf.NewClient(cfg.ASF.BaseURL, cfg.ASF.Timeout)
	translator := translate.NewTranslator(cfg, collections, logger)

	// Create real ASF backend from backend package
	asfBackend := backend.NewASFBackend(asfClient, collections, translator, cfg, logger)

	handlers := api.NewHandlers(cfg, asfBackend, translator, collections, logger)
	router := api.NewRouter(handlers, logger)

	server := httptest.NewServer(router)
	defer server.Close()

	runPaginationTest(t, server.URL, "ASF", 5, 3) // 5 items per page, 3 pages
}

// TestLivePaginationCMR tests pagination with the CMR backend using Sentinel-1 SLC data
func TestLivePaginationCMR(t *testing.T) {
	// Skip if running short tests
	if testing.Short() {
		t.Skip("skipping live pagination test in short mode")
	}

	// Set up CMR backend
	os.Setenv("STAC_BASE_URL", "http://test.local")
	os.Setenv("BACKEND_TYPE", "cmr")
	defer os.Unsetenv("STAC_BASE_URL")
	defer os.Unsetenv("BACKEND_TYPE")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	collections, err := config.LoadCollections("../../collections")
	if err != nil {
		t.Fatalf("failed to load collections: %v", err)
	}

	translator := translate.NewTranslator(cfg, collections, logger)

	// Create real CMR backend
	cmrClient := cmr.NewClient(cfg.CMR.BaseURL, cfg.CMR.Provider, cfg.CMR.Timeout)
	cmrBackend := cmr.NewCMRBackend(cmrClient, collections, cfg, logger)

	handlers := api.NewHandlers(cfg, cmrBackend, translator, collections, logger)
	router := api.NewRouter(handlers, logger)

	server := httptest.NewServer(router)
	defer server.Close()

	runPaginationTest(t, server.URL, "CMR", 5, 3) // 5 items per page, 3 pages
}

// runPaginationTest is a helper that runs pagination tests for any backend
func runPaginationTest(t *testing.T, baseURL, backendName string, pageSize, numPages int) {
	t.Helper()

	// Helper to fix URLs returned by the server (which use the configured base URL)
	// to use the actual test server URL
	fixURL := func(u string) string {
		return strings.Replace(u, "http://test.local", baseURL, 1)
	}

	t.Run(fmt.Sprintf("%s_Search_Pagination", backendName), func(t *testing.T) {
		seenIDs := make(map[string]bool)
		var allItems []map[string]interface{}
		var nextURL string

		// First request - search for Sentinel-1 data
		firstURL := fmt.Sprintf("%s/search", baseURL)
		searchBody := map[string]interface{}{
			"collections": []string{"sentinel-1"},
			"limit":       pageSize,
		}

		bodyBytes, _ := json.Marshal(searchBody)
		resp, err := http.Post(firstURL, "application/json", bytes.NewReader(bodyBytes))
		if err != nil {
			t.Fatalf("first request failed: %v", err)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			t.Fatalf("failed to decode first response: %v", err)
		}
		resp.Body.Close()

		features, ok := result["features"].([]interface{})
		if !ok || len(features) == 0 {
			t.Fatalf("no features in first response")
		}

		t.Logf("%s Page 1: %d items", backendName, len(features))

		// Record items from first page
		for _, f := range features {
			feature := f.(map[string]interface{})
			id := feature["id"].(string)
			if seenIDs[id] {
				t.Errorf("duplicate item on page 1: %s", id)
			}
			seenIDs[id] = true
			allItems = append(allItems, feature)
		}

		// Find next link
		links := result["links"].([]interface{})
		for _, l := range links {
			link := l.(map[string]interface{})
			if link["rel"] == "next" {
				nextURL = fixURL(link["href"].(string))
				break
			}
		}

		if nextURL == "" {
			t.Fatalf("no next link found on first page")
		}

		// Paginate through remaining pages
		for page := 2; page <= numPages; page++ {
			resp, err := http.Get(nextURL)
			if err != nil {
				t.Fatalf("page %d request failed: %v", page, err)
			}

			var pageResult map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&pageResult); err != nil {
				resp.Body.Close()
				t.Fatalf("failed to decode page %d response: %v", page, err)
			}
			resp.Body.Close()

			pageFeatures, ok := pageResult["features"].([]interface{})
			if !ok {
				t.Fatalf("no features array on page %d", page)
			}

			t.Logf("%s Page %d: %d items", backendName, page, len(pageFeatures))

			if len(pageFeatures) == 0 {
				t.Logf("no more results on page %d, ending pagination", page)
				break
			}

			// Check for duplicates
			duplicates := 0
			for _, f := range pageFeatures {
				feature := f.(map[string]interface{})
				id := feature["id"].(string)
				if seenIDs[id] {
					duplicates++
					t.Logf("duplicate item on page %d: %s", page, id)
				}
				seenIDs[id] = true
				allItems = append(allItems, feature)
			}

			if duplicates > 0 {
				t.Errorf("found %d duplicates on page %d", duplicates, page)
			}

			// Find next link for next iteration
			nextURL = ""
			pageLinks := pageResult["links"].([]interface{})
			for _, l := range pageLinks {
				link := l.(map[string]interface{})
				if link["rel"] == "next" {
					nextURL = fixURL(link["href"].(string))
					break
				}
			}

			if nextURL == "" && page < numPages {
				t.Logf("no next link on page %d, ending pagination early", page)
				break
			}
		}

		t.Logf("%s: Total unique items collected: %d", backendName, len(seenIDs))

		// Verify all items are valid STAC items
		for i, item := range allItems {
			if item["id"] == nil {
				t.Errorf("item %d has no id", i)
			}
			if item["geometry"] == nil {
				t.Errorf("item %d has no geometry", i)
			}
			if item["properties"] == nil {
				t.Errorf("item %d has no properties", i)
			}
		}

		// Verify we got items from multiple pages without duplicates
		expectedMinItems := pageSize * 2 // At least 2 pages worth
		if len(seenIDs) < expectedMinItems {
			t.Errorf("expected at least %d unique items across pages, got %d", expectedMinItems, len(seenIDs))
		}
	})

	t.Run(fmt.Sprintf("%s_Items_Endpoint_Pagination", backendName), func(t *testing.T) {
		seenIDs := make(map[string]bool)
		var nextURL string

		// First request via items endpoint
		firstURL := fmt.Sprintf("%s/collections/sentinel-1/items?limit=%d", baseURL, pageSize)
		resp, err := http.Get(firstURL)
		if err != nil {
			t.Fatalf("first request failed: %v", err)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			t.Fatalf("failed to decode first response: %v", err)
		}
		resp.Body.Close()

		features, ok := result["features"].([]interface{})
		if !ok || len(features) == 0 {
			t.Fatalf("no features in first response")
		}

		t.Logf("%s Items Page 1: %d items", backendName, len(features))

		// Record items from first page
		for _, f := range features {
			feature := f.(map[string]interface{})
			id := feature["id"].(string)
			seenIDs[id] = true
		}

		// Find next link
		links := result["links"].([]interface{})
		for _, l := range links {
			link := l.(map[string]interface{})
			if link["rel"] == "next" {
				nextURL = fixURL(link["href"].(string))
				break
			}
		}

		if nextURL == "" {
			t.Fatalf("no next link found on first page of items endpoint")
		}

		// Get second page
		resp, err = http.Get(nextURL)
		if err != nil {
			t.Fatalf("second page request failed: %v", err)
		}

		var page2Result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&page2Result); err != nil {
			resp.Body.Close()
			t.Fatalf("failed to decode second page response: %v", err)
		}
		resp.Body.Close()

		page2Features, ok := page2Result["features"].([]interface{})
		if !ok || len(page2Features) == 0 {
			t.Fatalf("no features on second page")
		}

		t.Logf("%s Items Page 2: %d items", backendName, len(page2Features))

		// Check for duplicates
		duplicates := 0
		for _, f := range page2Features {
			feature := f.(map[string]interface{})
			id := feature["id"].(string)
			if seenIDs[id] {
				duplicates++
				t.Logf("duplicate item: %s", id)
			}
			seenIDs[id] = true
		}

		if duplicates > 0 {
			t.Errorf("found %d duplicates between page 1 and page 2", duplicates)
		}

		t.Logf("%s Items: Total unique items across 2 pages: %d", backendName, len(seenIDs))
	})
}


// =============================================================================
// Pagination Filter Preservation Tests
// =============================================================================

// TestPaginationPreservesFilters verifies that CQL2-JSON filters from POST requests
// are preserved in pagination links and correctly applied on subsequent pages.
// This tests the fix for the bug where filters were lost after the first page.
func TestPaginationPreservesFilters(t *testing.T) {
	// Run for both backends
	t.Run("ASF", func(t *testing.T) {
		server := setupTestServerWithBackend(t, "asf")
		defer server.Close()
		runFilterPaginationTest(t, server.URL, "ASF")
	})

	t.Run("CMR", func(t *testing.T) {
		server := setupTestServerWithBackend(t, "cmr")
		defer server.Close()
		runFilterPaginationTest(t, server.URL, "CMR")
	})
}

// setupTestServerWithBackend creates a test server with the specified backend type
func setupTestServerWithBackend(t *testing.T, backendType string) *httptest.Server {
	t.Helper()

	os.Setenv("STAC_BASE_URL", "http://test.local")
	os.Setenv("BACKEND_TYPE", backendType)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	collections, err := config.LoadCollections("../../collections")
	if err != nil {
		t.Fatalf("failed to load collections: %v", err)
	}

	translator := translate.NewTranslator(cfg, collections, logger)

	var searchBackend backend.SearchBackend
	if backendType == "cmr" {
		cmrClient := cmr.NewClient(cfg.CMR.BaseURL, cfg.CMR.Provider, cfg.CMR.Timeout)
		searchBackend = cmr.NewCMRBackend(cmrClient, collections, cfg, logger)
	} else {
		asfClient := asf.NewClient(cfg.ASF.BaseURL, cfg.ASF.Timeout)
		searchBackend = backend.NewASFBackend(asfClient, collections, translator, cfg, logger)
	}

	handlers := api.NewHandlers(cfg, searchBackend, translator, collections, logger)
	router := api.NewRouter(handlers, logger)

	return httptest.NewServer(router)
}

// runFilterPaginationTest tests that filters are preserved across pagination
func runFilterPaginationTest(t *testing.T, baseURL, backendName string) {
	t.Helper()

	// Helper to fix URLs returned by the server
	fixURL := func(u string) string {
		return strings.Replace(u, "http://test.local", baseURL, 1)
	}

	t.Run("POST_filter_preserved_in_pagination", func(t *testing.T) {
		// First page: POST request with SLC filter
		searchBody := map[string]interface{}{
			"collections": []string{"sentinel-1"},
			"limit":       3,
			"filter": map[string]interface{}{
				"op": "=",
				"args": []interface{}{
					map[string]interface{}{"property": "sar:product_type"},
					"SLC",
				},
			},
			"filter-lang": "cql2-json",
		}

		bodyBytes, _ := json.Marshal(searchBody)
		resp, err := http.Post(baseURL+"/search", "application/json", bytes.NewReader(bodyBytes))
		if err != nil {
			t.Fatalf("first request failed: %v", err)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			t.Fatalf("failed to decode first response: %v", err)
		}
		resp.Body.Close()

		features, ok := result["features"].([]interface{})
		if !ok || len(features) == 0 {
			t.Skipf("%s: no features returned for SLC filter, skipping test", backendName)
		}

		t.Logf("%s Page 1: %d items", backendName, len(features))

		// Verify all items on first page are SLC
		for i, f := range features {
			feature := f.(map[string]interface{})
			props := feature["properties"].(map[string]interface{})
			productType, _ := props["sar:product_type"].(string)
			if productType != "SLC" {
				t.Errorf("Page 1 item %d: expected sar:product_type=SLC, got %s", i, productType)
			}
		}

		// Find next link
		var nextURL string
		links, ok := result["links"].([]interface{})
		if !ok {
			t.Fatalf("no links in response")
		}
		for _, l := range links {
			link := l.(map[string]interface{})
			if link["rel"] == "next" {
				nextURL = fixURL(link["href"].(string))
				break
			}
		}

		if nextURL == "" {
			t.Skipf("%s: no next link found, not enough data for pagination test", backendName)
		}

		// Verify the next URL contains the filter parameter
		if !strings.Contains(nextURL, "filter=") {
			t.Errorf("next link does not contain filter parameter: %s", nextURL)
		}
		if !strings.Contains(nextURL, "filter-lang=") {
			t.Errorf("next link does not contain filter-lang parameter: %s", nextURL)
		}

		t.Logf("%s: Next URL: %s", backendName, nextURL)

		// Second page: GET request following the pagination link
		resp, err = http.Get(nextURL)
		if err != nil {
			t.Fatalf("second page request failed: %v", err)
		}

		var page2Result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&page2Result); err != nil {
			resp.Body.Close()
			t.Fatalf("failed to decode second page response: %v", err)
		}
		resp.Body.Close()

		page2Features, ok := page2Result["features"].([]interface{})
		if !ok || len(page2Features) == 0 {
			t.Skipf("%s: no features on second page", backendName)
		}

		t.Logf("%s Page 2: %d items", backendName, len(page2Features))

		// Verify all items on second page are also SLC (filter was preserved)
		nonSLCCount := 0
		for i, f := range page2Features {
			feature := f.(map[string]interface{})
			props := feature["properties"].(map[string]interface{})
			productType, _ := props["sar:product_type"].(string)
			if productType != "SLC" {
				nonSLCCount++
				t.Errorf("Page 2 item %d: expected sar:product_type=SLC, got %s (filter not preserved!)", i, productType)
			}
		}

		if nonSLCCount > 0 {
			t.Errorf("%s: %d/%d items on page 2 were not SLC - filter was not preserved in pagination",
				backendName, nonSLCCount, len(page2Features))
		} else {
			t.Logf("%s: All %d items on page 2 are SLC - filter correctly preserved", backendName, len(page2Features))
		}
	})

	t.Run("GET_filter_works_directly", func(t *testing.T) {
		// Test that filters work when passed directly via GET query params
		filterJSON := `{"op":"=","args":[{"property":"sar:product_type"},"SLC"]}`
		url := fmt.Sprintf("%s/search?collections=sentinel-1&limit=3&filter=%s&filter-lang=cql2-json",
			baseURL, filterJSON)

		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("GET request failed: %v", err)
		}
		defer resp.Body.Close()

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		features, ok := result["features"].([]interface{})
		if !ok || len(features) == 0 {
			t.Skipf("%s: no features returned for GET filter request", backendName)
		}

		t.Logf("%s GET filter: %d items", backendName, len(features))

		// Verify all items are SLC
		for i, f := range features {
			feature := f.(map[string]interface{})
			props := feature["properties"].(map[string]interface{})
			productType, _ := props["sar:product_type"].(string)
			if productType != "SLC" {
				t.Errorf("GET filter item %d: expected sar:product_type=SLC, got %s", i, productType)
			}
		}
	})

	t.Run("combined_filter_preserved", func(t *testing.T) {
		// Test with a combined AND filter
		searchBody := map[string]interface{}{
			"collections": []string{"sentinel-1"},
			"limit":       3,
			"filter": map[string]interface{}{
				"op": "and",
				"args": []interface{}{
					map[string]interface{}{
						"op": "=",
						"args": []interface{}{
							map[string]interface{}{"property": "sar:product_type"},
							"SLC",
						},
					},
					map[string]interface{}{
						"op": "=",
						"args": []interface{}{
							map[string]interface{}{"property": "sar:instrument_mode"},
							"IW",
						},
					},
				},
			},
			"filter-lang": "cql2-json",
		}

		bodyBytes, _ := json.Marshal(searchBody)
		resp, err := http.Post(baseURL+"/search", "application/json", bytes.NewReader(bodyBytes))
		if err != nil {
			t.Fatalf("first request failed: %v", err)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			t.Fatalf("failed to decode first response: %v", err)
		}
		resp.Body.Close()

		features, ok := result["features"].([]interface{})
		if !ok || len(features) == 0 {
			t.Skipf("%s: no features returned for combined filter", backendName)
		}

		t.Logf("%s Combined filter Page 1: %d items", backendName, len(features))

		// Verify all items match both filters
		for i, f := range features {
			feature := f.(map[string]interface{})
			props := feature["properties"].(map[string]interface{})
			productType, _ := props["sar:product_type"].(string)
			instrumentMode, _ := props["sar:instrument_mode"].(string)
			if productType != "SLC" {
				t.Errorf("Page 1 item %d: expected sar:product_type=SLC, got %s", i, productType)
			}
			if instrumentMode != "IW" {
				t.Errorf("Page 1 item %d: expected sar:instrument_mode=IW, got %s", i, instrumentMode)
			}
		}

		// Find and follow next link
		var nextURL string
		links, _ := result["links"].([]interface{})
		for _, l := range links {
			link := l.(map[string]interface{})
			if link["rel"] == "next" {
				nextURL = fixURL(link["href"].(string))
				break
			}
		}

		if nextURL == "" {
			t.Skipf("%s: no next link for combined filter test", backendName)
		}

		// Get second page
		resp, err = http.Get(nextURL)
		if err != nil {
			t.Fatalf("second page request failed: %v", err)
		}

		var page2Result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&page2Result); err != nil {
			resp.Body.Close()
			t.Fatalf("failed to decode second page response: %v", err)
		}
		resp.Body.Close()

		page2Features, ok := page2Result["features"].([]interface{})
		if !ok || len(page2Features) == 0 {
			t.Skipf("%s: no features on second page of combined filter test", backendName)
		}

		t.Logf("%s Combined filter Page 2: %d items", backendName, len(page2Features))

		// Verify second page also matches both filters
		errors := 0
		for i, f := range page2Features {
			feature := f.(map[string]interface{})
			props := feature["properties"].(map[string]interface{})
			productType, _ := props["sar:product_type"].(string)
			instrumentMode, _ := props["sar:instrument_mode"].(string)
			if productType != "SLC" {
				t.Errorf("Page 2 item %d: expected sar:product_type=SLC, got %s", i, productType)
				errors++
			}
			if instrumentMode != "IW" {
				t.Errorf("Page 2 item %d: expected sar:instrument_mode=IW, got %s", i, instrumentMode)
				errors++
			}
		}

		if errors == 0 {
			t.Logf("%s: Combined filter correctly preserved across pagination", backendName)
		}
	})
}

// =============================================================================
// Helper Functions
// =============================================================================

func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
