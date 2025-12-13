package asf

import "encoding/json"

// ASFGeoJSONResponse represents ASF's GeoJSON FeatureCollection response
type ASFGeoJSONResponse struct {
	Type     string       `json:"type"` // "FeatureCollection"
	Features []ASFFeature `json:"features"`

	// Pagination metadata (if provided by ASF)
	// Note: ASF API may not always provide these fields
	TotalCount *int `json:"total,omitempty"`
}

// ASFFeature represents a single ASF search result feature
type ASFFeature struct {
	Type       string        `json:"type"` // "Feature"
	Geometry   *Geometry     `json:"geometry"`
	Properties ASFProperties `json:"properties"`
}

// Geometry represents a GeoJSON geometry
type Geometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

// ASFProperties contains ASF-specific metadata for a granule
type ASFProperties struct {
	// Basic metadata
	SceneName  string `json:"sceneName"`
	FileID     string `json:"fileID"`
	Platform   string `json:"platform"`
	Instrument string `json:"instrument"`

	// SAR-specific parameters
	BeamMode     string `json:"beamMode"`
	BeamModeType string `json:"beamModeType"`
	Polarization string `json:"polarization"`

	// Orbital parameters
	FlightDirection string `json:"flightDirection"`
	LookDirection   string `json:"lookDirection"`
	FrameNumber     *int   `json:"frameNumber"`
	AbsoluteOrbit   *int   `json:"absoluteOrbit"`
	RelativeOrbit   *int   `json:"relativeOrbit"`

	// Processing information
	ProcessingLevel string `json:"processingLevel"`
	ProcessingType  string `json:"processingType"`
	ProcessingDate  string `json:"processingDate"`

	// Temporal information
	StartTime string `json:"startTime"`
	StopTime  string `json:"stopTime"`

	// Spatial coordinates
	CenterLat    *float64 `json:"centerLat"`
	CenterLon    *float64 `json:"centerLon"`
	NearStartLat *float64 `json:"nearStartLat"`
	NearStartLon *float64 `json:"nearStartLon"`
	FarStartLat  *float64 `json:"farStartLat"`
	FarStartLon  *float64 `json:"farStartLon"`
	NearEndLat   *float64 `json:"nearEndLat"`
	NearEndLon   *float64 `json:"nearEndLon"`
	FarEndLat    *float64 `json:"farEndLat"`
	FarEndLon    *float64 `json:"farEndLon"`

	// Additional geometric parameters
	FaradayRotation *float64 `json:"faradayRotation"`
	OffNadirAngle   *float64 `json:"offNadirAngle"`
	PathNumber      *int     `json:"pathNumber"`

	// File information
	URL       string           `json:"url"`
	FileName  string           `json:"fileName"`
	FileSize  *int64           `json:"fileSize"`
	Bytes     json.RawMessage  `json:"bytes"`    // Can be int64 or string depending on ASF response
	MD5Sum    string           `json:"md5sum"`
	Browse    []string         `json:"browse"`
	Thumbnail string           `json:"thumbnail"`

	// Grouping and InSAR
	GroupID      string  `json:"groupID"`
	InsarStackID *string `json:"insarStackId"`

	// Baseline parameters
	InsarBaseline         *float64 `json:"insarBaseline"`
	TemporalBaseline      *int     `json:"temporalBaseline"`
	PerpendicularBaseline *float64 `json:"perpendicularBaseline"`
}
