package asf

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_Search_Success(t *testing.T) {
	// Create a mock server that returns a valid response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/services/search/param") {
			t.Errorf("Expected path /services/search/param, got %s", r.URL.Path)
		}

		// Return mock response
		response := ASFGeoJSONResponse{
			Type: "FeatureCollection",
			Features: []ASFFeature{
				{
					Type: "Feature",
					Geometry: &Geometry{
						Type:        "Polygon",
						Coordinates: json.RawMessage(`[[[-122.0, 37.0], [-121.0, 37.0], [-121.0, 38.0], [-122.0, 38.0], [-122.0, 37.0]]]`),
					},
					Properties: ASFProperties{
						SceneName:       "S1A_IW_SLC__1SDV_20240101T000000",
						FileID:          "S1A_IW_SLC__1SDV_20240101T000000-SLC",
						Platform:        "Sentinel-1A",
						BeamModeType:    "IW",
						Polarization:    "VV+VH",
						FlightDirection: "ASCENDING",
						ProcessingLevel: "SLC",
						StartTime:       "2024-01-01T00:00:00.000000Z",
						StopTime:        "2024-01-01T00:01:00.000000Z",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, 30*time.Second)

	params := SearchParams{
		Dataset:    []string{"SENTINEL-1"},
		MaxResults: 10,
		Output:     "geojson",
	}

	result, err := client.Search(context.Background(), params)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(result.Features) != 1 {
		t.Errorf("Expected 1 feature, got %d", len(result.Features))
	}

	feature := result.Features[0]
	if feature.Properties.Platform != "Sentinel-1A" {
		t.Errorf("Expected platform Sentinel-1A, got %s", feature.Properties.Platform)
	}
	if feature.Properties.BeamModeType != "IW" {
		t.Errorf("Expected beamModeType IW, got %s", feature.Properties.BeamModeType)
	}
}

func TestClient_Search_WithParams(t *testing.T) {
	// Test that search parameters are correctly passed in the URL
	var capturedURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ASFGeoJSONResponse{Type: "FeatureCollection", Features: []ASFFeature{}})
	}))
	defer server.Close()

	client := NewClient(server.URL, 30*time.Second)

	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)

	params := SearchParams{
		Dataset:         []string{"SENTINEL-1"},
		BeamMode:        []string{"IW", "EW"},
		FlightDirection: "ASCENDING",
		MaxResults:      50,
		Start:           &start,
		End:             &end,
		Output:          "geojson",
	}

	_, err := client.Search(context.Background(), params)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Verify URL contains expected parameters
	// Note: multi-value params like beamMode use separate query params, not comma-separated
	expectedParams := []string{
		"dataset=SENTINEL-1",
		"beamMode=IW",
		"beamMode=EW",
		"flightDirection=ASCENDING",
		"maxResults=50",
		"output=geojson",
	}

	for _, param := range expectedParams {
		if !strings.Contains(capturedURL, param) {
			t.Errorf("URL missing expected parameter '%s': %s", param, capturedURL)
		}
	}
}

func TestClient_Search_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, 30*time.Second)

	params := SearchParams{
		Dataset: []string{"SENTINEL-1"},
		Output:  "geojson",
	}

	_, err := client.Search(context.Background(), params)
	if err == nil {
		t.Fatal("Expected error for 500 response, got nil")
	}

	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Error should contain status code 500: %v", err)
	}
}

func TestClient_Search_NotFoundError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	client := NewClient(server.URL, 30*time.Second)

	params := SearchParams{
		Dataset: []string{"SENTINEL-1"},
		Output:  "geojson",
	}

	_, err := client.Search(context.Background(), params)
	if err == nil {
		t.Fatal("Expected error for 404 response, got nil")
	}

	if !strings.Contains(err.Error(), "404") {
		t.Errorf("Error should contain status code 404: %v", err)
	}
}

func TestClient_Search_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, 30*time.Second)

	params := SearchParams{
		Dataset: []string{"SENTINEL-1"},
		Output:  "geojson",
	}

	_, err := client.Search(context.Background(), params)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}

	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("Error should mention decode failure: %v", err)
	}
}

func TestClient_Search_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than client timeout
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Client with very short timeout
	client := NewClient(server.URL, 50*time.Millisecond)

	params := SearchParams{
		Dataset: []string{"SENTINEL-1"},
		Output:  "geojson",
	}

	_, err := client.Search(context.Background(), params)
	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}
}

func TestClient_Search_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, 30*time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context immediately
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	params := SearchParams{
		Dataset: []string{"SENTINEL-1"},
		Output:  "geojson",
	}

	_, err := client.Search(ctx, params)
	if err == nil {
		t.Fatal("Expected context cancellation error, got nil")
	}
}

