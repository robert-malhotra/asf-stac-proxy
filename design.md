# Design Document: ASF-to-STAC Proxy Service

**Version:** 1.0  
**Date:** December 2025  
**Status:** Draft

---

## 1. Overview

### 1.1 Purpose

This document describes the design of a Go service that acts as a proxy between clients expecting a STAC (SpatioTemporal Asset Catalog) API and the Alaska Satellite Facility (ASF) Search API. The service translates STAC API requests into ASF API queries and transforms ASF responses into STAC-compliant responses.

### 1.2 Goals

- Expose ASF's SAR data catalog through a standards-compliant STAC API
- Enable existing STAC clients and tooling to discover and access ASF data
- Provide a seamless translation layer with minimal latency overhead
- Support the core STAC API specifications: Core, Features, and Item Search

### 1.3 Non-Goals

- Caching or persistence of ASF data (stateless proxy)
- Authentication/authorization (pass-through to ASF)
- Modification or upload of data to ASF
- Full OGC API - Features compliance beyond STAC requirements

---

## 2. Architecture

### 2.1 High-Level Architecture

```
┌─────────────────┐     ┌─────────────────────────────┐     ┌─────────────────┐
│   STAC Client   │────▶│    ASF-STAC Proxy (Go)      │────▶│   ASF Search    │
│  (pystac, etc)  │◀────│                             │◀────│      API        │
└─────────────────┘     │  ┌─────────────────────┐    │     └─────────────────┘
                        │  │   HTTP Server       │    │
                        │  │   (net/http)        │    │
                        │  └──────────┬──────────┘    │
                        │             │               │
                        │  ┌──────────▼──────────┐    │
                        │  │   Router            │    │
                        │  │   (chi/gorilla)     │    │
                        │  └──────────┬──────────┘    │
                        │             │               │
                        │  ┌──────────▼──────────┐    │
                        │  │   Handlers          │    │
                        │  │   - Landing Page    │    │
                        │  │   - Conformance     │    │
                        │  │   - Collections     │    │
                        │  │   - Items           │    │
                        │  │   - Search          │    │
                        │  └──────────┬──────────┘    │
                        │             │               │
                        │  ┌──────────▼──────────┐    │
                        │  │   Translation Layer │    │
                        │  │   - Request Mapper  │    │
                        │  │   - Response Mapper │    │
                        │  └──────────┬──────────┘    │
                        │             │               │
                        │  ┌──────────▼──────────┐    │
                        │  │   ASF Client        │    │
                        │  │   (HTTP Client)     │    │
                        │  └─────────────────────┘    │
                        └─────────────────────────────┘
```

### 2.2 Component Overview

| Component | Responsibility |
|-----------|----------------|
| HTTP Server | Accept incoming STAC API requests |
| Router | Route requests to appropriate handlers |
| Handlers | Implement STAC API endpoint logic |
| Translation Layer | Convert between STAC and ASF formats |
| ASF Client | Make requests to ASF Search API |

---

## 3. API Mapping

### 3.1 Endpoint Mapping

| STAC Endpoint | HTTP Method | ASF Equivalent | Description |
|---------------|-------------|----------------|-------------|
| `/` | GET | N/A (generated) | Landing page / root catalog |
| `/conformance` | GET | N/A (static) | Conformance classes declaration |
| `/collections` | GET | N/A (static/configured) | List all collections |
| `/collections/{collectionId}` | GET | N/A (static/configured) | Single collection metadata |
| `/collections/{collectionId}/items` | GET | `/services/search/param` | Items in a collection |
| `/collections/{collectionId}/items/{itemId}` | GET | `/services/search/param?granule_list=` | Single item by ID |
| `/search` | GET/POST | `/services/search/param` | Cross-collection search |
| `/queryables` | GET | N/A (static/configured) | Available query parameters |

### 3.2 Collection Mapping

ASF datasets map to STAC collections. The following mapping is proposed:

