package translate

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/planetlabs/go-stac"
	"github.com/rkm/asf-stac-proxy/internal/asf"
	"github.com/rkm/asf-stac-proxy/pkg/geojson"
)

// TranslateASFFeatureToItem converts an ASF feature to a STAC Item.
// Implements property mapping as defined in design doc section 3.3.
func TranslateASFFeatureToItem(feature *asf.ASFFeature, collectionID, baseURL, stacVersion string) (*stac.Item, error) {
	if feature == nil {
		return nil, fmt.Errorf("feature is nil")
	}

	props := feature.Properties

	// Use fileID as the item ID (primary identifier)
	itemID := props.FileID
	if itemID == "" {
		// Fallback to sceneName if fileID is not present
		itemID = props.SceneName
	}
	if itemID == "" {
		return nil, fmt.Errorf("feature has no fileID or sceneName")
	}

	// Create the STAC item
	item := &stac.Item{
		Version:    stacVersion,
		Id:         itemID,
		Collection: collectionID,
		Properties: make(map[string]any),
		Assets:     make(map[string]*stac.Asset),
		Links:      make([]*stac.Link, 0),
	}

	// Set geometry - convert ASF geometry to GeoJSON
	if feature.Geometry != nil {
		geom, err := convertASFGeometry(feature.Geometry)
		if err != nil {
			return nil, fmt.Errorf("failed to convert geometry: %w", err)
		}
		item.Geometry = geom

		// Compute bbox from geometry
		bbox, err := geojson.ComputeBBox(geom)
		if err == nil {
			item.Bbox = bbox
		}
	}

	// Parse temporal properties
	if props.StartTime != "" {
		t, err := ParseASFTime(props.StartTime)
		if err != nil {
			return nil, fmt.Errorf("failed to parse start time: %w", err)
		}
		item.Properties["start_datetime"] = t
	}

	if props.StopTime != "" {
		t, err := ParseASFTime(props.StopTime)
		if err != nil {
			return nil, fmt.Errorf("failed to parse stop time: %w", err)
		}
		item.Properties["end_datetime"] = t
	}

	// Set datetime properties
	// STAC requires either datetime or start_datetime/end_datetime
	// For time ranges, set datetime to null and use start_datetime/end_datetime
	item.Properties["datetime"] = nil

	// Map basic platform properties
	if props.Platform != "" {
		// Convert platform to lowercase (e.g., "Sentinel-1A" -> "sentinel-1a")
		item.Properties["platform"] = strings.ToLower(props.Platform)
	}

	// Map instrument
	if props.Instrument != "" {
		// Convert to lowercase and create array
		instrument := strings.ToLower(props.Instrument)
		item.Properties["instruments"] = []string{instrument}
	}

	// Determine constellation from platform
	if constellation := getConstellation(props.Platform); constellation != "" {
		item.Properties["constellation"] = constellation
	}

	// SAR Extension properties (https://stac-extensions.github.io/sar/v1.0.0/schema.json)
	// ASF returns beamModeType (e.g., "IW", "EW") for the instrument mode
	if props.BeamModeType != "" {
		item.Properties["sar:instrument_mode"] = props.BeamModeType
	}

	// Determine frequency band from platform/instrument
	if band := getFrequencyBand(props.Platform, props.Instrument); band != "" {
		item.Properties["sar:frequency_band"] = band
	}

	// Parse polarizations (e.g., "VV+VH" -> ["VV", "VH"])
	if props.Polarization != "" {
		polarizations := parsePolarizations(props.Polarization)
		if len(polarizations) > 0 {
			item.Properties["sar:polarizations"] = polarizations
		}
	}

	// Set product type from processing level
	if props.ProcessingLevel != "" {
		item.Properties["sar:product_type"] = props.ProcessingLevel
	}

	// Observation direction from look direction
	if props.LookDirection != "" {
		item.Properties["sar:observation_direction"] = strings.ToLower(props.LookDirection)
	}

	// Satellite Extension properties (https://stac-extensions.github.io/sat/v1.0.0/schema.json)
	if props.FlightDirection != "" {
		// Convert "ASCENDING" -> "ascending", "DESCENDING" -> "descending"
		item.Properties["sat:orbit_state"] = strings.ToLower(props.FlightDirection)
	}

	if props.RelativeOrbit != nil {
		item.Properties["sat:relative_orbit"] = *props.RelativeOrbit
	}

	if props.AbsoluteOrbit != nil {
		item.Properties["sat:absolute_orbit"] = *props.AbsoluteOrbit
	}

	// View Extension properties (https://stac-extensions.github.io/view/v1.0.0/schema.json)
	if props.OffNadirAngle != nil {
		item.Properties["view:off_nadir"] = *props.OffNadirAngle
	}

	// Processing Extension (https://stac-extensions.github.io/processing/v1.0.0/schema.json)
	if props.ProcessingLevel != "" {
		// Map ASF processing level to STAC processing level
		item.Properties["processing:level"] = mapProcessingLevel(props.ProcessingLevel)
	}

	// Note: STAC extensions are handled by go-stac during JSON marshaling
	// based on the extension URIs in stac_extensions field. We don't need to
	// set item.Extensions directly as that expects registered extension objects.

	// Set up assets
	if err := addAssets(item, &props); err != nil {
		return nil, fmt.Errorf("failed to add assets: %w", err)
	}

	// Set up links
	addLinks(item, collectionID, baseURL)

	return item, nil
}

