package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCollections(t *testing.T) {
	// Create temporary directory for test collections
	tmpDir := t.TempDir()

	// Create a test collection file
	collection := CollectionConfig{
		ID:          "test-collection",
		Title:       "Test Collection",
		Description: "A test collection",
		ASFDatasets: []string{"TEST-DATASET"},
		License:     "proprietary",
		Extent: Extent{
			Spatial: SpatialExtent{
				BBox: [][]float64{{-180, -90, 180, 90}},
			},
			Temporal: TemporalExtent{
				Interval: [][]interface{}{{"2020-01-01T00:00:00Z", nil}},
			},
		},
	}

	data, err := json.MarshalIndent(collection, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal test collection: %v", err)
	}

	collectionFile := filepath.Join(tmpDir, "test.json")
	if err := os.WriteFile(collectionFile, data, 0644); err != nil {
		t.Fatalf("failed to write test collection: %v", err)
	}

	// Load collections
	registry, err := LoadCollections(tmpDir)
	if err != nil {
		t.Fatalf("LoadCollections() failed: %v", err)
	}

	if registry.Count() != 1 {
		t.Errorf("expected 1 collection, got %d", registry.Count())
	}

	col := registry.Get("test-collection")
	if col == nil {
		t.Fatal("collection not found")
	}

	if col.Title != "Test Collection" {
		t.Errorf("expected title 'Test Collection', got %s", col.Title)
	}
}

func TestLoadCollectionsInvalidDirectory(t *testing.T) {
	_, err := LoadCollections("/nonexistent/directory")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestLoadCollectionsEmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := LoadCollections(tmpDir)
	if err == nil {
		t.Error("expected error for empty directory")
	}
}