| ASF Dataset | STAC Collection ID | Description |
|-------------|-------------------|-------------|
| SENTINEL-1 | `sentinel-1` | Sentinel-1 SAR data |
| SLC-BURST | `sentinel-1-bursts` | Sentinel-1 burst products |
| OPERA-S1 | `opera-s1` | OPERA Sentinel-1 products |
| ALOS PALSAR | `alos-palsar` | ALOS PALSAR data |
| ALOS AVNIR-2 | `alos-avnir-2` | ALOS AVNIR-2 optical data |
| RADARSAT-1 | `radarsat-1` | RADARSAT-1 SAR data |
| ERS | `ers` | ERS-1 and ERS-2 SAR data |
| JERS-1 | `jers-1` | JERS-1 SAR data |
| SEASAT | `seasat` | SEASAT SAR data |
| UAVSAR | `uavsar` | UAVSAR airborne SAR |
| AIRSAR | `airsar` | AIRSAR data |
| SIR-C | `sir-c` | SIR-C data |
| SMAP | `smap` | SMAP soil moisture data |
| ARIA S1 GUNW | `aria-s1-gunw` | ARIA Sentinel-1 GUNW |
| NISAR | `nisar` | NISAR data (when available) |

### 3.3 Query Parameter Mapping

#### STAC to ASF Parameter Translation

| STAC Parameter | ASF Parameter | Notes |
|----------------|---------------|-------|
| `bbox` | `intersectsWith` | Convert bbox to WKT POLYGON |
| `datetime` | `start`, `end` | Parse ISO 8601 interval |
| `intersects` | `intersectsWith` | GeoJSON to WKT conversion |
| `ids` | `granule_list` | Direct mapping |
| `collections` | `dataset` | Map collection IDs to datasets |
| `limit` | `maxResults` | Direct mapping |
| `pt` (page token) | Custom pagination | See pagination section |

#### Additional Queryables (Extension)

| STAC Queryable | ASF Parameter | Type |
|----------------|---------------|------|
| `platform` | `platform` | string |
| `sar:instrument_mode` | `beamMode` | string |
| `sar:polarizations` | `polarization` | string[] |
| `sar:frequency_band` | N/A (derived) | string |
| `sat:orbit_state` | `flightDirection` | string |
| `sat:relative_orbit` | `relativeOrbit` | integer |
| `sat:absolute_orbit` | `absoluteOrbit` | integer |
| `view:off_nadir` | `offNadirAngle` | number |
| `processing:level` | `processingLevel` | string |

---

## 4. Data Models

### 4.1 STAC Item Structure

```go
// Item represents a STAC Item (GeoJSON Feature)
type Item struct {
    Type          string                 `json:"type"` // "Feature"
    StacVersion   string                 `json:"stac_version"`
    StacExtensions []string              `json:"stac_extensions,omitempty"`
    ID            string                 `json:"id"`
    Geometry      *geojson.Geometry      `json:"geometry"`
    BBox          []float64              `json:"bbox"`
    Properties    ItemProperties         `json:"properties"`
    Links         []Link                 `json:"links"`
    Assets        map[string]Asset       `json:"assets"`
    Collection    string                 `json:"collection,omitempty"`
}

// ItemProperties contains temporal and additional properties
type ItemProperties struct {
    DateTime    *time.Time             `json:"datetime"`
    StartTime   *time.Time             `json:"start_datetime,omitempty"`
    EndTime     *time.Time             `json:"end_datetime,omitempty"`
    Created     *time.Time             `json:"created,omitempty"`
    Updated     *time.Time             `json:"updated,omitempty"`
    Platform    string                 `json:"platform,omitempty"`
    Instrument  []string               `json:"instruments,omitempty"`
    Constellation string               `json:"constellation,omitempty"`
    GSD         *float64               `json:"gsd,omitempty"`
    
    // SAR Extension properties
    SARInstrumentMode   string    `json:"sar:instrument_mode,omitempty"`
    SARFrequencyBand    string    `json:"sar:frequency_band,omitempty"`
    SARCenterFrequency  *float64  `json:"sar:center_frequency,omitempty"`
    SARPolarizations    []string  `json:"sar:polarizations,omitempty"`
    SARProductType      string    `json:"sar:product_type,omitempty"`
    SARLooksRange       *int      `json:"sar:looks_range,omitempty"`
    SARLooksAzimuth     *int      `json:"sar:looks_azimuth,omitempty"`
    SARLooksEquivalent  *float64  `json:"sar:looks_equivalent_number,omitempty"`
    SARObservationDir   string    `json:"sar:observation_direction,omitempty"`
    
    // Satellite Extension properties
    SatOrbitState       string    `json:"sat:orbit_state,omitempty"`
    SatRelativeOrbit    *int      `json:"sat:relative_orbit,omitempty"`
    SatAbsoluteOrbit    *int      `json:"sat:absolute_orbit,omitempty"`
    SatAnxDatetime      *time.Time `json:"sat:anx_datetime,omitempty"`
    
    // View Extension properties
    ViewOffNadir        *float64  `json:"view:off_nadir,omitempty"`
    
    // Processing Extension
    ProcessingLevel     string    `json:"processing:level,omitempty"`
    ProcessingSoftware  map[string]string `json:"processing:software,omitempty"`
}

// Asset represents a STAC Asset
type Asset struct {
    Href        string   `json:"href"`
    Title       string   `json:"title,omitempty"`
    Description string   `json:"description,omitempty"`
    Type        string   `json:"type,omitempty"` // MIME type
    Roles       []string `json:"roles,omitempty"`
}

// Link represents a STAC Link
type Link struct {
    Href  string `json:"href"`
    Rel   string `json:"rel"`
    Type  string `json:"type,omitempty"`
    Title string `json:"title,omitempty"`
}
```

