# Config Package

The `config` package provides configuration management for the ASF-STAC proxy service. It handles loading configuration from environment variables and collection definitions from JSON files.

## Features

- **Environment-based configuration**: Uses `github.com/caarlos0/env/v10` for parsing environment variables
- **Type-safe configuration**: Strongly-typed configuration structs with validation
- **Collection registry**: Load and manage STAC collection definitions from JSON files
- **Sensible defaults**: Provides production-ready default values for all optional settings

## Usage

### Loading Configuration

```go
import "github.com/rkm/asf-stac-proxy/internal/config"

// Load configuration from environment variables
cfg, err := config.Load()
if err != nil {
    log.Fatal(err)
}

// Access configuration values
fmt.Printf("Server listening on %s\n", cfg.Server.Address())
fmt.Printf("ASF API: %s\n", cfg.ASF.BaseURL)
fmt.Printf("STAC version: %s\n", cfg.STAC.Version)
```

### Loading Collections

```go
// Load collection definitions from JSON files
registry, err := config.LoadCollections("./collections")
if err != nil {
    log.Fatal(err)
}

// Get a specific collection
collection := registry.Get("sentinel-1")
if collection != nil {
    fmt.Printf("Collection: %s\n", collection.Title)
    fmt.Printf("ASF Datasets: %v\n", collection.ASFDatasets)
}

// List all collections
for _, col := range registry.All() {
    fmt.Printf("- %s: %s\n", col.ID, col.Title)
}

// Find collections by ASF dataset
collections := registry.FindByASFDataset("SENTINEL-1")
```

## Environment Variables

All environment variables support prefixes to organize configuration:

### Server Configuration (`SERVER_*`)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `SERVER_HOST` | string | `0.0.0.0` | Server listen address |
| `SERVER_PORT` | int | `8080` | Server listen port |
| `SERVER_READ_TIMEOUT` | duration | `30s` | HTTP read timeout |
| `SERVER_WRITE_TIMEOUT` | duration | `60s` | HTTP write timeout |
| `SERVER_SHUTDOWN_TIMEOUT` | duration | `10s` | Graceful shutdown timeout |

### ASF API Configuration (`ASF_*`)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `ASF_BASE_URL` | string | `https://api.daac.asf.alaska.edu` | ASF API base URL |
| `ASF_TIMEOUT` | duration | `30s` | ASF API request timeout |

### STAC Configuration (`STAC_*`)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `STAC_VERSION` | string | `1.0.0` | STAC specification version |
| `STAC_BASE_URL` | string | **(required)** | Public-facing URL of this service |
| `STAC_TITLE` | string | `ASF STAC API` | API title |
| `STAC_DESCRIPTION` | string | `STAC API proxy for Alaska Satellite Facility` | API description |

### Feature Flags (`FEATURE_*`)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `FEATURE_ENABLE_SEARCH` | bool | `true` | Enable `/search` endpoint |
| `FEATURE_ENABLE_QUERYABLES` | bool | `true` | Enable `/queryables` endpoint |
| `FEATURE_DEFAULT_LIMIT` | int | `10` | Default page size for results |
| `FEATURE_MAX_LIMIT` | int | `250` | Maximum page size for results |

### Logging Configuration (`LOG_*`)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LOG_LEVEL` | string | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | string | `json` | Log format: `json`, `text` |

## Collection JSON Format

Collection definitions are stored as JSON files in a directory (e.g., `./collections/`). Each file represents one STAC collection.

### Example: sentinel-1.json

```json
{
  "id": "sentinel-1",
  "title": "Sentinel-1 SAR",
  "description": "Sentinel-1 synthetic aperture radar data from the European Space Agency",
  "asf_datasets": ["SENTINEL-1"],
  "asf_platforms": ["Sentinel-1A", "Sentinel-1B"],
  "license": "proprietary",
  "providers": [
    {
      "name": "ESA",
      "description": "European Space Agency",
      "roles": ["producer", "licensor"],
      "url": "https://www.esa.int"
    },
    {
      "name": "Alaska Satellite Facility",
      "description": "ASF DAAC",
      "roles": ["processor", "host"],
      "url": "https://asf.alaska.edu"
    }
  ],
  "extent": {
    "spatial": {
      "bbox": [[-180, -90, 180, 90]]
    },
    "temporal": {
      "interval": [["2014-04-03T00:00:00Z", null]]
    }
  },
  "summaries": {
    "platform": ["sentinel-1a", "sentinel-1b"],
    "instruments": ["c-sar"],
    "constellation": ["sentinel-1"],
    "sar:instrument_mode": ["IW", "EW", "SM", "WV"],
    "sar:frequency_band": ["C"],
    "sar:polarizations": [["VV"], ["VH"], ["VV", "VH"]],
    "sat:orbit_state": ["ascending", "descending"]
  },
  "stac_extensions": [
    "https://stac-extensions.github.io/sar/v1.0.0/schema.json",
    "https://stac-extensions.github.io/sat/v1.0.0/schema.json",
    "https://stac-extensions.github.io/processing/v1.0.0/schema.json"
  ]
}
```

### Required Fields

- `id`: Unique collection identifier
- `title`: Human-readable collection title
- `description`: Collection description
- `asf_datasets`: Array of ASF dataset names that map to this collection
- `license`: License identifier (e.g., "proprietary", "CC-BY-4.0")
- `extent.spatial.bbox`: Array of bounding boxes (each with 4 or 6 values)
- `extent.temporal.interval`: Array of time intervals (each with start and end, end can be null)

### Optional Fields

- `asf_platforms`: Array of ASF platform names to filter by
- `providers`: Array of provider information
- `summaries`: Map of property summaries (used in STAC collection metadata)
- `stac_extensions`: Array of STAC extension URLs

## Types

### CollectionConfig

Main collection configuration struct:

```go
type CollectionConfig struct {
    ID           string
    Title        string
    Description  string
    ASFDatasets  []string
    ASFPlatforms []string
    License      string
    Providers    []Provider
    Extent       Extent
    Summaries    map[string]interface{}
    Extensions   []string
}
```

### CollectionRegistry

Registry for managing collections:

```go
type CollectionRegistry struct {
    // ...private fields...
}

// Methods:
func (r *CollectionRegistry) Get(id string) *CollectionConfig
func (r *CollectionRegistry) Has(id string) bool
func (r *CollectionRegistry) All() []*CollectionConfig
func (r *CollectionRegistry) IDs() []string
func (r *CollectionRegistry) Count() int
func (r *CollectionRegistry) GetASFDatasets(collectionID string) []string
func (r *CollectionRegistry) FindByASFDataset(dataset string) []*CollectionConfig
```

## Validation

The package performs comprehensive validation:

### Configuration Validation

- Server port must be between 1 and 65535
- All timeout values must be positive
- ASF base URL is required
- STAC base URL is required
- STAC version is required
- Default limit must be at least 1
- Max limit must be >= default limit
- Log level must be one of: debug, info, warn, error
- Log format must be one of: json, text

### Collection Validation

- Collection ID is required
- Title and description are required
- At least one ASF dataset must be specified
- License is required
- At least one spatial bbox is required
- Each bbox must have 4 or 6 values
- At least one temporal interval is required
- Each interval must have exactly 2 values (start and end)

## Testing

Run the package tests:

```bash
go test ./internal/config/
```

Run with coverage:

```bash
go test -cover ./internal/config/
```

Run with verbose output:

```bash
go test -v ./internal/config/
```