func TestValidateCollection(t *testing.T) {
	tests := []struct {
		name      string
		collection *CollectionConfig
		wantError bool
	}{
		{
			name: "valid collection",
			collection: &CollectionConfig{
				ID:          "test",
				Title:       "Test",
				Description: "Test collection",
				ASFDatasets: []string{"DATASET"},
				License:     "proprietary",
				Extent: Extent{
					Spatial: SpatialExtent{
						BBox: [][]float64{{-180, -90, 180, 90}},
					},
					Temporal: TemporalExtent{
						Interval: [][]interface{}{{"2020-01-01T00:00:00Z", nil}},
					},
				},
			},
			wantError: false,
		},
		{
			name: "missing ID",
			collection: &CollectionConfig{
				Title:       "Test",
				Description: "Test collection",
				ASFDatasets: []string{"DATASET"},
				License:     "proprietary",
				Extent: Extent{
					Spatial: SpatialExtent{
						BBox: [][]float64{{-180, -90, 180, 90}},
					},
					Temporal: TemporalExtent{
						Interval: [][]interface{}{{"2020-01-01T00:00:00Z", nil}},
					},
				},
			},
			wantError: true,
		},
		{
			name: "missing ASF datasets",
			collection: &CollectionConfig{
				ID:          "test",
				Title:       "Test",
				Description: "Test collection",
				ASFDatasets: []string{},
				License:     "proprietary",
				Extent: Extent{
					Spatial: SpatialExtent{
						BBox: [][]float64{{-180, -90, 180, 90}},
					},
					Temporal: TemporalExtent{
						Interval: [][]interface{}{{"2020-01-01T00:00:00Z", nil}},
					},
				},
			},
			wantError: true,
		},
		{
			name: "missing license",
			collection: &CollectionConfig{
				ID:          "test",
				Title:       "Test",
				Description: "Test collection",
				ASFDatasets: []string{"DATASET"},
				Extent: Extent{
					Spatial: SpatialExtent{
						BBox: [][]float64{{-180, -90, 180, 90}},
					},
					Temporal: TemporalExtent{
						Interval: [][]interface{}{{"2020-01-01T00:00:00Z", nil}},
					},
				},
			},
			wantError: true,
		},
		{
			name: "invalid bbox",
			collection: &CollectionConfig{
				ID:          "test",
				Title:       "Test",
				Description: "Test collection",
				ASFDatasets: []string{"DATASET"},
				License:     "proprietary",
				Extent: Extent{
					Spatial: SpatialExtent{
						BBox: [][]float64{{-180, -90}}, // Invalid: only 2 values
					},
					Temporal: TemporalExtent{
						Interval: [][]interface{}{{"2020-01-01T00:00:00Z", nil}},
					},
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCollection(tt.collection)
			if (err != nil) != tt.wantError {
				t.Errorf("validateCollection() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestCollectionRegistryAdd(t *testing.T) {
	registry := NewCollectionRegistry()

	collection := &CollectionConfig{
		ID:    "test",
		Title: "Test",
	}

	err := registry.Add(collection)
	if err != nil {
		t.Errorf("Add() failed: %v", err)
	}

	// Try to add duplicate
	err = registry.Add(collection)
	if err == nil {
		t.Error("expected error when adding duplicate collection")
	}

	// Try to add nil
	err = registry.Add(nil)
	if err == nil {
		t.Error("expected error when adding nil collection")
	}
}

func TestCollectionRegistryGet(t *testing.T) {
	registry := NewCollectionRegistry()

	collection := &CollectionConfig{
		ID:    "test",
		Title: "Test",
	}

	registry.Add(collection)

	// Get existing collection
	result := registry.Get("test")
	if result == nil {
		t.Error("expected collection, got nil")
	}
	if result.Title != "Test" {
		t.Errorf("expected title 'Test', got %s", result.Title)
	}

	// Get non-existent collection
	result = registry.Get("nonexistent")
	if result != nil {
		t.Error("expected nil for non-existent collection")
	}
}

func TestCollectionRegistryHas(t *testing.T) {
	registry := NewCollectionRegistry()

	collection := &CollectionConfig{
		ID: "test",
	}

	registry.Add(collection)

	if !registry.Has("test") {
		t.Error("expected Has() to return true for existing collection")
	}

	if registry.Has("nonexistent") {
		t.Error("expected Has() to return false for non-existent collection")
	}
}

func TestCollectionRegistryAll(t *testing.T) {
	registry := NewCollectionRegistry()

	c1 := &CollectionConfig{ID: "collection1"}
	c2 := &CollectionConfig{ID: "collection2"}

	registry.Add(c1)
	registry.Add(c2)

	all := registry.All()
	if len(all) != 2 {
		t.Errorf("expected 2 collections, got %d", len(all))
	}
}

func TestCollectionRegistryIDs(t *testing.T) {
	registry := NewCollectionRegistry()

	registry.Add(&CollectionConfig{ID: "collection1"})
	registry.Add(&CollectionConfig{ID: "collection2"})

	ids := registry.IDs()
	if len(ids) != 2 {
		t.Errorf("expected 2 IDs, got %d", len(ids))
	}

	// Check that both IDs are present
	idMap := make(map[string]bool)
	for _, id := range ids {
		idMap[id] = true
	}

	if !idMap["collection1"] || !idMap["collection2"] {
		t.Error("expected both collection IDs to be present")
	}
}

func TestCollectionRegistryGetASFDatasets(t *testing.T) {
	registry := NewCollectionRegistry()

	collection := &CollectionConfig{
		ID:          "test",
		ASFDatasets: []string{"DATASET1", "DATASET2"},
	}

	registry.Add(collection)

	datasets := registry.GetASFDatasets("test")
	if len(datasets) != 2 {
		t.Errorf("expected 2 datasets, got %d", len(datasets))
	}

	// Test non-existent collection
	datasets = registry.GetASFDatasets("nonexistent")
	if datasets != nil {
		t.Error("expected nil for non-existent collection")
	}
}

func TestCollectionRegistryFindByASFDataset(t *testing.T) {
	registry := NewCollectionRegistry()

	c1 := &CollectionConfig{
		ID:          "collection1",
		ASFDatasets: []string{"DATASET1", "DATASET2"},
	}
	c2 := &CollectionConfig{
		ID:          "collection2",
		ASFDatasets: []string{"DATASET2", "DATASET3"},
	}

	registry.Add(c1)
	registry.Add(c2)

	// Find collections with DATASET2
	matches := registry.FindByASFDataset("DATASET2")
	if len(matches) != 2 {
		t.Errorf("expected 2 matches for DATASET2, got %d", len(matches))
	}

	// Find collections with DATASET1
	matches = registry.FindByASFDataset("DATASET1")
	if len(matches) != 1 {
		t.Errorf("expected 1 match for DATASET1, got %d", len(matches))
	}

	// Find collections with non-existent dataset
	matches = registry.FindByASFDataset("NONEXISTENT")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for NONEXISTENT, got %d", len(matches))
	}
}