### 4.2 STAC Collection Structure

```go
// Collection represents a STAC Collection
type Collection struct {
    Type           string              `json:"type"` // "Collection"
    StacVersion    string              `json:"stac_version"`
    StacExtensions []string            `json:"stac_extensions,omitempty"`
    ID             string              `json:"id"`
    Title          string              `json:"title,omitempty"`
    Description    string              `json:"description"`
    Keywords       []string            `json:"keywords,omitempty"`
    License        string              `json:"license"`
    Providers      []Provider          `json:"providers,omitempty"`
    Extent         Extent              `json:"extent"`
    Summaries      map[string]any      `json:"summaries,omitempty"`
    Links          []Link              `json:"links"`
    Assets         map[string]Asset    `json:"assets,omitempty"`
}

// Extent defines spatial and temporal bounds
type Extent struct {
    Spatial  SpatialExtent  `json:"spatial"`
    Temporal TemporalExtent `json:"temporal"`
}

// SpatialExtent defines bounding boxes
type SpatialExtent struct {
    BBox [][]float64 `json:"bbox"` // [[west, south, east, north]]
}

// TemporalExtent defines time intervals
type TemporalExtent struct {
    Interval [][]string `json:"interval"` // [["start", "end"]]
}

// Provider represents data provider information
type Provider struct {
    Name        string   `json:"name"`
    Description string   `json:"description,omitempty"`
    Roles       []string `json:"roles,omitempty"`
    URL         string   `json:"url,omitempty"`
}
```

### 4.3 ASF Response Structures

```go
// ASFGeoJSONResponse represents ASF's GeoJSON output
type ASFGeoJSONResponse struct {
    Type     string       `json:"type"` // "FeatureCollection"
    Features []ASFFeature `json:"features"`
}

// ASFFeature represents a single ASF search result
type ASFFeature struct {
    Type       string            `json:"type"` // "Feature"
    Geometry   *geojson.Geometry `json:"geometry"`
    Properties ASFProperties     `json:"properties"`
}

// ASFProperties contains ASF-specific metadata
type ASFProperties struct {
    SceneName           string    `json:"sceneName"`
    FileID              string    `json:"fileID"`
    Platform            string    `json:"platform"`
    Instrument          string    `json:"instrument"`
    BeamMode            string    `json:"beamMode"`
    BeamModeType        string    `json:"beamModeType"`
    Polarization        string    `json:"polarization"`
    FlightDirection     string    `json:"flightDirection"`
    LookDirection       string    `json:"lookDirection"`
    ProcessingLevel     string    `json:"processingLevel"`
    ProcessingType      string    `json:"processingType"`
    FrameNumber         *int      `json:"frameNumber"`
    AbsoluteOrbit       *int      `json:"absoluteOrbit"`
    RelativeOrbit       *int      `json:"relativeOrbit"`
    StartTime           string    `json:"startTime"`
    StopTime            string    `json:"stopTime"`
    CenterLat           *float64  `json:"centerLat"`
    CenterLon           *float64  `json:"centerLon"`
    NearStartLat        *float64  `json:"nearStartLat"`
    NearStartLon        *float64  `json:"nearStartLon"`
    FarStartLat         *float64  `json:"farStartLat"`
    FarStartLon         *float64  `json:"farStartLon"`
    NearEndLat          *float64  `json:"nearEndLat"`
    NearEndLon          *float64  `json:"nearEndLon"`
    FarEndLat           *float64  `json:"farEndLat"`
    FarEndLon           *float64  `json:"farEndLon"`
    FaradayRotation     *float64  `json:"faradayRotation"`
    OffNadirAngle       *float64  `json:"offNadirAngle"`
    PathNumber          *int      `json:"pathNumber"`
    URL                 string    `json:"url"`
    FileName            string    `json:"fileName"`
    FileSize            *int64    `json:"fileSize"`
    Bytes               *int64    `json:"bytes"`
    MD5Sum              string    `json:"md5sum"`
    Browse              string    `json:"browse"`
    Thumbnail           string    `json:"thumbnail"`
    GroupID             string    `json:"groupID"`
    InsarStackID        *string   `json:"insarStackId"`
    ProcessingDate      string    `json:"processingDate"`
    InsarBaseline       *float64  `json:"insarBaseline"`
    TemporalBaseline    *int      `json:"temporalBaseline"`
    PerpendicularBaseline *float64 `json:"perpendicularBaseline"`
}
```

