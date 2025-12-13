package cmr

import (
	"encoding/json"
	"fmt"
	"time"
)

// UMMSearchResponse represents a CMR UMM-G search response.
type UMMSearchResponse struct {
	Hits  int             `json:"hits"`
	Took  int             `json:"took"`
	Items []UMMResultItem `json:"items"`
}

// UMMResultItem wraps a UMM granule with metadata.
type UMMResultItem struct {
	Meta UMMMeta    `json:"meta"`
	UMM  UMMGranule `json:"umm"`
}

// UMMMeta contains metadata about a CMR result item.
type UMMMeta struct {
	ConceptID    string    `json:"concept-id"`
	RevisionID   int       `json:"revision-id"`
	NativeID     string    `json:"native-id"`
	ProviderID   string    `json:"provider-id"`
	FormatString string    `json:"format"`
	RevisionDate time.Time `json:"revision-date"`
}

// UMMGranule represents a UMM-G (Unified Metadata Model for Granules) record.
type UMMGranule struct {
	GranuleUR                    string                        `json:"GranuleUR"`
	ProviderDates                []ProviderDate                `json:"ProviderDates,omitempty"`
	CollectionReference          CollectionReference           `json:"CollectionReference"`
	RelatedUrls                  []RelatedURL                  `json:"RelatedUrls,omitempty"`
	DataGranule                  *DataGranule                  `json:"DataGranule,omitempty"`
	TemporalExtent               *TemporalExtent               `json:"TemporalExtent,omitempty"`
	SpatialExtent                *SpatialExtent                `json:"SpatialExtent,omitempty"`
	OrbitCalculatedSpatialDomains []OrbitCalculatedSpatialDomain `json:"OrbitCalculatedSpatialDomains,omitempty"`
	Platforms                    []Platform                    `json:"Platforms,omitempty"`
	AdditionalAttributes         []AdditionalAttribute         `json:"AdditionalAttributes,omitempty"`
	CloudCover                   *float64                      `json:"CloudCover,omitempty"`
}

// ProviderDate contains provider date information.
type ProviderDate struct {
	Date string `json:"Date"`
	Type string `json:"Type"` // e.g., "Insert", "Update"
}

// CollectionReference identifies the parent collection.
type CollectionReference struct {
	ShortName string `json:"ShortName"`
	Version   string `json:"Version"`
}

// RelatedURL represents a URL related to the granule.
type RelatedURL struct {
	URL         string `json:"URL"`
	Type        string `json:"Type"`        // e.g., "GET DATA", "GET RELATED VISUALIZATION"
	Subtype     string `json:"Subtype,omitempty"`
	Description string `json:"Description,omitempty"`
	Format      string `json:"Format,omitempty"`
	MimeType    string `json:"MimeType,omitempty"`
	Size        *float64 `json:"Size,omitempty"` // Size in bytes
	SizeUnit    string `json:"SizeUnit,omitempty"`
}

// DataGranule contains data granule information.
type DataGranule struct {
	DayNightFlag       string   `json:"DayNightFlag,omitempty"`
	ProductionDateTime string   `json:"ProductionDateTime,omitempty"`
	Identifiers        []Identifier `json:"Identifiers,omitempty"`
	ArchiveAndDistributionInformation []ArchiveDistInfo `json:"ArchiveAndDistributionInformation,omitempty"`
}

// Identifier contains identifier information.
type Identifier struct {
	Identifier     string `json:"Identifier"`
	IdentifierType string `json:"IdentifierType"` // e.g., "ProducerGranuleId"
}

// ArchiveDistInfo contains archive and distribution information.
type ArchiveDistInfo struct {
	Name             string   `json:"Name"`
	Size             *float64 `json:"Size,omitempty"`
	SizeUnit         string   `json:"SizeUnit,omitempty"`
	Format           string   `json:"Format,omitempty"`
	MimeType         string   `json:"MimeType,omitempty"`
	Checksum         *Checksum `json:"Checksum,omitempty"`
}

