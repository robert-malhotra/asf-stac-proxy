package backend

import (
	"log/slog"
	"os"
	"testing"

	"github.com/robert-malhotra/asf-stac-proxy/internal/config"
)

// createTestASFBackend creates an ASF backend for testing (without actual client)
func createTestASFBackend() *ASFBackend {
	cfg := &config.Config{
		STAC: config.STACConfig{
			BaseURL: "http://test.example.com",
		},
	}

	registry := config.NewCollectionRegistry()
	coll := &config.CollectionConfig{
		ID:                 "sentinel-1-slc",
		Title:              "Sentinel-1 SLC",
		Description:        "Test collection",
		License:            "proprietary",
		ASFDatasets:        []string{"SENTINEL-1"},
		ASFPlatforms:       []string{"Sentinel-1A", "Sentinel-1B"},
		ASFProcessingLevel: "SLC",
		Extent: config.Extent{
			Spatial: config.SpatialExtent{
				BBox: [][]float64{{-180, -90, 180, 90}},
			},
			Temporal: config.TemporalExtent{
				Interval: [][]interface{}{{"2014-04-03T00:00:00Z", nil}},
			},
		},
	}
	_ = registry.Add(coll)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	return &ASFBackend{
		client:      nil, // No client needed for toASFParams tests
		collections: registry,
		translator:  nil,
		cfg:         cfg,
		logger:      logger,
	}
}

func TestASFBackend_toASFParams_IDsOnly(t *testing.T) {
	// Test that when IDs are provided, other search params are NOT included
	// This is required because ASF API doesn't allow combining granule_list with ANY other params
	backend := createTestASFBackend()

	params := &SearchParams{
		Collections: []string{"sentinel-1-slc"},
		IDs:         []string{"S1A_IW_SLC__1SDV_20241130T171452_20241130T171517_056532_06F888_B167-SLC"},
		BBox:        []float64{-180, -90, 180, 90},
		Limit:       10,
	}

	asfParams, err := backend.toASFParams(params)
	if err != nil {
		t.Fatalf("toASFParams failed: %v", err)
	}

	// Verify granule list is set
	if len(asfParams.GranuleList) != 1 {
		t.Errorf("Expected 1 granule in list, got %d", len(asfParams.GranuleList))
	}
	if asfParams.GranuleList[0] != params.IDs[0] {
		t.Errorf("Expected granule %q, got %q", params.IDs[0], asfParams.GranuleList[0])
	}

	// Verify NO other params are set (ASF doesn't allow ANY other params with granule_list)
	if asfParams.MaxResults != 0 {
		t.Errorf("MaxResults should not be set when IDs are provided, got %d", asfParams.MaxResults)
	}
	if len(asfParams.Dataset) > 0 {
		t.Errorf("Dataset should not be set when IDs are provided, got %v", asfParams.Dataset)
	}
	if asfParams.IntersectsWith != "" {
		t.Errorf("IntersectsWith should not be set when IDs are provided, got %q", asfParams.IntersectsWith)
	}
	if len(asfParams.ProcessingLevel) > 0 {
		t.Errorf("ProcessingLevel should not be set when IDs are provided, got %v", asfParams.ProcessingLevel)
	}
}

func TestASFBackend_toASFParams_WithoutIDs(t *testing.T) {
	// Test that when IDs are NOT provided, other params are included normally
	backend := createTestASFBackend()

	params := &SearchParams{
		Collections: []string{"sentinel-1-slc"},
		BBox:        []float64{-180, -90, 180, 90},
		Limit:       10,
	}

	asfParams, err := backend.toASFParams(params)
	if err != nil {
		t.Fatalf("toASFParams failed: %v", err)
	}

	// Verify granule list is NOT set
	if len(asfParams.GranuleList) > 0 {
		t.Errorf("GranuleList should not be set without IDs, got %v", asfParams.GranuleList)
	}

	// Verify dataset is set from collection
	if len(asfParams.Dataset) == 0 {
		t.Error("Dataset should be set from collection")
	}
	if asfParams.Dataset[0] != "SENTINEL-1" {
		t.Errorf("Expected Dataset 'SENTINEL-1', got %v", asfParams.Dataset)
	}

	// Verify spatial filter is set
	if asfParams.IntersectsWith == "" {
		t.Error("IntersectsWith should be set from bbox")
	}

	// Verify processing level is set from collection
	if len(asfParams.ProcessingLevel) == 0 {
		t.Error("ProcessingLevel should be set from collection")
	}
	if asfParams.ProcessingLevel[0] != "SLC" {
		t.Errorf("Expected ProcessingLevel 'SLC', got %v", asfParams.ProcessingLevel)
	}
}

func TestASFBackend_toASFParams_MultipleIDs(t *testing.T) {
	// Test that multiple IDs are all included
	backend := createTestASFBackend()

	params := &SearchParams{
		IDs: []string{
			"S1A_IW_SLC__1SDV_20241130T171452_20241130T171517_056532_06F888_B167-SLC",
			"S1B_IW_SLC__1SDV_20241130T171500_20241130T171527_056533_06F889_A123-SLC",
		},
		Limit: 20,
	}

	asfParams, err := backend.toASFParams(params)
	if err != nil {
		t.Fatalf("toASFParams failed: %v", err)
	}

	if len(asfParams.GranuleList) != 2 {
		t.Errorf("Expected 2 granules in list, got %d", len(asfParams.GranuleList))
	}

	// MaxResults should NOT be set (ASF doesn't allow it with granule_list)
	if asfParams.MaxResults != 0 {
		t.Errorf("MaxResults should not be set when IDs are provided, got %d", asfParams.MaxResults)
	}
}