---

## 5. Core Components

### 5.1 Configuration

```go
// Config holds application configuration
type Config struct {
    // Server settings
    Host            string        `env:"HOST" envDefault:"0.0.0.0"`
    Port            int           `env:"PORT" envDefault:"8080"`
    ReadTimeout     time.Duration `env:"READ_TIMEOUT" envDefault:"30s"`
    WriteTimeout    time.Duration `env:"WRITE_TIMEOUT" envDefault:"60s"`
    ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`
    
    // ASF API settings
    ASFBaseURL      string `env:"ASF_BASE_URL" envDefault:"https://api.daac.asf.alaska.edu"`
    ASFTimeout      time.Duration `env:"ASF_TIMEOUT" envDefault:"30s"`
    
    // STAC settings
    STACVersion     string `env:"STAC_VERSION" envDefault:"1.0.0"`
    BaseURL         string `env:"BASE_URL"` // Public-facing URL
    Title           string `env:"TITLE" envDefault:"ASF STAC API"`
    Description     string `env:"DESCRIPTION" envDefault:"STAC API proxy for Alaska Satellite Facility"`
    
    // Feature flags
    EnableSearch    bool `env:"ENABLE_SEARCH" envDefault:"true"`
    EnableQueryables bool `env:"ENABLE_QUERYABLES" envDefault:"true"`
    DefaultLimit    int  `env:"DEFAULT_LIMIT" envDefault:"10"`
    MaxLimit        int  `env:"MAX_LIMIT" envDefault:"250"`
    
    // Logging
    LogLevel        string `env:"LOG_LEVEL" envDefault:"info"`
    LogFormat       string `env:"LOG_FORMAT" envDefault:"json"`
}
```

### 5.2 ASF Client

```go
// ASFClient handles communication with the ASF API
type ASFClient struct {
    baseURL    string
    httpClient *http.Client
    logger     *slog.Logger
}

// NewASFClient creates a new ASF API client
func NewASFClient(cfg *Config, logger *slog.Logger) *ASFClient {
    return &ASFClient{
        baseURL: cfg.ASFBaseURL,
        httpClient: &http.Client{
            Timeout: cfg.ASFTimeout,
            Transport: &http.Transport{
                MaxIdleConns:        100,
                MaxIdleConnsPerHost: 100,
                IdleConnTimeout:     90 * time.Second,
            },
        },
        logger: logger,
    }
}

// Search performs a search against the ASF API
func (c *ASFClient) Search(ctx context.Context, params ASFSearchParams) (*ASFGeoJSONResponse, error)

// GetGranule retrieves a single granule by name
func (c *ASFClient) GetGranule(ctx context.Context, granuleName string) (*ASFFeature, error)

// GetBaseline retrieves baseline information for a reference scene
func (c *ASFClient) GetBaseline(ctx context.Context, reference string) (*ASFGeoJSONResponse, error)

// ASFSearchParams contains parameters for ASF search
type ASFSearchParams struct {
    Dataset          []string
    Platform         []string
    IntersectsWith   string // WKT
    Start            *time.Time
    End              *time.Time
    GranuleList      []string
    BeamMode         []string
    Polarization     []string
    FlightDirection  string
    RelativeOrbit    []int
    AbsoluteOrbit    []int
    ProcessingLevel  []string
    OffNadirAngle    []float64
    MaxResults       int
    Output           string // always "geojson"
}
```

### 5.3 Translation Layer

```go
// Translator handles conversion between STAC and ASF formats
type Translator struct {
    cfg            *Config
    collectionMap  map[string]CollectionConfig
}

// CollectionConfig maps a STAC collection to ASF dataset(s)
type CollectionConfig struct {
    ID             string
    Title          string
    Description    string
    ASFDatasets    []string
    ASFPlatforms   []string
    License        string
    Providers      []Provider
    Extent         Extent
    Summaries      map[string]any
    Extensions     []string
}

