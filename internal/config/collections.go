package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CollectionConfig represents a STAC collection configuration that maps
// to one or more ASF datasets. This is typically loaded from JSON files
// in the collections directory.
type CollectionConfig struct {
	ID                   string                 `json:"id"`
	Title                string                 `json:"title"`
	Description          string                 `json:"description"`
	ASFDatasets          []string               `json:"asf_datasets"`
	ASFPlatforms         []string               `json:"asf_platforms,omitempty"`
	ASFProcessingLevels  []string               `json:"asf_processing_levels,omitempty"`
	CMR                  *CMRMapping            `json:"cmr,omitempty"`
	License              string                 `json:"license"`
	Providers            []Provider             `json:"providers,omitempty"`
	Extent               Extent                 `json:"extent"`
	Summaries            map[string]interface{} `json:"summaries,omitempty"`
	Extensions           []string               `json:"stac_extensions,omitempty"`
}

// CMRMapping contains CMR-specific configuration for a collection.
type CMRMapping struct {
	// ShortNames are CMR collection short names that map to this STAC collection
	ShortNames []string `json:"short_names,omitempty"`
	// ConceptIDs are CMR collection concept IDs (more precise than short names)
	ConceptIDs []string `json:"concept_ids,omitempty"`
	// Provider is the CMR provider (defaults to ASF)
	Provider string `json:"provider,omitempty"`
}

// Provider represents a data provider in a STAC collection.
type Provider struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Roles       []string `json:"roles,omitempty"`
	URL         string   `json:"url,omitempty"`
}

// Extent defines the spatial and temporal extent of a collection.
type Extent struct {
	Spatial  SpatialExtent  `json:"spatial"`
	Temporal TemporalExtent `json:"temporal"`
}

// SpatialExtent defines the bounding boxes for a collection.
type SpatialExtent struct {
	BBox [][]float64 `json:"bbox"`
}

// TemporalExtent defines the time intervals for a collection.
type TemporalExtent struct {
	Interval [][]interface{} `json:"interval"`
}

// CollectionRegistry holds all loaded collection configurations indexed by ID.
type CollectionRegistry struct {
	collections map[string]*CollectionConfig
}

// NewCollectionRegistry creates a new empty collection registry.
func NewCollectionRegistry() *CollectionRegistry {
	return &CollectionRegistry{
		collections: make(map[string]*CollectionConfig),
	}
}

// LoadCollections loads collection definitions from JSON files in the specified directory.
// It returns a CollectionRegistry containing all successfully loaded collections.
// Only files with a .json extension are processed.
func LoadCollections(collectionsDir string) (*CollectionRegistry, error) {
	registry := NewCollectionRegistry()

	// Check if directory exists
	info, err := os.Stat(collectionsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to access collections directory %q: %w", collectionsDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("collections path %q is not a directory", collectionsDir)
	}

	// Read directory entries
	entries, err := os.ReadDir(collectionsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read collections directory %q: %w", collectionsDir, err)
	}

	// Load each JSON file
	loadedCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		if !strings.HasSuffix(strings.ToLower(filename), ".json") {
			continue
		}

		filePath := filepath.Join(collectionsDir, filename)
		collection, err := loadCollectionFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load collection from %q: %w", filePath, err)
		}

		if err := registry.Add(collection); err != nil {
			return nil, fmt.Errorf("failed to add collection from %q: %w", filePath, err)
		}

		loadedCount++
	}

	if loadedCount == 0 {
		return nil, fmt.Errorf("no collection files found in %q", collectionsDir)
	}

	return registry, nil
}

// loadCollectionFile loads a single collection configuration from a JSON file.
func loadCollectionFile(filePath string) (*CollectionConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var collection CollectionConfig
	if err := json.Unmarshal(data, &collection); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	if err := validateCollection(&collection); err != nil {
		return nil, fmt.Errorf("invalid collection configuration: %w", err)
	}

	return &collection, nil
}

// validateCollection checks that a collection configuration is valid.
func validateCollection(c *CollectionConfig) error {
	if c.ID == "" {
		return fmt.Errorf("collection ID is required")
	}

	if c.Title == "" {
		return fmt.Errorf("collection title is required")
	}

	if c.Description == "" {
		return fmt.Errorf("collection description is required")
	}

	if len(c.ASFDatasets) == 0 {
		return fmt.Errorf("collection must specify at least one ASF dataset")
	}

	if c.License == "" {
		return fmt.Errorf("collection license is required")
	}

	// Validate spatial extent
	if len(c.Extent.Spatial.BBox) == 0 {
		return fmt.Errorf("collection must have at least one spatial bbox")
	}

	for i, bbox := range c.Extent.Spatial.BBox {
		if len(bbox) != 4 && len(bbox) != 6 {
			return fmt.Errorf("bbox[%d] must have 4 or 6 values, got %d", i, len(bbox))
		}
	}

	// Validate temporal extent
	if len(c.Extent.Temporal.Interval) == 0 {
		return fmt.Errorf("collection must have at least one temporal interval")
	}

	for i, interval := range c.Extent.Temporal.Interval {
		if len(interval) != 2 {
			return fmt.Errorf("temporal interval[%d] must have exactly 2 values, got %d", i, len(interval))
		}
	}

	return nil
}

// Add registers a collection in the registry.
// Returns an error if a collection with the same ID already exists.
func (r *CollectionRegistry) Add(collection *CollectionConfig) error {
	if collection == nil {
		return fmt.Errorf("cannot add nil collection")
	}

	if _, exists := r.collections[collection.ID]; exists {
		return fmt.Errorf("collection with ID %q already exists", collection.ID)
	}

	r.collections[collection.ID] = collection
	return nil
}

// Get retrieves a collection by ID.
// Returns nil if the collection does not exist.
func (r *CollectionRegistry) Get(id string) *CollectionConfig {
	return r.collections[id]
}

// Has checks if a collection with the given ID exists in the registry.
func (r *CollectionRegistry) Has(id string) bool {
	_, exists := r.collections[id]
	return exists
}

// All returns all collections in the registry.
func (r *CollectionRegistry) All() []*CollectionConfig {
	collections := make([]*CollectionConfig, 0, len(r.collections))
	for _, collection := range r.collections {
		collections = append(collections, collection)
	}
	return collections
}

// IDs returns all collection IDs in the registry.
func (r *CollectionRegistry) IDs() []string {
	ids := make([]string, 0, len(r.collections))
	for id := range r.collections {
		ids = append(ids, id)
	}
	return ids
}

// Count returns the number of collections in the registry.
func (r *CollectionRegistry) Count() int {
	return len(r.collections)
}

// GetASFDatasets returns all ASF datasets for the given collection ID.
// Returns nil if the collection does not exist.
func (r *CollectionRegistry) GetASFDatasets(collectionID string) []string {
	collection := r.Get(collectionID)
	if collection == nil {
		return nil
	}
	return collection.ASFDatasets
}

// GetASFProcessingLevels returns the ASF processing levels for the given collection ID.
// Returns nil if the collection does not exist or has no processing levels configured.
func (r *CollectionRegistry) GetASFProcessingLevels(collectionID string) []string {
	collection := r.Get(collectionID)
	if collection == nil {
		return nil
	}
	return collection.ASFProcessingLevels
}

// FindByASFDataset returns all collections that include the specified ASF dataset.
func (r *CollectionRegistry) FindByASFDataset(dataset string) []*CollectionConfig {
	var matches []*CollectionConfig
	for _, collection := range r.collections {
		for _, ds := range collection.ASFDatasets {
			if ds == dataset {
				matches = append(matches, collection)
				break
			}
		}
	}
	return matches
}