func TestClient_GetGranule_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ASFGeoJSONResponse{
			Type: "FeatureCollection",
			Features: []ASFFeature{
				{
					Type: "Feature",
					Properties: ASFProperties{
						SceneName: "S1A_IW_SLC__1SDV_20240101T000000",
						FileID:    "S1A_IW_SLC__1SDV_20240101T000000-SLC",
						Platform:  "Sentinel-1A",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, 30*time.Second)

	result, err := client.GetGranule(context.Background(), "S1A_IW_SLC__1SDV_20240101T000000-SLC")
	if err != nil {
		t.Fatalf("GetGranule failed: %v", err)
	}

	if result.Properties.FileID != "S1A_IW_SLC__1SDV_20240101T000000-SLC" {
		t.Errorf("Expected fileID S1A_IW_SLC__1SDV_20240101T000000-SLC, got %s", result.Properties.FileID)
	}
}

func TestClient_GetGranule_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ASFGeoJSONResponse{
			Type:     "FeatureCollection",
			Features: []ASFFeature{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, 30*time.Second)

	_, err := client.GetGranule(context.Background(), "NONEXISTENT")
	if err == nil {
		t.Fatal("Expected error for not found granule, got nil")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error should mention 'not found': %v", err)
	}
}

func TestClient_GetGranule_MultipleResults_MatchesByFileID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ASFGeoJSONResponse{
			Type: "FeatureCollection",
			Features: []ASFFeature{
				{
					Type: "Feature",
					Properties: ASFProperties{
						SceneName: "S1A_IW_SLC__1SDV_20240101T000000",
						FileID:    "S1A_IW_SLC__1SDV_20240101T000000-RAW",
						Platform:  "Sentinel-1A",
					},
				},
				{
					Type: "Feature",
					Properties: ASFProperties{
						SceneName: "S1A_IW_SLC__1SDV_20240101T000000",
						FileID:    "S1A_IW_SLC__1SDV_20240101T000000-SLC",
						Platform:  "Sentinel-1A",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, 30*time.Second)

	result, err := client.GetGranule(context.Background(), "S1A_IW_SLC__1SDV_20240101T000000-SLC")
	if err != nil {
		t.Fatalf("GetGranule failed: %v", err)
	}

	// Should match the second feature by fileID
	if result.Properties.FileID != "S1A_IW_SLC__1SDV_20240101T000000-SLC" {
		t.Errorf("Expected to match by fileID, got %s", result.Properties.FileID)
	}
}

func TestClient_WithLogger(t *testing.T) {
	client := NewClient("http://example.com", 30*time.Second)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client = client.WithLogger(logger)

	if client.logger != logger {
		t.Error("Logger was not set correctly")
	}
}

func TestSearchParams_ToQueryString(t *testing.T) {
	tests := []struct {
		name           string
		params         SearchParams
		expectedParams []string
		notExpected    []string
	}{
		{
			name: "basic search",
			params: SearchParams{
				Dataset:    []string{"SENTINEL-1"},
				MaxResults: 10,
				Output:     "geojson",
			},
			expectedParams: []string{"dataset=SENTINEL-1", "maxResults=10", "output=geojson"},
		},
		{
			name: "multiple datasets",
			params: SearchParams{
				Dataset: []string{"SENTINEL-1", "ALOS"},
				Output:  "geojson",
			},
			// Multiple values use separate query params, not comma-separated
			expectedParams: []string{"dataset=SENTINEL-1", "dataset=ALOS"},
		},
		{
			name: "beam modes",
			params: SearchParams{
				BeamMode: []string{"IW", "EW"},
				Output:   "geojson",
			},
			// Multiple values use separate query params
			expectedParams: []string{"beamMode=IW", "beamMode=EW"},
		},
		{
			name: "flight direction",
			params: SearchParams{
				FlightDirection: "ASCENDING",
				Output:          "geojson",
			},
			expectedParams: []string{"flightDirection=ASCENDING"},
		},
		{
			name: "processing level",
			params: SearchParams{
				ProcessingLevel: []string{"SLC", "GRD"},
				Output:          "geojson",
			},
			// Processing level uses comma-separated values
			expectedParams: []string{"processingLevel=SLC%2CGRD"},
		},
		{
			name: "relative orbit",
			params: SearchParams{
				RelativeOrbit: []int{10, 20},
				Output:        "geojson",
			},
			// Multiple values use separate query params
			expectedParams: []string{"relativeOrbit=10", "relativeOrbit=20"},
		},
		{
			name: "granule list",
			params: SearchParams{
				GranuleList: []string{"SCENE1", "SCENE2"},
				Output:      "geojson",
			},
			expectedParams: []string{"granule_list=SCENE1%2CSCENE2"},
		},
		{
			name: "intersects with WKT",
			params: SearchParams{
				IntersectsWith: "POLYGON((-122 37,-121 37,-121 38,-122 38,-122 37))",
				Output:         "geojson",
			},
			expectedParams: []string{"intersectsWith="},
		},
		{
			name: "empty params should not appear",
			params: SearchParams{
				Dataset: []string{},
				Output:  "geojson",
			},
			notExpected: []string{"dataset="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queryString := tt.params.ToQueryString()

			for _, expected := range tt.expectedParams {
				if !strings.Contains(queryString, expected) {
					t.Errorf("Query string missing '%s': %s", expected, queryString)
				}
			}

			for _, notExpected := range tt.notExpected {
				if strings.Contains(queryString, notExpected) {
					t.Errorf("Query string should not contain '%s': %s", notExpected, queryString)
				}
			}
		})
	}
}

func TestSearchParams_ToQueryString_WithDates(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)

	params := SearchParams{
		Start:  &start,
		End:    &end,
		Output: "geojson",
	}

	queryString := params.ToQueryString()

	if !strings.Contains(queryString, "start=2024-01-01") {
		t.Errorf("Query string missing start date: %s", queryString)
	}
	if !strings.Contains(queryString, "end=2024-12-31") {
		t.Errorf("Query string missing end date: %s", queryString)
	}
}