// Checksum contains checksum information.
type Checksum struct {
	Value     string `json:"Value"`
	Algorithm string `json:"Algorithm"` // e.g., "MD5"
}

// TemporalExtent contains temporal information.
type TemporalExtent struct {
	RangeDateTime *RangeDateTime `json:"RangeDateTime,omitempty"`
	SingleDateTime string        `json:"SingleDateTime,omitempty"`
}

// RangeDateTime represents a time range.
type RangeDateTime struct {
	BeginningDateTime string `json:"BeginningDateTime"`
	EndingDateTime    string `json:"EndingDateTime"`
}

// SpatialExtent contains spatial information.
type SpatialExtent struct {
	HorizontalSpatialDomain *HorizontalSpatialDomain `json:"HorizontalSpatialDomain,omitempty"`
}

// HorizontalSpatialDomain contains horizontal spatial domain information.
type HorizontalSpatialDomain struct {
	Geometry *Geometry `json:"Geometry,omitempty"`
	Orbit    *Orbit    `json:"Orbit,omitempty"`
}

// Geometry contains geometry information.
type Geometry struct {
	GPolygons     []GPolygon     `json:"GPolygons,omitempty"`
	BoundingRectangles []BoundingRectangle `json:"BoundingRectangles,omitempty"`
	Points        []Point        `json:"Points,omitempty"`
	Lines         []Line         `json:"Lines,omitempty"`
}

// GPolygon represents a polygon geometry.
type GPolygon struct {
	Boundary Boundary `json:"Boundary"`
}

// Boundary contains boundary points.
type Boundary struct {
	Points []Point `json:"Points"`
}

// Point represents a geographic point.
type Point struct {
	Longitude float64 `json:"Longitude"`
	Latitude  float64 `json:"Latitude"`
}

// BoundingRectangle represents a bounding box.
type BoundingRectangle struct {
	WestBoundingCoordinate  float64 `json:"WestBoundingCoordinate"`
	NorthBoundingCoordinate float64 `json:"NorthBoundingCoordinate"`
	EastBoundingCoordinate  float64 `json:"EastBoundingCoordinate"`
	SouthBoundingCoordinate float64 `json:"SouthBoundingCoordinate"`
}

// Line represents a line geometry.
type Line struct {
	Points []Point `json:"Points"`
}

// Orbit contains orbit information.
type Orbit struct {
	AscendingCrossing       float64 `json:"AscendingCrossing,omitempty"`
	StartLatitude          float64 `json:"StartLatitude,omitempty"`
	StartDirection         string  `json:"StartDirection,omitempty"` // "A" or "D"
	EndLatitude            float64 `json:"EndLatitude,omitempty"`
	EndDirection           string  `json:"EndDirection,omitempty"`
}

// OrbitCalculatedSpatialDomain contains calculated orbit spatial information.
type OrbitCalculatedSpatialDomain struct {
	OrbitNumber          *int    `json:"OrbitNumber,omitempty"`
	EquatorCrossingLongitude *float64 `json:"EquatorCrossingLongitude,omitempty"`
	EquatorCrossingDateTime  string  `json:"EquatorCrossingDateTime,omitempty"`
}

// Platform contains platform/instrument information.
type Platform struct {
	ShortName   string       `json:"ShortName"`
	Instruments []Instrument `json:"Instruments,omitempty"`
}

// Instrument contains instrument information.
type Instrument struct {
	ShortName string `json:"ShortName"`
}

// AdditionalAttribute contains additional attribute information.
// This is used for SAR-specific properties like polarization, beam mode, etc.
type AdditionalAttribute struct {
	Name   string      `json:"Name"`
	Values []string    `json:"Values"`
}

// GetAdditionalAttribute retrieves a specific additional attribute by name.
func (g *UMMGranule) GetAdditionalAttribute(name string) []string {
	for _, attr := range g.AdditionalAttributes {
		if attr.Name == name {
			return attr.Values
		}
	}
	return nil
}

