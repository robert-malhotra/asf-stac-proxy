package asf

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SearchParams represents parameters for ASF search queries
type SearchParams struct {
	// Dataset filters
	Dataset  []string // ASF dataset names (e.g., "SENTINEL-1", "ALOS")
	Platform []string // Platform names (e.g., "Sentinel-1A", "ALOS")

	// Spatial filters
	IntersectsWith string // WKT geometry string

	// Temporal filters
	Start *time.Time // Start datetime (inclusive)
	End   *time.Time // End datetime (inclusive)

	// Granule identification
	GranuleList []string // List of specific granule names

	// SAR-specific filters
	BeamMode     []string // Beam modes (e.g., "IW", "EW")
	Polarization []string // Polarizations (e.g., "VV", "VH", "HH")

	// Orbital filters
	FlightDirection string // "ASCENDING" or "DESCENDING"
	RelativeOrbit   []int  // Relative orbit numbers
	AbsoluteOrbit   []int  // Absolute orbit numbers

	// Processing filters
	ProcessingLevel []string // Processing levels (e.g., "SLC", "GRD")

	// Geometric filters
	OffNadirAngle []float64 // Off-nadir angle values

	// Sorting
	// Note: ASF API does not support sort direction (sortDir). Results are always sorted in descending order.
	Sort string // ASF sort parameter (e.g., "startTime", "stopTime", "dataset", "platform", "frame", "orbit")

	// Result limiting
	MaxResults int    // Maximum number of results to return
	Output     string // Output format (default: "geojson")
	// Note: ASF API does not support pagination parameters like page or offset.
	// Pagination must be handled client-side or via cursor-based approaches.
}

// ToQueryString converts SearchParams to a URL query string
func (p *SearchParams) ToQueryString() string {
	values := p.ToURLValues()
	return values.Encode()
}

// ToURLValues converts SearchParams to url.Values for query string building
func (p *SearchParams) ToURLValues() url.Values {
	values := url.Values{}

	// Dataset
	if len(p.Dataset) > 0 {
		for _, d := range p.Dataset {
			values.Add("dataset", d)
		}
	}

	// Platform
	if len(p.Platform) > 0 {
		for _, pl := range p.Platform {
			values.Add("platform", pl)
		}
	}

	// Spatial
	if p.IntersectsWith != "" {
		values.Set("intersectsWith", p.IntersectsWith)
	}

	// Temporal
	if p.Start != nil {
		values.Set("start", formatASFTime(p.Start))
	}
	if p.End != nil {
		values.Set("end", formatASFTime(p.End))
	}

	// Granule list
	if len(p.GranuleList) > 0 {
		values.Set("granule_list", strings.Join(p.GranuleList, ","))
	}

	// Beam mode
	if len(p.BeamMode) > 0 {
		for _, bm := range p.BeamMode {
			values.Add("beamMode", bm)
		}
	}

	// Polarization
	if len(p.Polarization) > 0 {
		for _, pol := range p.Polarization {
			values.Add("polarization", pol)
		}
	}

	// Flight direction
	if p.FlightDirection != "" {
		values.Set("flightDirection", p.FlightDirection)
	}

	// Relative orbit
	if len(p.RelativeOrbit) > 0 {
		for _, ro := range p.RelativeOrbit {
			values.Add("relativeOrbit", strconv.Itoa(ro))
		}
	}

	// Absolute orbit
	if len(p.AbsoluteOrbit) > 0 {
		for _, ao := range p.AbsoluteOrbit {
			values.Add("absoluteOrbit", strconv.Itoa(ao))
		}
	}

	// Processing level (comma-separated)
	if len(p.ProcessingLevel) > 0 {
		values.Set("processingLevel", strings.Join(p.ProcessingLevel, ","))
	}

	// Off-nadir angle
	if len(p.OffNadirAngle) > 0 {
		for _, angle := range p.OffNadirAngle {
			values.Add("offNadirAngle", fmt.Sprintf("%.2f", angle))
		}
	}

	// Sort (ASF API doesn't support sort direction)
	if p.Sort != "" {
		values.Set("sort", p.Sort)
	}

	// Max results
	if p.MaxResults > 0 {
		values.Set("maxResults", strconv.Itoa(p.MaxResults))
	}

	// Output format
	if p.Output != "" {
		values.Set("output", p.Output)
	} else {
		values.Set("output", "geojson") // Default to geojson
	}

	return values
}

// formatASFTime formats a time.Time for ASF API queries
// ASF expects ISO 8601 format: YYYY-MM-DDTHH:MM:SSZ
func formatASFTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05Z")
}
