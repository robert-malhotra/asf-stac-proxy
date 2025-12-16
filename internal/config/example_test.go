package config_test

import (
	"fmt"
	"log"
	"os"

	"github.com/robert-malhotra/asf-stac-proxy/internal/config"
)

func ExampleLoad() {
	// Set required environment variable
	os.Setenv("STAC_BASE_URL", "https://stac.example.com")
	defer os.Unsetenv("STAC_BASE_URL")

	// Load configuration from environment
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	// Access configuration values
	fmt.Printf("Server: %s\n", cfg.Server.Address())
	fmt.Printf("ASF API: %s\n", cfg.ASF.BaseURL)
	fmt.Printf("STAC Version: %s\n", cfg.STAC.Version)
	fmt.Printf("Default Limit: %d\n", cfg.Features.DefaultLimit)

	// Output:
	// Server: 0.0.0.0:8080
	// ASF API: https://api.daac.asf.alaska.edu
	// STAC Version: 1.0.0
	// Default Limit: 10
}

func ExampleLoadCollections() {
	// Assuming collections are in ./collections directory
	registry, err := config.LoadCollections("../../collections")
	if err != nil {
		log.Printf("Warning: %v", err)
		return
	}

	// Get a specific collection
	if collection := registry.Get("sentinel-1-slc"); collection != nil {
		fmt.Printf("Collection ID: %s\n", collection.ID)
		fmt.Printf("Title: %s\n", collection.Title)
		fmt.Printf("ASF Datasets: %v\n", collection.ASFDatasets)
	}

	// List all collection IDs
	fmt.Printf("Total collections: %d\n", registry.Count())

	// Output:
	// Collection ID: sentinel-1-slc
	// Title: Sentinel-1 SAR - Single Look Complex (SLC)
	// ASF Datasets: [SENTINEL-1]
	// Total collections: 14
}

func ExampleCollectionRegistry_FindByASFDataset() {
	registry := config.NewCollectionRegistry()

	// Add some test collections
	registry.Add(&config.CollectionConfig{
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

	registry.Add(&config.CollectionConfig{
		ID:          "alos-palsar",
		Title:       "ALOS PALSAR",
		Description: "ALOS PALSAR data",
		ASFDatasets: []string{"ALOS"},
		License:     "proprietary",
		Extent: config.Extent{
			Spatial:  config.SpatialExtent{BBox: [][]float64{{-180, -90, 180, 90}}},
			Temporal: config.TemporalExtent{Interval: [][]interface{}{{"2006-01-01T00:00:00Z", nil}}},
		},
	})

	// Find collections by ASF dataset
	collections := registry.FindByASFDataset("SENTINEL-1")
	for _, col := range collections {
		fmt.Printf("Found: %s\n", col.ID)
	}

	// Output:
	// Found: sentinel-1
}

func ExampleServerConfig_Address() {
	// Set custom port
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("STAC_BASE_URL", "https://stac.example.com")
	defer func() {
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("STAC_BASE_URL")
	}()

	cfg, _ := config.Load()

	// Get server address
	addr := cfg.Server.Address()
	fmt.Printf("Listen on: %s\n", addr)

	// Output:
	// Listen on: 0.0.0.0:9090
}