// TranslateSTACSearchToASF converts STAC search params to ASF params
func (t *Translator) TranslateSTACSearchToASF(req STACSearchRequest, collectionID string) (ASFSearchParams, error)

// TranslateASFFeatureToSTACItem converts an ASF feature to a STAC Item
func (t *Translator) TranslateASFFeatureToSTACItem(feature *ASFFeature, collectionID string) (*Item, error)

// TranslateASFResponseToItemCollection converts ASF response to STAC ItemCollection
func (t *Translator) TranslateASFResponseToItemCollection(
    resp *ASFGeoJSONResponse, 
    req STACSearchRequest,
    collectionID string,
) (*ItemCollection, error)
```

### 5.4 Geometry Utilities

```go
// GeometryConverter handles geometry transformations
type GeometryConverter struct{}

// BBoxToWKT converts a STAC bbox to WKT POLYGON
func (g *GeometryConverter) BBoxToWKT(bbox []float64) (string, error)

// GeoJSONToWKT converts GeoJSON geometry to WKT
func (g *GeometryConverter) GeoJSONToWKT(geom *geojson.Geometry) (string, error)

// WKTToGeoJSON converts WKT to GeoJSON geometry
func (g *GeometryConverter) WKTToGeoJSON(wkt string) (*geojson.Geometry, error)

// ComputeBBox computes bounding box from GeoJSON geometry
func (g *GeometryConverter) ComputeBBox(geom *geojson.Geometry) []float64
```

---

## 6. HTTP Handlers

### 6.1 Handler Structure

```go
// Handlers contains all HTTP handlers
type Handlers struct {
    cfg        *Config
    asfClient  *ASFClient
    translator *Translator
    logger     *slog.Logger
}

// NewHandlers creates a new Handlers instance
func NewHandlers(cfg *Config, asfClient *ASFClient, translator *Translator, logger *slog.Logger) *Handlers
```

### 6.2 Landing Page Handler

```go
// GET /
func (h *Handlers) LandingPage(w http.ResponseWriter, r *http.Request) {
    // Returns STAC Catalog with links to:
    // - self
    // - root
    // - service-desc (OpenAPI)
    // - service-doc
    // - conformance
    // - data (collections)
    // - search (if enabled)
}
```

**Response Example:**
```json
{
  "type": "Catalog",
  "id": "asf-stac-root",
  "stac_version": "1.0.0",
  "title": "ASF STAC API",
  "description": "STAC API proxy for Alaska Satellite Facility SAR data",
  "conformsTo": [
    "https://api.stacspec.org/v1.0.0/core",
    "https://api.stacspec.org/v1.0.0/ogcapi-features",
    "https://api.stacspec.org/v1.0.0/item-search"
  ],
  "links": [
    {"rel": "self", "href": "/", "type": "application/json"},
    {"rel": "root", "href": "/", "type": "application/json"},
    {"rel": "conformance", "href": "/conformance", "type": "application/json"},
    {"rel": "data", "href": "/collections", "type": "application/json"},
    {"rel": "search", "href": "/search", "type": "application/geo+json", "method": "GET"},
    {"rel": "search", "href": "/search", "type": "application/geo+json", "method": "POST"}
  ]
}
```

### 6.3 Conformance Handler

```go
// GET /conformance
func (h *Handlers) Conformance(w http.ResponseWriter, r *http.Request) {
    // Returns conformance classes supported
}
```

**Response Example:**
```json
{
  "conformsTo": [
    "https://api.stacspec.org/v1.0.0/core",
    "https://api.stacspec.org/v1.0.0/ogcapi-features",
    "https://api.stacspec.org/v1.0.0/item-search",
    "https://api.stacspec.org/v1.0.0/item-search#filter",
    "http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/core",
    "http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/geojson"
  ]
}
```

### 6.4 Collections Handler

```go
// GET /collections
func (h *Handlers) Collections(w http.ResponseWriter, r *http.Request) {
    // Returns list of all collections (static/configured)
}

// GET /collections/{collectionId}
func (h *Handlers) Collection(w http.ResponseWriter, r *http.Request) {
    // Returns single collection by ID
}
```

### 6.5 Items Handler

```go
// GET /collections/{collectionId}/items
func (h *Handlers) Items(w http.ResponseWriter, r *http.Request) {
    // Parse query parameters: bbox, datetime, limit, pt (page token)
    // Translate to ASF search
    // Return ItemCollection
}

