// Package geojson provides GeoJSON geometry types and utilities.
package geojson

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Geometry represents a GeoJSON geometry object.
type Geometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

// Point returns the coordinates as a Point [lon, lat].
// Returns error if geometry is not a Point.
func (g *Geometry) Point() ([]float64, error) {
	if g.Type != "Point" {
		return nil, fmt.Errorf("geometry is not a Point, got %s", g.Type)
	}
	var coords []float64
	if err := json.Unmarshal(g.Coordinates, &coords); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Point coordinates: %w", err)
	}
	if len(coords) < 2 {
		return nil, fmt.Errorf("invalid Point coordinates: expected at least 2 values, got %d", len(coords))
	}
	return coords, nil
}

// LineString returns the coordinates as a LineString [][lon, lat].
// Returns error if geometry is not a LineString.
func (g *Geometry) LineString() ([][]float64, error) {
	if g.Type != "LineString" {
		return nil, fmt.Errorf("geometry is not a LineString, got %s", g.Type)
	}
	var coords [][]float64
	if err := json.Unmarshal(g.Coordinates, &coords); err != nil {
		return nil, fmt.Errorf("failed to unmarshal LineString coordinates: %w", err)
	}
	return coords, nil
}

// Polygon returns the coordinates as a Polygon [][][lon, lat].
// Returns error if geometry is not a Polygon.
func (g *Geometry) Polygon() ([][][]float64, error) {
	if g.Type != "Polygon" {
		return nil, fmt.Errorf("geometry is not a Polygon, got %s", g.Type)
	}
	var coords [][][]float64
	if err := json.Unmarshal(g.Coordinates, &coords); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Polygon coordinates: %w", err)
	}
	return coords, nil
}

// MultiPolygon returns the coordinates as a MultiPolygon [][][][lon, lat].
// Returns error if geometry is not a MultiPolygon.
func (g *Geometry) MultiPolygon() ([][][][]float64, error) {
	if g.Type != "MultiPolygon" {
		return nil, fmt.Errorf("geometry is not a MultiPolygon, got %s", g.Type)
	}
	var coords [][][][]float64
	if err := json.Unmarshal(g.Coordinates, &coords); err != nil {
		return nil, fmt.Errorf("failed to unmarshal MultiPolygon coordinates: %w", err)
	}
	return coords, nil
}

// BBox computes the bounding box of the geometry.
// Returns [west, south, east, north].
func (g *Geometry) BBox() ([]float64, error) {
	return ComputeBBox(g)
}

// ComputeBBox computes the bounding box of a geometry.
// Returns [west, south, east, north].
func ComputeBBox(g *Geometry) ([]float64, error) {
	if g == nil {
		return nil, fmt.Errorf("geometry is nil")
	}

	minLon, minLat := math.Inf(1), math.Inf(1)
	maxLon, maxLat := math.Inf(-1), math.Inf(-1)

	switch g.Type {
	case "Point":
		coords, err := g.Point()
		if err != nil {
			return nil, err
		}
		return []float64{coords[0], coords[1], coords[0], coords[1]}, nil

	case "LineString":
		coords, err := g.LineString()
		if err != nil {
			return nil, err
		}
		for _, point := range coords {
			if len(point) < 2 {
				continue
			}
			minLon = math.Min(minLon, point[0])
			maxLon = math.Max(maxLon, point[0])
			minLat = math.Min(minLat, point[1])
			maxLat = math.Max(maxLat, point[1])
		}

	case "Polygon":
		coords, err := g.Polygon()
		if err != nil {
			return nil, err
		}
		for _, ring := range coords {
			for _, point := range ring {
				if len(point) < 2 {
					continue
				}
				minLon = math.Min(minLon, point[0])
				maxLon = math.Max(maxLon, point[0])
				minLat = math.Min(minLat, point[1])
				maxLat = math.Max(maxLat, point[1])
			}
		}

	case "MultiPolygon":
		coords, err := g.MultiPolygon()
		if err != nil {
			return nil, err
		}
		for _, polygon := range coords {
			for _, ring := range polygon {
				for _, point := range ring {
					if len(point) < 2 {
						continue
					}
					minLon = math.Min(minLon, point[0])
					maxLon = math.Max(maxLon, point[0])
					minLat = math.Min(minLat, point[1])
					maxLat = math.Max(maxLat, point[1])
				}
			}
		}

	default:
		return nil, fmt.Errorf("unsupported geometry type: %s", g.Type)
	}

	if math.IsInf(minLon, 0) || math.IsInf(minLat, 0) {
		return nil, fmt.Errorf("failed to compute bounding box: no valid coordinates found")
	}

	return []float64{minLon, minLat, maxLon, maxLat}, nil
}

