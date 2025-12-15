package geojson_test

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/robert-malhotra/asf-stac-proxy/pkg/geojson"
)

func ExampleGeometry_Point() {
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
	// Output: Longitude: -122.419400, Latitude: 37.774900
}

func ExampleGeometry_BBox() {
	// Create a Polygon geometry
	coords := [][][]float64{
		{{-122.5, 37.8}, {-122.4, 37.8}, {-122.4, 37.9}, {-122.5, 37.9}, {-122.5, 37.8}},
	}
	coordsJSON, _ := json.Marshal(coords)

	g := &geojson.Geometry{
		Type:        "Polygon",
		Coordinates: coordsJSON,
	}

	// Compute bounding box
	bbox, err := g.BBox()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("BBox: [%f, %f, %f, %f]\n", bbox[0], bbox[1], bbox[2], bbox[3])
	// Output: BBox: [-122.500000, 37.800000, -122.400000, 37.900000]
}

func ExampleToWKT() {
	// Create a Polygon geometry
	coords := [][][]float64{
		{{-122.5, 37.8}, {-122.4, 37.8}, {-122.4, 37.9}, {-122.5, 37.9}, {-122.5, 37.8}},
	}
	coordsJSON, _ := json.Marshal(coords)

	g := &geojson.Geometry{
		Type:        "Polygon",
		Coordinates: coordsJSON,
	}

	// Convert to WKT
	wkt, err := geojson.ToWKT(g)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(wkt)
	// Output: POLYGON((-122.5 37.8,-122.4 37.8,-122.4 37.9,-122.5 37.9,-122.5 37.8))
}

func ExampleFromWKT() {
	// Parse WKT string
	wkt := "POINT(-122.4194 37.7749)"

	g, err := geojson.FromWKT(wkt)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Type: %s\n", g.Type)

	coords, _ := g.Point()
	fmt.Printf("Coordinates: [%f, %f]\n", coords[0], coords[1])
	// Output: Type: Point
	// Coordinates: [-122.419400, 37.774900]
}

func ExampleNewPolygonFromBBox() {
	// Create a polygon from a bounding box
	bbox := []float64{-122.5, 37.8, -122.4, 37.9}

	g, err := geojson.NewPolygonFromBBox(bbox)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Type: %s\n", g.Type)

	// Verify the bounding box
	computedBBox, _ := g.BBox()
	fmt.Printf("BBox: [%f, %f, %f, %f]\n", computedBBox[0], computedBBox[1], computedBBox[2], computedBBox[3])
	// Output: Type: Polygon
	// BBox: [-122.500000, 37.800000, -122.400000, 37.900000]
}

func ExampleComputeBBox() {
	// Create a MultiPolygon geometry
	coords := [][][][]float64{
		{
			{{-122.5, 37.8}, {-122.4, 37.8}, {-122.4, 37.9}, {-122.5, 37.9}, {-122.5, 37.8}},
		},
		{
			{{-123.5, 38.8}, {-123.4, 38.8}, {-123.4, 38.9}, {-123.5, 38.9}, {-123.5, 38.8}},
		},
	}
	coordsJSON, _ := json.Marshal(coords)

	g := &geojson.Geometry{
		Type:        "MultiPolygon",
		Coordinates: coordsJSON,
	}

	// Compute bounding box that encompasses all polygons
	bbox, err := geojson.ComputeBBox(g)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("BBox: [%f, %f, %f, %f]\n", bbox[0], bbox[1], bbox[2], bbox[3])
	// Output: BBox: [-123.500000, 37.800000, -122.400000, 38.900000]
}