// GET /collections/{collectionId}/items/{itemId}
func (h *Handlers) Item(w http.ResponseWriter, r *http.Request) {
    // Translate itemId to ASF granule name
    // Fetch from ASF
    // Return single Item
}
```

### 6.6 Search Handler

```go
// GET/POST /search
func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
    // Parse STAC search request (GET params or POST body)
    // Validate parameters
    // Translate to ASF search
    // Return ItemCollection with pagination links
}

// STACSearchRequest represents a STAC search request
type STACSearchRequest struct {
    BBox        []float64          `json:"bbox"`
    DateTime    string             `json:"datetime"`
    Intersects  *geojson.Geometry  `json:"intersects"`
    IDs         []string           `json:"ids"`
    Collections []string           `json:"collections"`
    Limit       int                `json:"limit"`
    PageToken   string             `json:"pt"` // For pagination
    
    // Filter extension
    Filter      any                `json:"filter"`
    FilterLang  string             `json:"filter-lang"`
    FilterCRS   string             `json:"filter-crs"`
}
```

### 6.7 Queryables Handler

```go
// GET /queryables
// GET /collections/{collectionId}/queryables
func (h *Handlers) Queryables(w http.ResponseWriter, r *http.Request) {
    // Return JSON Schema describing available query parameters
}
```

---

## 7. Pagination Strategy

ASF's API uses `maxResults` but doesn't provide cursor-based pagination. The proxy implements pagination by:

### 7.1 Approach: Offset-Based with Opaque Token

```go
// PageToken encodes pagination state
type PageToken struct {
    Offset      int       `json:"o"`
    Limit       int       `json:"l"`
    QueryHash   string    `json:"q"` // Hash of original query for validation
}

// EncodePageToken creates an opaque page token
func EncodePageToken(pt PageToken) string {
    data, _ := json.Marshal(pt)
    return base64.URLEncoding.EncodeToString(data)
}

// DecodePageToken parses an opaque page token
func DecodePageToken(token string) (*PageToken, error) {
    data, err := base64.URLEncoding.DecodeString(token)
    if err != nil {
        return nil, err
    }
    var pt PageToken
    if err := json.Unmarshal(data, &pt); err != nil {
        return nil, err
    }
    return &pt, nil
}
```

### 7.2 Pagination in Response

```go
// ItemCollection represents a STAC ItemCollection (GeoJSON FeatureCollection)
type ItemCollection struct {
    Type           string   `json:"type"` // "FeatureCollection"
    Features       []Item   `json:"features"`
    Links          []Link   `json:"links"`
    NumberMatched  *int     `json:"numberMatched,omitempty"`
    NumberReturned int      `json:"numberReturned"`
    Context        *Context `json:"context,omitempty"` // STAC Context extension
}

// Context provides additional metadata about the response
type Context struct {
    Returned int  `json:"returned"`
    Limit    int  `json:"limit,omitempty"`
    Matched  *int `json:"matched,omitempty"`
}
```

### 7.3 Limitations

Since ASF doesn't expose total count efficiently, `numberMatched` may be omitted or estimated. The `next` link is provided when results equal the limit, indicating more results may exist.

---

## 8. Error Handling

### 8.1 Error Response Format

```go
// STACError represents a STAC-compliant error response
type STACError struct {
    Code        string `json:"code"`
    Description string `json:"description"`
}

// Common error codes
const (
    ErrCodeBadRequest          = "BadRequest"
    ErrCodeNotFound            = "NotFound"
    ErrCodeInvalidParameter    = "InvalidParameterValue"
    ErrCodeServerError         = "ServerError"
    ErrCodeUpstreamError       = "UpstreamServiceError"
)
```

### 8.2 Error Mapping

| HTTP Status | Code | Description |
|-------------|------|-------------|
| 400 | BadRequest | Malformed request |
| 400 | InvalidParameterValue | Invalid query parameter |
| 404 | NotFound | Collection or item not found |
| 500 | ServerError | Internal server error |
| 502 | UpstreamServiceError | ASF API error |
| 504 | UpstreamServiceError | ASF API timeout |

---

## 9. Project Structure

```
asf-stac-proxy/
├── cmd/
│   └── server/
│       └── main.go              # Application entry point
├── internal/
│   ├── api/
│   │   ├── handlers.go          # HTTP handlers
│   │   ├── middleware.go        # HTTP middleware
│   │   ├── router.go            # Route definitions
│   │   └── responses.go         # Response helpers
│   ├── asf/
│   │   ├── client.go            # ASF API client
│   │   ├── models.go            # ASF data models
│   │   └── params.go            # ASF parameter building
│   ├── config/
│   │   ├── config.go            # Configuration loading
│   │   └── collections.go       # Collection definitions
│   ├── stac/
│   │   ├── models.go            # STAC data models
│   │   ├── extensions.go        # STAC extension handling
│   │   └── validation.go        # Request validation
│   └── translate/
│       ├── translator.go        # Main translation logic
│       ├── geometry.go          # Geometry conversions
│       ├── datetime.go          # Datetime parsing
│       └── properties.go        # Property mapping
├── pkg/
│   └── geojson/
│       └── geojson.go           # GeoJSON utilities
├── collections/
│   ├── sentinel-1.json          # Collection definitions
│   ├── alos-palsar.json
│   └── ...
├── openapi/
│   └── openapi.yaml             # OpenAPI specification
├── Dockerfile
├── docker-compose.yml
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## 10. Dependencies