// NewPolygonFromBBox creates a polygon geometry from a bounding box.
// bbox should be [west, south, east, north].
func NewPolygonFromBBox(bbox []float64) (*Geometry, error) {
	if len(bbox) != 4 {
		return nil, fmt.Errorf("bbox must have 4 values [west, south, east, north], got %d", len(bbox))
	}

	west, south, east, north := bbox[0], bbox[1], bbox[2], bbox[3]

	// Create a rectangular polygon from the bounding box
	coords := [][][]float64{
		{
			{west, south},
			{east, south},
			{east, north},
			{west, north},
			{west, south}, // Close the ring
		},
	}

	coordsJSON, err := json.Marshal(coords)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal polygon coordinates: %w", err)
	}

	return &Geometry{
		Type:        "Polygon",
		Coordinates: coordsJSON,
	}, nil
}

// ToWKT converts a GeoJSON geometry to WKT format.
// Supports Point, Polygon, and MultiPolygon.
func ToWKT(g *Geometry) (string, error) {
	if g == nil {
		return "", fmt.Errorf("geometry is nil")
	}

	switch g.Type {
	case "Point":
		return pointToWKT(g)
	case "Polygon":
		return polygonToWKT(g)
	case "MultiPolygon":
		return multiPolygonToWKT(g)
	default:
		return "", fmt.Errorf("unsupported geometry type for WKT conversion: %s", g.Type)
	}
}

func pointToWKT(g *Geometry) (string, error) {
	coords, err := g.Point()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("POINT(%s %s)", formatFloat(coords[0]), formatFloat(coords[1])), nil
}

func polygonToWKT(g *Geometry) (string, error) {
	coords, err := g.Polygon()
	if err != nil {
		return "", err
	}

	var rings []string
	for _, ring := range coords {
		points := make([]string, len(ring))
		for i, point := range ring {
			if len(point) < 2 {
				return "", fmt.Errorf("invalid point in polygon ring: expected at least 2 coordinates")
			}
			points[i] = fmt.Sprintf("%s %s", formatFloat(point[0]), formatFloat(point[1]))
		}
		rings = append(rings, "("+strings.Join(points, ",")+")")
	}

	return "POLYGON(" + strings.Join(rings, ",") + ")", nil
}

func multiPolygonToWKT(g *Geometry) (string, error) {
	coords, err := g.MultiPolygon()
	if err != nil {
		return "", err
	}

	var polygons []string
	for _, polygon := range coords {
		var rings []string
		for _, ring := range polygon {
			points := make([]string, len(ring))
			for i, point := range ring {
				if len(point) < 2 {
					return "", fmt.Errorf("invalid point in multipolygon ring: expected at least 2 coordinates")
				}
				points[i] = fmt.Sprintf("%s %s", formatFloat(point[0]), formatFloat(point[1]))
			}
			rings = append(rings, "("+strings.Join(points, ",")+")")
		}
		polygons = append(polygons, "("+strings.Join(rings, ",")+")")
	}

	return "MULTIPOLYGON(" + strings.Join(polygons, ",") + ")", nil
}

// FromWKT parses a WKT string into a GeoJSON geometry.
// Supports Point, Polygon, and MultiPolygon.
func FromWKT(wkt string) (*Geometry, error) {
	wkt = strings.TrimSpace(wkt)
	if wkt == "" {
		return nil, fmt.Errorf("empty WKT string")
	}

	// Determine geometry type
	upperWKT := strings.ToUpper(wkt)
	switch {
	case strings.HasPrefix(upperWKT, "POINT"):
		return parsePointWKT(wkt)
	case strings.HasPrefix(upperWKT, "MULTIPOLYGON"):
		return parseMultiPolygonWKT(wkt)
	case strings.HasPrefix(upperWKT, "POLYGON"):
		return parsePolygonWKT(wkt)
	default:
		return nil, fmt.Errorf("unsupported WKT geometry type")
	}
}

func parsePointWKT(wkt string) (*Geometry, error) {
	// Extract content between parentheses
	start := strings.Index(wkt, "(")
	end := strings.LastIndex(wkt, ")")
	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("invalid POINT WKT format")
	}

	content := strings.TrimSpace(wkt[start+1 : end])
	coords, err := parseCoordPair(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse POINT coordinates: %w", err)
	}

	coordsJSON, err := json.Marshal(coords)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal point coordinates: %w", err)
	}

	return &Geometry{
		Type:        "Point",
		Coordinates: coordsJSON,
	}, nil
}

func parsePolygonWKT(wkt string) (*Geometry, error) {
	// Extract content between outer parentheses
	start := strings.Index(wkt, "(")
	end := strings.LastIndex(wkt, ")")
	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("invalid POLYGON WKT format")
	}

	content := wkt[start+1 : end]
	rings, err := parseRings(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse POLYGON rings: %w", err)
	}

	coordsJSON, err := json.Marshal(rings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal polygon coordinates: %w", err)
	}

	return &Geometry{
		Type:        "Polygon",
		Coordinates: coordsJSON,
	}, nil
}

