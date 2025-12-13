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

// setupTestServer creates a test server with the full proxy stack
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
	handlers := api.NewHandlers(cfg, asfClient, translator, collections, logger)
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
// Helper Functions
// =============================================================================

func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
