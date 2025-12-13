package cmr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSearchParams_ToURLValues(t *testing.T) {
	tests := []struct {
		name     string
		params   *SearchParams
		contains []string
	}{
		{
			name: "basic params",
			params: &SearchParams{
				ShortName: []string{"SENTINEL-1A_SLC"},
				PageSize:  100,
			},
			contains: []string{
				"short_name=SENTINEL-1A_SLC",
				"page_size=100",
			},
		},
		{
			name: "spatial params",
			params: &SearchParams{
				BoundingBox: "-180,-90,180,90",
				PageSize:    250,
			},
			contains: []string{
				"bounding_box=-180%2C-90%2C180%2C90",
			},
		},
		{
			name: "temporal params",
			params: &SearchParams{
				Temporal: "2020-01-01T00:00:00Z,2020-12-31T23:59:59Z",
				PageSize: 250,
			},
			contains: []string{
				"temporal=2020-01-01T00",
			},
		},
		{
			name: "attribute filters",
			params: &SearchParams{
				Polarization: []string{"VV", "VH"},
				BeamMode:     []string{"IW"},
				PageSize:     250,
			},
			contains: []string{
				"attribute%5B%5D=string%2CPOLARIZATION%2CVV",
				"attribute%5B%5D=string%2CBEAM_MODE%2CIW",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := tt.params.ToURLValues()
			encoded := values.Encode()

			for _, want := range tt.contains {
				if !contains(encoded, want) {
					t.Errorf("ToURLValues() = %s, want to contain %s", encoded, want)
				}
			}
		})
	}
}

func TestClient_Search(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.URL.Path != "/granules.umm_json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Check provider parameter
		if r.URL.Query().Get("provider") != "ASF" {
			t.Errorf("expected provider ASF, got %s", r.URL.Query().Get("provider"))
		}

		// Return mock response
		resp := UMMSearchResponse{
			Hits: 1,
			Took: 100,
			Items: []UMMResultItem{
				{
					Meta: UMMMeta{
						ConceptID:  "G123456-ASF",
						ProviderID: "ASF",
					},
					UMM: UMMGranule{
						GranuleUR: "S1A_IW_SLC__1SDV_20200101_123456",
						CollectionReference: CollectionReference{
							ShortName: "SENTINEL-1A_SLC",
							Version:   "1",
						},
						TemporalExtent: &TemporalExtent{
							RangeDateTime: &RangeDateTime{
								BeginningDateTime: "2020-01-01T00:00:00.000Z",
								EndingDateTime:    "2020-01-01T00:01:00.000Z",
							},
						},
					},
				},
			},
		}

		// Add CMR-Search-After header for pagination
		w.Header().Set(CMRSearchAfterHeader, "next-cursor-value")
		w.Header().Set("Content-Type", "application/vnd.nasa.cmr.umm_results+json")

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create client
	client := NewClient(server.URL, "ASF", 30*time.Second)

	// Execute search
	result, err := client.Search(context.Background(), &SearchParams{
		ShortName: []string{"SENTINEL-1A_SLC"},
		PageSize:  10,
	})

	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if result.Hits != 1 {
		t.Errorf("Search() hits = %d, want 1", result.Hits)
	}

	if len(result.Granules) != 1 {
		t.Errorf("Search() granules = %d, want 1", len(result.Granules))
	}

	if result.SearchAfter != "next-cursor-value" {
		t.Errorf("Search() SearchAfter = %s, want next-cursor-value", result.SearchAfter)
	}

	if result.Granules[0].GranuleUR != "S1A_IW_SLC__1SDV_20200101_123456" {
		t.Errorf("Search() GranuleUR = %s, want S1A_IW_SLC__1SDV_20200101_123456", result.Granules[0].GranuleUR)
	}
}

func TestClient_GetGranule(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		granuleUR := r.URL.Query().Get("granule_ur")
		if granuleUR != "S1A_TEST_GRANULE" {
			t.Errorf("expected granule_ur S1A_TEST_GRANULE, got %s", granuleUR)
		}

		resp := UMMSearchResponse{
			Hits: 1,
			Items: []UMMResultItem{
				{
					UMM: UMMGranule{
						GranuleUR: "S1A_TEST_GRANULE",
						CollectionReference: CollectionReference{
							ShortName: "SENTINEL-1A_SLC",
						},
					},
				},
			},
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "ASF", 30*time.Second)

	granule, err := client.GetGranule(context.Background(), "S1A_TEST_GRANULE")
	if err != nil {
		t.Fatalf("GetGranule() error = %v", err)
	}

	if granule.GranuleUR != "S1A_TEST_GRANULE" {
		t.Errorf("GetGranule() GranuleUR = %s, want S1A_TEST_GRANULE", granule.GranuleUR)
	}
}

func TestClient_GetGranule_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := UMMSearchResponse{
			Hits:  0,
			Items: []UMMResultItem{},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "ASF", 30*time.Second)

	_, err := client.GetGranule(context.Background(), "NONEXISTENT")
	if err == nil {
		t.Error("GetGranule() expected error for not found, got nil")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