### 10.1 Direct Dependencies

| Package | Purpose |
|---------|---------|
| `net/http` | HTTP server (stdlib) |
| `encoding/json` | JSON encoding/decoding (stdlib) |
| `github.com/go-chi/chi/v5` | HTTP router |
| `github.com/go-chi/cors` | CORS middleware |
| `github.com/caarlos0/env/v10` | Environment config parsing |
| `github.com/paulmach/orb` | Geometry operations |
| `github.com/peterstace/simplefeatures` | WKT parsing/generation |
| `log/slog` | Structured logging (stdlib) |

### 10.2 Development Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/stretchr/testify` | Testing assertions |
| `github.com/jarcoal/httpmock` | HTTP mocking for tests |
| `github.com/golangci/golangci-lint` | Linting |

---

## 11. Deployment

### 11.1 Docker

```dockerfile
# Build stage
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /asf-stac-proxy ./cmd/server

# Runtime stage
FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /asf-stac-proxy .
COPY collections/ ./collections/
EXPOSE 8080
ENTRYPOINT ["./asf-stac-proxy"]
```

### 11.2 Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HOST` | `0.0.0.0` | Listen address |
| `PORT` | `8080` | Listen port |
| `ASF_BASE_URL` | `https://api.daac.asf.alaska.edu` | ASF API base URL |
| `ASF_TIMEOUT` | `30s` | ASF API request timeout |
| `BASE_URL` | (required) | Public URL of this service |
| `STAC_VERSION` | `1.0.0` | STAC version to report |
| `DEFAULT_LIMIT` | `10` | Default page size |
| `MAX_LIMIT` | `250` | Maximum page size |
| `LOG_LEVEL` | `info` | Log level |
| `LOG_FORMAT` | `json` | Log format (json/text) |

### 11.3 Health Check

```go
// GET /health
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
    // Check ASF API connectivity
    // Return health status
}
```

---

## 12. Testing Strategy

### 12.1 Unit Tests

- Translation layer: Test ASF-to-STAC and STAC-to-ASF conversions
- Geometry utilities: Test WKT/GeoJSON conversions
- Pagination: Test token encoding/decoding
- Datetime parsing: Test ISO 8601 interval handling

### 12.2 Integration Tests

- Mock ASF API responses
- Test full request/response cycle for each endpoint
- Test error handling scenarios

### 12.3 Conformance Tests

Run the STAC API Validator against the deployed service:
```bash
stac-api-validator --root-url http://localhost:8080
```

---

## 13. Future Enhancements

### 13.1 Phase 2 (Potential)

- **Caching**: Add response caching with Redis/Memcached
- **Rate Limiting**: Protect ASF API from excessive requests
- **Metrics**: Prometheus metrics endpoint
- **Filter Extension**: Full CQL2 filter support
- **Sort Extension**: Result sorting
- **Fields Extension**: Partial response selection
- **Transaction Extension**: If ASF supports data submission

### 13.2 Additional Collections

As ASF adds new datasets, add corresponding collection configurations.

### 13.3 Authentication

If ASF requires authentication for certain operations, implement OAuth2/Earthdata Login pass-through.

---

## 14. References

