# GeoJSON Package

A lightweight Go package for working with GeoJSON geometry types and WKT (Well-Known Text) conversions.

## Features

- Core GeoJSON geometry types with type-safe coordinate access
- Bounding box computation for all supported geometry types
- WKT (Well-Known Text) conversion utilities (bidirectional)
- No external dependencies (pure Go implementation)
- Production-ready with comprehensive error handling
- 80.5% test coverage

## Supported Geometry Types

- Point
- LineString
- Polygon (including polygons with holes)
- MultiPolygon

## Installation

```bash
go get github.com/rkm/asf-stac-proxy/pkg/geojson
```

## Usage

### Creating and Working with Geometries

```go
import (
    "encoding/json"
    "github.com/rkm/asf-stac-proxy/pkg/geojson"
)

// Create a Point geometry
coords := []float64{-122.4194, 37.7749}
coordsJSON, _ := json.Marshal(coords)

g := &geojson.Geometry{
    Type:        "Point",
    Coordinates: coordsJSON,
}

// Access point coordinates
point, err := g.Point()
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Longitude: %f, Latitude: %f\n", point[0], point[1])
```

### Computing Bounding Boxes

```go
// Create a Polygon
coords := [][][]float64{
    {{-122.5, 37.8}, {-122.4, 37.8}, {-122.4, 37.9}, {-122.5, 37.9}, {-122.5, 37.8}},
}
coordsJSON, _ := json.Marshal(coords)

g := &geojson.Geometry{
    Type:        "Polygon",
    Coordinates: coordsJSON,
}

// Compute bounding box [west, south, east, north]
bbox, err := g.BBox()
// bbox = [-122.5, 37.8, -122.4, 37.9]
```

### Creating Polygons from Bounding Boxes

```go
// Create a rectangular polygon from a bounding box
bbox := []float64{-122.5, 37.8, -122.4, 37.9} // [west, south, east, north]

g, err := geojson.NewPolygonFromBBox(bbox)
// Creates a polygon with 5 points forming a rectangle
```

### WKT Conversion

```go
// GeoJSON to WKT
coords := [][][]float64{
    {{-122.5, 37.8}, {-122.4, 37.8}, {-122.4, 37.9}, {-122.5, 37.9}, {-122.5, 37.8}},
}
coordsJSON, _ := json.Marshal(coords)
g := &geojson.Geometry{
    Type:        "Polygon",
    Coordinates: coordsJSON,
}

wkt, err := geojson.ToWKT(g)
// wkt = "POLYGON((-122.5 37.8,-122.4 37.8,-122.4 37.9,-122.5 37.9,-122.5 37.8))"

// WKT to GeoJSON
g2, err := geojson.FromWKT(wkt)
// Returns a Geometry struct
```

### Working with JSON

The `Geometry` struct can be marshaled/unmarshaled directly with `encoding/json`:

```go
// Unmarshal from JSON
var g geojson.Geometry
err := json.Unmarshal(jsonData, &g)

// Marshal to JSON
jsonData, err := json.Marshal(g)
```

## API Reference

### Types

#### `Geometry`
```go
type Geometry struct {
    Type        string          `json:"type"`
    Coordinates json.RawMessage `json:"coordinates"`
}
```

### Methods

#### `(*Geometry) Point() ([]float64, error)`
Returns coordinates as `[lon, lat]`. Returns error if geometry is not a Point.

#### `(*Geometry) LineString() ([][]float64, error)`
Returns coordinates as `[][lon, lat]`. Returns error if geometry is not a LineString.

#### `(*Geometry) Polygon() ([][][]float64, error)`
Returns coordinates as `[][][lon, lat]`. Returns error if geometry is not a Polygon.

#### `(*Geometry) MultiPolygon() ([][][][]float64, error)`
Returns coordinates as `[][][][lon, lat]`. Returns error if geometry is not a MultiPolygon.

#### `(*Geometry) BBox() ([]float64, error)`
Computes and returns the bounding box as `[west, south, east, north]`.

### Functions

#### `ComputeBBox(g *Geometry) ([]float64, error)`
Computes the bounding box of a geometry. Returns `[west, south, east, north]`.

#### `NewPolygonFromBBox(bbox []float64) (*Geometry, error)`
Creates a rectangular polygon from a bounding box `[west, south, east, north]`.

#### `ToWKT(g *Geometry) (string, error)`
Converts a GeoJSON geometry to WKT format. Supports Point, Polygon, and MultiPolygon.

#### `FromWKT(wkt string) (*Geometry, error)`
Parses a WKT string into a GeoJSON geometry. Supports Point, Polygon, and MultiPolygon. Case-insensitive and handles whitespace gracefully.

## Implementation Details

### Coordinate Handling

The `Coordinates` field uses `json.RawMessage` for flexible coordinate handling. This allows the geometry to store coordinates of varying dimensions without type assertions during JSON marshaling/unmarshaling.

### WKT Parser

The WKT parser is implemented from scratch without external dependencies. It:
- Handles nested parentheses correctly
- Is case-insensitive
- Tolerates extra whitespace
- Validates coordinate formats
- Supports polygons with holes (multiple rings)

### Bounding Box Computation

Bounding boxes are computed by iterating through all coordinates and finding the minimum and maximum longitude and latitude values. The function supports:
- Point: Returns the point itself as a zero-area bbox
- LineString: Computes bbox encompassing all points
- Polygon: Handles exterior and interior rings
- MultiPolygon: Computes bbox encompassing all polygons

## Testing

The package includes comprehensive tests with 80.5% code coverage:

```bash
# Run tests
go test ./pkg/geojson/

# Run tests with coverage
go test -cover ./pkg/geojson/

# Run example tests
go test -v -run Example ./pkg/geojson/
```

## Go Version

This package uses Go 1.25 idioms and requires Go 1.25 or later.

## License

Part of the ASF-STAC proxy project.
