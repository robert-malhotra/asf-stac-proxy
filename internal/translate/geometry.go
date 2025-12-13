package translate

import (
	"fmt"

	"github.com/rkm/asf-stac-proxy/pkg/geojson"
)

// BBoxToWKT converts a STAC bbox to a WKT POLYGON string.
// bbox should be [west, south, east, north] (4 values).
// Returns a WKT POLYGON that can be used with ASF's intersectsWith parameter.
func BBoxToWKT(bbox []float64) (string, error) {
	if len(bbox) != 4 && len(bbox) != 6 {
		return "", fmt.Errorf("bbox must have 4 or 6 values, got %d", len(bbox))
	}

	// For 6-value bbox (3D), we ignore the elevation values
	if len(bbox) == 6 {
		bbox = []float64{bbox[0], bbox[1], bbox[3], bbox[4]}
	}

	// Create a polygon geometry from the bbox
	polygon, err := geojson.NewPolygonFromBBox(bbox)
	if err != nil {
		return "", fmt.Errorf("failed to create polygon from bbox: %w", err)
	}

	// Convert polygon to WKT
	wkt, err := geojson.ToWKT(polygon)
	if err != nil {
		return "", fmt.Errorf("failed to convert polygon to WKT: %w", err)
	}

	return wkt, nil
}

// IntersectsToWKT converts a GeoJSON intersects geometry to WKT.
// The intersects parameter can be a Point, Polygon, or MultiPolygon.
func IntersectsToWKT(g *geojson.Geometry) (string, error) {
	if g == nil {
		return "", fmt.Errorf("geometry is nil")
	}

	wkt, err := geojson.ToWKT(g)
	if err != nil {
		return "", fmt.Errorf("failed to convert geometry to WKT: %w", err)
	}

	return wkt, nil
}
