package geojson

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestPoint(t *testing.T) {
	coords := []float64{-122.4, 37.8}
	coordsJSON, _ := json.Marshal(coords)
	g := &Geometry{
		Type:        "Point",
		Coordinates: coordsJSON,
	}

	result, err := g.Point()
	if err != nil {
		t.Fatalf("Point() error: %v", err)
	}

	if len(result) != 2 || result[0] != -122.4 || result[1] != 37.8 {
		t.Errorf("Point() = %v, want [-122.4, 37.8]", result)
	}
}

func TestPoint_WrongType(t *testing.T) {
	coords := [][]float64{{-122.4, 37.8}, {-122.5, 37.9}}
	coordsJSON, _ := json.Marshal(coords)
	g := &Geometry{
		Type:        "LineString",
		Coordinates: coordsJSON,
	}

	_, err := g.Point()
	if err == nil {
		t.Error("Point() should return error for non-Point geometry")
	}
}

func TestLineString(t *testing.T) {
	coords := [][]float64{{-122.4, 37.8}, {-122.5, 37.9}}
	coordsJSON, _ := json.Marshal(coords)
	g := &Geometry{
		Type:        "LineString",
		Coordinates: coordsJSON,
	}

	result, err := g.LineString()
	if err != nil {
		t.Fatalf("LineString() error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("LineString() length = %d, want 2", len(result))
	}
}

func TestPolygon(t *testing.T) {
	coords := [][][]float64{
		{{-122.4, 37.8}, {-122.5, 37.8}, {-122.5, 37.9}, {-122.4, 37.9}, {-122.4, 37.8}},
	}
	coordsJSON, _ := json.Marshal(coords)
	g := &Geometry{
		Type:        "Polygon",
		Coordinates: coordsJSON,
	}

	result, err := g.Polygon()
	if err != nil {
		t.Fatalf("Polygon() error: %v", err)
	}

	if len(result) != 1 || len(result[0]) != 5 {
		t.Errorf("Polygon() structure incorrect")
	}
}

func TestMultiPolygon(t *testing.T) {
	coords := [][][][]float64{
		{
			{{-122.4, 37.8}, {-122.5, 37.8}, {-122.5, 37.9}, {-122.4, 37.9}, {-122.4, 37.8}},
		},
		{
			{{-123.4, 38.8}, {-123.5, 38.8}, {-123.5, 38.9}, {-123.4, 38.9}, {-123.4, 38.8}},
		},
	}
	coordsJSON, _ := json.Marshal(coords)
	g := &Geometry{
		Type:        "MultiPolygon",
		Coordinates: coordsJSON,
	}

	result, err := g.MultiPolygon()
	if err != nil {
		t.Fatalf("MultiPolygon() error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("MultiPolygon() length = %d, want 2", len(result))
	}
}

func TestComputeBBox_Point(t *testing.T) {
	coords := []float64{-122.4, 37.8}
	coordsJSON, _ := json.Marshal(coords)
	g := &Geometry{
		Type:        "Point",
		Coordinates: coordsJSON,
	}

	bbox, err := ComputeBBox(g)
	if err != nil {
		t.Fatalf("ComputeBBox() error: %v", err)
	}

	expected := []float64{-122.4, 37.8, -122.4, 37.8}
	if !floatSlicesEqual(bbox, expected) {
		t.Errorf("ComputeBBox() = %v, want %v", bbox, expected)
	}
}

func TestComputeBBox_Polygon(t *testing.T) {
	coords := [][][]float64{
		{{-122.5, 37.8}, {-122.4, 37.8}, {-122.4, 37.9}, {-122.5, 37.9}, {-122.5, 37.8}},
	}
	coordsJSON, _ := json.Marshal(coords)
	g := &Geometry{
		Type:        "Polygon",
		Coordinates: coordsJSON,
	}

	bbox, err := ComputeBBox(g)
	if err != nil {
		t.Fatalf("ComputeBBox() error: %v", err)
	}

	expected := []float64{-122.5, 37.8, -122.4, 37.9}
	if !floatSlicesEqual(bbox, expected) {
		t.Errorf("ComputeBBox() = %v, want %v", bbox, expected)
	}
}

func TestComputeBBox_MultiPolygon(t *testing.T) {
	coords := [][][][]float64{
		{
			{{-122.5, 37.8}, {-122.4, 37.8}, {-122.4, 37.9}, {-122.5, 37.9}, {-122.5, 37.8}},
		},
		{
			{{-123.5, 38.8}, {-123.4, 38.8}, {-123.4, 38.9}, {-123.5, 38.9}, {-123.5, 38.8}},
		},
	}
	coordsJSON, _ := json.Marshal(coords)
	g := &Geometry{
		Type:        "MultiPolygon",
		Coordinates: coordsJSON,
	}

	bbox, err := ComputeBBox(g)
	if err != nil {
		t.Fatalf("ComputeBBox() error: %v", err)
	}

	// Should span both polygons
	expected := []float64{-123.5, 37.8, -122.4, 38.9}
	if !floatSlicesEqual(bbox, expected) {
		t.Errorf("ComputeBBox() = %v, want %v", bbox, expected)
	}
}

func TestBBoxMethod(t *testing.T) {
	coords := []float64{-122.4, 37.8}
	coordsJSON, _ := json.Marshal(coords)
	g := &Geometry{
		Type:        "Point",
		Coordinates: coordsJSON,
	}

	bbox1, err1 := g.BBox()
	bbox2, err2 := ComputeBBox(g)

	if err1 != nil || err2 != nil {
		t.Fatalf("BBox() errors: %v, %v", err1, err2)
	}

	if !floatSlicesEqual(bbox1, bbox2) {
		t.Errorf("BBox() and ComputeBBox() should return same result")
	}
}

func TestNewPolygonFromBBox(t *testing.T) {
	bbox := []float64{-122.5, 37.8, -122.4, 37.9}

	g, err := NewPolygonFromBBox(bbox)
	if err != nil {
		t.Fatalf("NewPolygonFromBBox() error: %v", err)
	}

	if g.Type != "Polygon" {
		t.Errorf("NewPolygonFromBBox() Type = %s, want Polygon", g.Type)
	}

	coords, err := g.Polygon()
	if err != nil {
		t.Fatalf("Failed to parse created polygon: %v", err)
	}

	if len(coords) != 1 || len(coords[0]) != 5 {
		t.Errorf("NewPolygonFromBBox() created invalid polygon structure")
	}

	// Verify the polygon covers the bbox
	computedBBox, err := ComputeBBox(g)
	if err != nil {
		t.Fatalf("ComputeBBox() error: %v", err)
	}

	if !floatSlicesEqual(computedBBox, bbox) {
		t.Errorf("Computed bbox %v doesn't match original %v", computedBBox, bbox)
	}
}

func TestNewPolygonFromBBox_InvalidInput(t *testing.T) {
	bbox := []float64{-122.5, 37.8, -122.4} // Only 3 values

	_, err := NewPolygonFromBBox(bbox)
	if err == nil {
		t.Error("NewPolygonFromBBox() should return error for invalid bbox")
	}
}

func TestToWKT_Point(t *testing.T) {
	coords := []float64{-122.4, 37.8}
	coordsJSON, _ := json.Marshal(coords)
	g := &Geometry{
		Type:        "Point",
		Coordinates: coordsJSON,
	}

	wkt, err := ToWKT(g)
	if err != nil {
		t.Fatalf("ToWKT() error: %v", err)
	}

	expected := "POINT(-122.4 37.8)"
	if wkt != expected {
		t.Errorf("ToWKT() = %s, want %s", wkt, expected)
	}
}

func TestToWKT_Polygon(t *testing.T) {
	coords := [][][]float64{
		{{-122.5, 37.8}, {-122.4, 37.8}, {-122.4, 37.9}, {-122.5, 37.9}, {-122.5, 37.8}},
	}
	coordsJSON, _ := json.Marshal(coords)
	g := &Geometry{
		Type:        "Polygon",
		Coordinates: coordsJSON,
	}

	wkt, err := ToWKT(g)
	if err != nil {
		t.Fatalf("ToWKT() error: %v", err)
	}

	expected := "POLYGON((-122.5 37.8,-122.4 37.8,-122.4 37.9,-122.5 37.9,-122.5 37.8))"
	if wkt != expected {
		t.Errorf("ToWKT() = %s, want %s", wkt, expected)
	}
}

func TestToWKT_MultiPolygon(t *testing.T) {
	coords := [][][][]float64{
		{
			{{-122.5, 37.8}, {-122.4, 37.8}, {-122.4, 37.9}, {-122.5, 37.9}, {-122.5, 37.8}},
		},
		{
			{{-123.5, 38.8}, {-123.4, 38.8}, {-123.4, 38.9}, {-123.5, 38.9}, {-123.5, 38.8}},
		},
	}
	coordsJSON, _ := json.Marshal(coords)
	g := &Geometry{
		Type:        "MultiPolygon",
		Coordinates: coordsJSON,
	}

	wkt, err := ToWKT(g)
	if err != nil {
		t.Fatalf("ToWKT() error: %v", err)
	}

	expected := "MULTIPOLYGON(((-122.5 37.8,-122.4 37.8,-122.4 37.9,-122.5 37.9,-122.5 37.8)),((-123.5 38.8,-123.4 38.8,-123.4 38.9,-123.5 38.9,-123.5 38.8)))"
	if wkt != expected {
		t.Errorf("ToWKT() = %s, want %s", wkt, expected)
	}
}

func TestFromWKT_Point(t *testing.T) {
	wkt := "POINT(-122.4 37.8)"

	g, err := FromWKT(wkt)
	if err != nil {
		t.Fatalf("FromWKT() error: %v", err)
	}

	if g.Type != "Point" {
		t.Errorf("FromWKT() Type = %s, want Point", g.Type)
	}

	coords, err := g.Point()
	if err != nil {
		t.Fatalf("Failed to parse point: %v", err)
	}

	expected := []float64{-122.4, 37.8}
	if !floatSlicesEqual(coords, expected) {
		t.Errorf("FromWKT() coords = %v, want %v", coords, expected)
	}
}

func TestFromWKT_Polygon(t *testing.T) {
	wkt := "POLYGON((-122.5 37.8,-122.4 37.8,-122.4 37.9,-122.5 37.9,-122.5 37.8))"

	g, err := FromWKT(wkt)
	if err != nil {
		t.Fatalf("FromWKT() error: %v", err)
	}

	if g.Type != "Polygon" {
		t.Errorf("FromWKT() Type = %s, want Polygon", g.Type)
	}

	coords, err := g.Polygon()
	if err != nil {
		t.Fatalf("Failed to parse polygon: %v", err)
	}

	if len(coords) != 1 || len(coords[0]) != 5 {
		t.Errorf("FromWKT() polygon structure incorrect")
	}
}

func TestFromWKT_MultiPolygon(t *testing.T) {
	wkt := "MULTIPOLYGON(((-122.5 37.8,-122.4 37.8,-122.4 37.9,-122.5 37.9,-122.5 37.8)),((-123.5 38.8,-123.4 38.8,-123.4 38.9,-123.5 38.9,-123.5 38.8)))"

	g, err := FromWKT(wkt)
	if err != nil {
		t.Fatalf("FromWKT() error: %v", err)
	}

	if g.Type != "MultiPolygon" {
		t.Errorf("FromWKT() Type = %s, want MultiPolygon", g.Type)
	}

	coords, err := g.MultiPolygon()
	if err != nil {
		t.Fatalf("Failed to parse multipolygon: %v", err)
	}

	if len(coords) != 2 {
		t.Errorf("FromWKT() multipolygon count = %d, want 2", len(coords))
	}
}

func TestWKTRoundTrip_Point(t *testing.T) {
	coords := []float64{-122.4, 37.8}
	coordsJSON, _ := json.Marshal(coords)
	original := &Geometry{
		Type:        "Point",
		Coordinates: coordsJSON,
	}

	wkt, err := ToWKT(original)
	if err != nil {
		t.Fatalf("ToWKT() error: %v", err)
	}

	result, err := FromWKT(wkt)
	if err != nil {
		t.Fatalf("FromWKT() error: %v", err)
	}

	originalCoords, _ := original.Point()
	resultCoords, _ := result.Point()

	if !floatSlicesEqual(originalCoords, resultCoords) {
		t.Errorf("WKT round trip failed: %v != %v", originalCoords, resultCoords)
	}
}

func TestWKTRoundTrip_Polygon(t *testing.T) {
	coords := [][][]float64{
		{{-122.5, 37.8}, {-122.4, 37.8}, {-122.4, 37.9}, {-122.5, 37.9}, {-122.5, 37.8}},
	}
	coordsJSON, _ := json.Marshal(coords)
	original := &Geometry{
		Type:        "Polygon",
		Coordinates: coordsJSON,
	}

	wkt, err := ToWKT(original)
	if err != nil {
		t.Fatalf("ToWKT() error: %v", err)
	}

	result, err := FromWKT(wkt)
	if err != nil {
		t.Fatalf("FromWKT() error: %v", err)
	}

	originalBBox, _ := ComputeBBox(original)
	resultBBox, _ := ComputeBBox(result)

	if !floatSlicesEqual(originalBBox, resultBBox) {
		t.Errorf("WKT round trip bbox mismatch: %v != %v", originalBBox, resultBBox)
	}
}

func TestWKTRoundTrip_MultiPolygon(t *testing.T) {
	coords := [][][][]float64{
		{
			{{-122.5, 37.8}, {-122.4, 37.8}, {-122.4, 37.9}, {-122.5, 37.9}, {-122.5, 37.8}},
		},
		{
			{{-123.5, 38.8}, {-123.4, 38.8}, {-123.4, 38.9}, {-123.5, 38.9}, {-123.5, 38.8}},
		},
	}
	coordsJSON, _ := json.Marshal(coords)
	original := &Geometry{
		Type:        "MultiPolygon",
		Coordinates: coordsJSON,
	}

	wkt, err := ToWKT(original)
	if err != nil {
		t.Fatalf("ToWKT() error: %v", err)
	}

	result, err := FromWKT(wkt)
	if err != nil {
		t.Fatalf("FromWKT() error: %v", err)
	}

	originalBBox, _ := ComputeBBox(original)
	resultBBox, _ := ComputeBBox(result)

	if !floatSlicesEqual(originalBBox, resultBBox) {
		t.Errorf("WKT round trip bbox mismatch: %v != %v", originalBBox, resultBBox)
	}
}

func TestFromWKT_CaseInsensitive(t *testing.T) {
	tests := []string{
		"POINT(-122.4 37.8)",
		"point(-122.4 37.8)",
		"Point(-122.4 37.8)",
		"PoInT(-122.4 37.8)",
	}

	for _, wkt := range tests {
		g, err := FromWKT(wkt)
		if err != nil {
			t.Errorf("FromWKT(%s) error: %v", wkt, err)
			continue
		}
		if g.Type != "Point" {
			t.Errorf("FromWKT(%s) Type = %s, want Point", wkt, g.Type)
		}
	}
}

func TestFromWKT_WithWhitespace(t *testing.T) {
	wkt := "  POINT  (  -122.4   37.8  )  "

	g, err := FromWKT(wkt)
	if err != nil {
		t.Fatalf("FromWKT() error: %v", err)
	}

	coords, err := g.Point()
	if err != nil {
		t.Fatalf("Failed to parse point: %v", err)
	}

	expected := []float64{-122.4, 37.8}
	if !floatSlicesEqual(coords, expected) {
		t.Errorf("FromWKT() coords = %v, want %v", coords, expected)
	}
}

func TestFromWKT_InvalidFormat(t *testing.T) {
	tests := []string{
		"",
		"INVALID",
		"POINT",
		"POINT(",
		"POINT()",
		"POINT(-122.4)",
		"POLYGON",
		"MULTIPOLYGON",
	}

	for _, wkt := range tests {
		_, err := FromWKT(wkt)
		if err == nil {
			t.Errorf("FromWKT(%s) should return error", wkt)
		}
	}
}

func TestToWKT_NilGeometry(t *testing.T) {
	_, err := ToWKT(nil)
	if err == nil {
		t.Error("ToWKT(nil) should return error")
	}
}

func TestComputeBBox_NilGeometry(t *testing.T) {
	_, err := ComputeBBox(nil)
	if err == nil {
		t.Error("ComputeBBox(nil) should return error")
	}
}

func TestComputeBBox_UnsupportedType(t *testing.T) {
	coordsJSON := json.RawMessage(`[]`)
	g := &Geometry{
		Type:        "GeometryCollection",
		Coordinates: coordsJSON,
	}

	_, err := ComputeBBox(g)
	if err == nil {
		t.Error("ComputeBBox() should return error for unsupported type")
	}
}

func TestToWKT_UnsupportedType(t *testing.T) {
	coordsJSON := json.RawMessage(`[]`)
	g := &Geometry{
		Type:        "LineString",
		Coordinates: coordsJSON,
	}

	_, err := ToWKT(g)
	if err == nil {
		t.Error("ToWKT() should return error for unsupported type")
	}
}

func TestJSONMarshaling(t *testing.T) {
	coords := []float64{-122.4, 37.8}
	coordsJSON, _ := json.Marshal(coords)
	original := &Geometry{
		Type:        "Point",
		Coordinates: coordsJSON,
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}

	// Unmarshal from JSON
	var result Geometry
	err = json.Unmarshal(data, &result)
	if err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}

	if result.Type != original.Type {
		t.Errorf("Type mismatch after JSON round trip: %s != %s", result.Type, original.Type)
	}

	// Verify coordinates
	originalCoords, _ := original.Point()
	resultCoords, _ := result.Point()

	if !floatSlicesEqual(originalCoords, resultCoords) {
		t.Errorf("Coordinates mismatch after JSON round trip: %v != %v", originalCoords, resultCoords)
	}
}

func TestPolygonWithHole(t *testing.T) {
	// Polygon with exterior ring and one hole
	coords := [][][]float64{
		// Exterior ring
		{{-122.5, 37.8}, {-122.4, 37.8}, {-122.4, 37.9}, {-122.5, 37.9}, {-122.5, 37.8}},
		// Hole
		{{-122.48, 37.82}, {-122.42, 37.82}, {-122.42, 37.88}, {-122.48, 37.88}, {-122.48, 37.82}},
	}
	coordsJSON, _ := json.Marshal(coords)
	g := &Geometry{
		Type:        "Polygon",
		Coordinates: coordsJSON,
	}

	// Test WKT conversion
	wkt, err := ToWKT(g)
	if err != nil {
		t.Fatalf("ToWKT() error: %v", err)
	}

	// Should contain both rings
	if !strings.Contains(wkt, "POLYGON") {
		t.Error("WKT should contain POLYGON")
	}

	// Count opening parentheses - should be 3 (POLYGON( + 2 rings)
	openParens := strings.Count(wkt, "(")
	if openParens != 3 {
		t.Errorf("Expected 3 opening parentheses for polygon with hole, got %d", openParens)
	}

	// Round trip
	result, err := FromWKT(wkt)
	if err != nil {
		t.Fatalf("FromWKT() error: %v", err)
	}

	resultCoords, err := result.Polygon()
	if err != nil {
		t.Fatalf("Failed to parse result polygon: %v", err)
	}

	if len(resultCoords) != 2 {
		t.Errorf("Expected 2 rings (exterior + hole), got %d", len(resultCoords))
	}
}

// Helper function to compare float slices with tolerance
func floatSlicesEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	const epsilon = 1e-9
	for i := range a {
		if math.Abs(a[i]-b[i]) > epsilon {
			return false
		}
	}
	return true
}