- [STAC Specification](https://stacspec.org/)
- [STAC API Specification](https://github.com/radiantearth/stac-api-spec)
- [ASF Search API Documentation](https://docs.asf.alaska.edu/api/keywords/)
- [OGC API - Features](https://ogcapi.ogc.org/features/)
- [GeoJSON Specification (RFC 7946)](https://tools.ietf.org/html/rfc7946)

---

## Appendix A: Collection Configuration Example

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
    "sar:polarizations": [["VV"], ["VH"], ["VV", "VH"], ["HH"], ["HV"], ["HH", "HV"]],
    "sat:orbit_state": ["ascending", "descending"]
  },
  "stac_extensions": [
    "https://stac-extensions.github.io/sar/v1.0.0/schema.json",
    "https://stac-extensions.github.io/sat/v1.0.0/schema.json",
    "https://stac-extensions.github.io/processing/v1.0.0/schema.json"
  ]
}
```

---

## Appendix B: Sample Item Translation

**ASF GeoJSON Feature:**
```json
{
  "type": "Feature",
  "geometry": {
    "type": "Polygon",
    "coordinates": [[[-122.5, 37.0], [-121.5, 37.0], [-121.5, 38.0], [-122.5, 38.0], [-122.5, 37.0]]]
  },
  "properties": {
    "sceneName": "S1A_IW_SLC__1SDV_20230615T140000_20230615T140030_048000_05C000_ABCD",
    "fileID": "S1A_IW_SLC__1SDV_20230615T140000_20230615T140030_048000_05C000_ABCD-SLC",
    "platform": "Sentinel-1A",
    "beamMode": "IW",
    "polarization": "VV+VH",
    "flightDirection": "ASCENDING",
    "processingLevel": "SLC",
    "absoluteOrbit": 48000,
    "relativeOrbit": 35,
    "startTime": "2023-06-15T14:00:00.000000",
    "stopTime": "2023-06-15T14:00:30.000000",
    "url": "https://datapool.asf.alaska.edu/SLC/SA/S1A_IW_SLC__1SDV_20230615T140000_20230615T140030_048000_05C000_ABCD.zip",
    "thumbnail": "https://datapool.asf.alaska.edu/THUMBNAIL/SA/S1A_IW_SLC__1SDV_20230615T140000_20230615T140030_048000_05C000_ABCD.jpg"
  }
}
```

**Translated STAC Item:**
```json
{
  "type": "Feature",
  "stac_version": "1.0.0",
  "stac_extensions": [
    "https://stac-extensions.github.io/sar/v1.0.0/schema.json",
    "https://stac-extensions.github.io/sat/v1.0.0/schema.json",
    "https://stac-extensions.github.io/processing/v1.0.0/schema.json"
  ],
  "id": "S1A_IW_SLC__1SDV_20230615T140000_20230615T140030_048000_05C000_ABCD-SLC",
  "geometry": {
    "type": "Polygon",
    "coordinates": [[[-122.5, 37.0], [-121.5, 37.0], [-121.5, 38.0], [-122.5, 38.0], [-122.5, 37.0]]]
  },
  "bbox": [-122.5, 37.0, -121.5, 38.0],
  "properties": {
    "datetime": null,
    "start_datetime": "2023-06-15T14:00:00Z",
    "end_datetime": "2023-06-15T14:00:30Z",
    "platform": "sentinel-1a",
    "instruments": ["c-sar"],
    "constellation": "sentinel-1",
    "sar:instrument_mode": "IW",
    "sar:frequency_band": "C",
    "sar:polarizations": ["VV", "VH"],
    "sar:product_type": "SLC",
    "sat:orbit_state": "ascending",
    "sat:absolute_orbit": 48000,
    "sat:relative_orbit": 35,
    "processing:level": "L1"
  },
  "collection": "sentinel-1",
  "links": [
    {"rel": "self", "href": "/collections/sentinel-1/items/S1A_IW_SLC__1SDV_20230615T140000_20230615T140030_048000_05C000_ABCD-SLC", "type": "application/geo+json"},
    {"rel": "parent", "href": "/collections/sentinel-1", "type": "application/json"},
    {"rel": "collection", "href": "/collections/sentinel-1", "type": "application/json"},
    {"rel": "root", "href": "/", "type": "application/json"}
  ],
  "assets": {
    "data": {
      "href": "https://datapool.asf.alaska.edu/SLC/SA/S1A_IW_SLC__1SDV_20230615T140000_20230615T140030_048000_05C000_ABCD.zip",
      "title": "SLC Data",
      "type": "application/zip",
      "roles": ["data"]
    },
    "thumbnail": {
      "href": "https://datapool.asf.alaska.edu/THUMBNAIL/SA/S1A_IW_SLC__1SDV_20230615T140000_20230615T140030_048000_05C000_ABCD.jpg",
      "title": "Thumbnail",
      "type": "image/jpeg",
      "roles": ["thumbnail"]
    }
  }
}
```