// convertASFGeometry converts ASF geometry to GeoJSON geometry
func convertASFGeometry(asfGeom *asf.Geometry) (*geojson.Geometry, error) {
	if asfGeom == nil {
		return nil, fmt.Errorf("geometry is nil")
	}

	// ASF geometry is already in GeoJSON format
	return &geojson.Geometry{
		Type:        asfGeom.Type,
		Coordinates: asfGeom.Coordinates,
	}, nil
}

// parsePolarizations splits a polarization string like "VV+VH" into ["VV", "VH"]
func parsePolarizations(pol string) []string {
	if pol == "" {
		return nil
	}

	// Split by + or , or space
	parts := strings.FieldsFunc(pol, func(r rune) bool {
		return r == '+' || r == ',' || r == ' '
	})

	// Trim and uppercase each part
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, strings.ToUpper(p))
		}
	}

	return result
}

// getConstellation determines the constellation from the platform name
func getConstellation(platform string) string {
	platform = strings.ToLower(platform)
	switch {
	case strings.HasPrefix(platform, "sentinel-1"):
		return "sentinel-1"
	case strings.HasPrefix(platform, "alos"):
		return "alos"
	case strings.HasPrefix(platform, "radarsat"):
		return "radarsat"
	case strings.HasPrefix(platform, "ers"):
		return "ers"
	default:
		return ""
	}
}

// getFrequencyBand determines the SAR frequency band from platform/instrument
func getFrequencyBand(platform, instrument string) string {
	platform = strings.ToLower(platform)
	instrument = strings.ToLower(instrument)

	// Most SAR satellites use C-band
	switch {
	case strings.HasPrefix(platform, "sentinel-1"):
		return "C"
	case strings.HasPrefix(platform, "radarsat"):
		return "C"
	case strings.HasPrefix(platform, "ers"):
		return "C"
	case strings.Contains(platform, "alos") && strings.Contains(instrument, "palsar"):
		return "L"
	case strings.Contains(platform, "seasat"):
		return "L"
	case strings.Contains(platform, "jers"):
		return "L"
	case strings.Contains(platform, "uavsar"):
		return "L"
	case strings.Contains(platform, "airsar"):
		return "C" // AIRSAR can be multiple bands, C is most common
	case strings.Contains(platform, "sir-c"):
		return "C" // SIR-C has both C and L band
	default:
		return ""
	}
}