func parseMultiPolygonWKT(wkt string) (*Geometry, error) {
	// Extract content between outer parentheses
	start := strings.Index(wkt, "(")
	end := strings.LastIndex(wkt, ")")
	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("invalid MULTIPOLYGON WKT format")
	}

	content := wkt[start+1 : end]
	polygons, err := parsePolygons(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MULTIPOLYGON polygons: %w", err)
	}

	coordsJSON, err := json.Marshal(polygons)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal multipolygon coordinates: %w", err)
	}

	return &Geometry{
		Type:        "MultiPolygon",
		Coordinates: coordsJSON,
	}, nil
}

// parseCoordPair parses a coordinate pair "lon lat" into [lon, lat]
func parseCoordPair(s string) ([]float64, error) {
	parts := strings.Fields(strings.TrimSpace(s))
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid coordinate pair: %s", s)
	}

	lon, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid longitude: %s", parts[0])
	}

	lat, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid latitude: %s", parts[1])
	}

	return []float64{lon, lat}, nil
}

// parseRing parses a ring string like "(lon lat,lon lat,...)" into [][]float64
func parseRing(s string) ([][]float64, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "(") || !strings.HasSuffix(s, ")") {
		return nil, fmt.Errorf("ring must be enclosed in parentheses")
	}

	content := s[1 : len(s)-1]
	coordPairs := strings.Split(content, ",")

	ring := make([][]float64, 0, len(coordPairs))
	for _, pair := range coordPairs {
		coords, err := parseCoordPair(pair)
		if err != nil {
			return nil, err
		}
		ring = append(ring, coords)
	}

	return ring, nil
}

// parseRings parses multiple rings for a polygon
func parseRings(s string) ([][][]float64, error) {
	s = strings.TrimSpace(s)

	// Split rings by finding matching parentheses
	ringStrings, err := splitByParentheses(s)
	if err != nil {
		return nil, err
	}

	rings := make([][][]float64, 0, len(ringStrings))
	for _, ringStr := range ringStrings {
		ring, err := parseRing(ringStr)
		if err != nil {
			return nil, err
		}
		rings = append(rings, ring)
	}

	return rings, nil
}

// parsePolygons parses multiple polygons for a multipolygon
func parsePolygons(s string) ([][][][]float64, error) {
	s = strings.TrimSpace(s)

	// Split polygons by finding matching double parentheses
	polygonStrings, err := splitPolygons(s)
	if err != nil {
		return nil, err
	}

	polygons := make([][][][]float64, 0, len(polygonStrings))
	for _, polyStr := range polygonStrings {
		// Remove outer parentheses from polygon
		polyStr = strings.TrimSpace(polyStr)
		if strings.HasPrefix(polyStr, "(") && strings.HasSuffix(polyStr, ")") {
			polyStr = polyStr[1 : len(polyStr)-1]
		}

		rings, err := parseRings(polyStr)
		if err != nil {
			return nil, err
		}
		polygons = append(polygons, rings)
	}

	return polygons, nil
}

// splitByParentheses splits a string into substrings enclosed by parentheses
func splitByParentheses(s string) ([]string, error) {
	var result []string
	var current strings.Builder
	depth := 0

	for i, ch := range s {
		switch ch {
		case '(':
			if depth == 0 && current.Len() > 0 {
				// Skip commas and whitespace between groups
				current.Reset()
			}
			current.WriteRune(ch)
			depth++
		case ')':
			current.WriteRune(ch)
			depth--
			if depth == 0 {
				result = append(result, current.String())
				current.Reset()
			} else if depth < 0 {
				return nil, fmt.Errorf("unmatched closing parenthesis at position %d", i)
			}
		case ',':
			if depth == 0 {
				// Skip commas between groups
				continue
			}
			current.WriteRune(ch)
		default:
			if depth > 0 {
				current.WriteRune(ch)
			}
		}
	}

	if depth != 0 {
		return nil, fmt.Errorf("unmatched parentheses")
	}

	return result, nil
}

// splitPolygons splits a multipolygon string into individual polygon strings
func splitPolygons(s string) ([]string, error) {
	var result []string
	var current strings.Builder
	depth := 0

	for i, ch := range s {
		switch ch {
		case '(':
			current.WriteRune(ch)
			depth++
		case ')':
			current.WriteRune(ch)
			depth--
			if depth == 0 {
				result = append(result, current.String())
				current.Reset()
			} else if depth < 0 {
				return nil, fmt.Errorf("unmatched closing parenthesis at position %d", i)
			}
		case ',':
			if depth == 0 {
				// Skip commas between polygons
				continue
			}
			current.WriteRune(ch)
		default:
			if depth > 0 || !isWhitespace(ch) {
				current.WriteRune(ch)
			}
		}
	}

	if depth != 0 {
		return nil, fmt.Errorf("unmatched parentheses")
	}

	// Add any remaining content
	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result, nil
}

// formatFloat formats a float64 for WKT output
func formatFloat(f float64) string {
	// Use strconv for clean formatting without unnecessary decimals
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// isWhitespace checks if a rune is whitespace
func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}