// GetStartTime returns the start time of the granule.
func (g *UMMGranule) GetStartTime() (time.Time, error) {
	if g.TemporalExtent == nil {
		return time.Time{}, nil
	}

	if g.TemporalExtent.RangeDateTime != nil && g.TemporalExtent.RangeDateTime.BeginningDateTime != "" {
		return parseTime(g.TemporalExtent.RangeDateTime.BeginningDateTime)
	}

	if g.TemporalExtent.SingleDateTime != "" {
		return parseTime(g.TemporalExtent.SingleDateTime)
	}

	return time.Time{}, nil
}

// GetEndTime returns the end time of the granule.
func (g *UMMGranule) GetEndTime() (time.Time, error) {
	if g.TemporalExtent == nil {
		return time.Time{}, nil
	}

	if g.TemporalExtent.RangeDateTime != nil && g.TemporalExtent.RangeDateTime.EndingDateTime != "" {
		return parseTime(g.TemporalExtent.RangeDateTime.EndingDateTime)
	}

	if g.TemporalExtent.SingleDateTime != "" {
		return parseTime(g.TemporalExtent.SingleDateTime)
	}

	return time.Time{}, nil
}

// GetDataURL returns the primary data download URL.
func (g *UMMGranule) GetDataURL() string {
	for _, url := range g.RelatedUrls {
		if url.Type == "GET DATA" {
			return url.URL
		}
	}
	return ""
}

// GetBrowseURL returns the browse/thumbnail URL.
func (g *UMMGranule) GetBrowseURL() string {
	for _, url := range g.RelatedUrls {
		if url.Type == "GET RELATED VISUALIZATION" {
			return url.URL
		}
	}
	return ""
}

// GetGeometry returns the granule geometry as GeoJSON.
func (g *UMMGranule) GetGeometry() (json.RawMessage, error) {
	if g.SpatialExtent == nil || g.SpatialExtent.HorizontalSpatialDomain == nil {
		return nil, nil
	}

	geom := g.SpatialExtent.HorizontalSpatialDomain.Geometry
	if geom == nil {
		return nil, nil
	}

	// Try polygons first
	if len(geom.GPolygons) > 0 {
		poly := geom.GPolygons[0]
		coords := make([][]float64, len(poly.Boundary.Points))
		for i, pt := range poly.Boundary.Points {
			coords[i] = []float64{pt.Longitude, pt.Latitude}
		}
		// Close the ring if not already closed
		if len(coords) > 0 {
			first := coords[0]
			last := coords[len(coords)-1]
			if first[0] != last[0] || first[1] != last[1] {
				coords = append(coords, first)
			}
		}
		geojson := map[string]interface{}{
			"type":        "Polygon",
			"coordinates": []interface{}{coords},
		}
		return json.Marshal(geojson)
	}

	// Try bounding rectangles
	if len(geom.BoundingRectangles) > 0 {
		rect := geom.BoundingRectangles[0]
		coords := [][]float64{
			{rect.WestBoundingCoordinate, rect.SouthBoundingCoordinate},
			{rect.EastBoundingCoordinate, rect.SouthBoundingCoordinate},
			{rect.EastBoundingCoordinate, rect.NorthBoundingCoordinate},
			{rect.WestBoundingCoordinate, rect.NorthBoundingCoordinate},
			{rect.WestBoundingCoordinate, rect.SouthBoundingCoordinate},
		}
		geojson := map[string]interface{}{
			"type":        "Polygon",
			"coordinates": []interface{}{coords},
		}
		return json.Marshal(geojson)
	}

	// Try points
	if len(geom.Points) > 0 {
		pt := geom.Points[0]
		geojson := map[string]interface{}{
			"type":        "Point",
			"coordinates": []float64{pt.Longitude, pt.Latitude},
		}
		return json.Marshal(geojson)
	}

	return nil, nil
}

// parseTime parses a CMR timestamp string.
func parseTime(s string) (time.Time, error) {
	// CMR uses ISO 8601 format
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse time: %s", s)
}