// mapProcessingLevel maps ASF processing level to STAC processing level
// ASF uses levels like "SLC", "GRD", "RAW", "L0", "L1", etc.
// STAC processing extension uses levels like "L0", "L1", "L2", etc.
func mapProcessingLevel(asfLevel string) string {
	asfLevel = strings.ToUpper(asfLevel)

	switch asfLevel {
	case "RAW", "L0":
		return "L0"
	case "SLC", "GRD", "L1", "GRD_HS", "GRD_HD", "GRD_MS", "GRD_MD":
		return "L1"
	case "L2", "RTC", "GUNW":
		return "L2"
	case "L3":
		return "L3"
	case "L4":
		return "L4"
	default:
		// Return as-is if we don't recognize it
		return asfLevel
	}
}

// addAssets adds assets (data, thumbnail, browse) to the STAC item
func addAssets(item *stac.Item, props *asf.ASFProperties) error {
	// Main data asset
	if props.URL != "" {
		item.Assets["data"] = &stac.Asset{
			Href:  props.URL,
			Title: "Product Data",
			Type:  getMediaTypeFromURL(props.URL),
			Roles: []string{"data"},
		}

		// Add file size if available
		if props.FileSize != nil && *props.FileSize > 0 {
			// go-stac doesn't have a FileSize field, so we'd need to use a custom field
			// For now, we'll skip this or add it as an extension property
		}
	}

	// Thumbnail asset
	if props.Thumbnail != "" {
		item.Assets["thumbnail"] = &stac.Asset{
			Href:  props.Thumbnail,
			Title: "Thumbnail Image",
			Type:  "image/jpeg",
			Roles: []string{"thumbnail"},
		}
	}

	// Browse image asset (ASF returns array of browse URLs)
	if len(props.Browse) > 0 {
		item.Assets["browse"] = &stac.Asset{
			Href:  props.Browse[0],
			Title: "Browse Image",
			Type:  getMediaTypeFromURL(props.Browse[0]),
			Roles: []string{"overview"},
		}
	}

	return nil
}

// getMediaTypeFromURL attempts to determine MIME type from URL
func getMediaTypeFromURL(url string) string {
	url = strings.ToLower(url)
	switch {
	case strings.HasSuffix(url, ".zip"):
		return "application/zip"
	case strings.HasSuffix(url, ".tar.gz"), strings.HasSuffix(url, ".tgz"):
		return "application/gzip"
	case strings.HasSuffix(url, ".tif"), strings.HasSuffix(url, ".tiff"):
		return "image/tiff; application=geotiff"
	case strings.HasSuffix(url, ".jpg"), strings.HasSuffix(url, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(url, ".png"):
		return "image/png"
	case strings.HasSuffix(url, ".json"):
		return "application/json"
	case strings.HasSuffix(url, ".nc"), strings.HasSuffix(url, ".nc4"):
		return "application/netcdf"
	case strings.HasSuffix(url, ".h5"), strings.HasSuffix(url, ".hdf5"):
		return "application/x-hdf5"
	default:
		return "application/octet-stream"
	}
}

// addLinks adds STAC links (self, parent, collection, root) to the item
func addLinks(item *stac.Item, collectionID, baseURL string) {
	if baseURL == "" {
		return
	}

	// Self link
	item.Links = append(item.Links, &stac.Link{
		Rel:  "self",
		Href: fmt.Sprintf("%s/collections/%s/items/%s", baseURL, collectionID, item.Id),
		Type: "application/geo+json",
	})

	// Parent link (collection)
	item.Links = append(item.Links, &stac.Link{
		Rel:  "parent",
		Href: fmt.Sprintf("%s/collections/%s", baseURL, collectionID),
		Type: "application/json",
	})

	// Collection link
	item.Links = append(item.Links, &stac.Link{
		Rel:  "collection",
		Href: fmt.Sprintf("%s/collections/%s", baseURL, collectionID),
		Type: "application/json",
	})

	// Root link
	item.Links = append(item.Links, &stac.Link{
		Rel:  "root",
		Href: baseURL,
		Type: "application/json",
	})
}

// MarshalItem marshals a STAC item to JSON with proper formatting.
func MarshalItem(item *stac.Item) ([]byte, error) {
	return json.Marshal(item)
}
