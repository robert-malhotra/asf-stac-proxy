package cmr

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	gostac "github.com/planetlabs/go-stac"
	"github.com/robert-malhotra/asf-stac-proxy/internal/stac"
)

// Note: STAC extension URIs are defined in internal/stac/models.go but are not
// directly used here because go-stac handles extensions via registered extension objects,
// not string URIs in the Extensions field.

// TranslateGranuleToItem converts a CMR UMM-G granule to a STAC Item.
func TranslateGranuleToItem(granule *UMMGranule, collectionID, baseURL, stacVersion string) (*stac.Item, error) {
	// Use GranuleUR as the item ID
	itemID := granule.GranuleUR
	if itemID == "" {
		return nil, fmt.Errorf("granule has no GranuleUR")
	}

	// Create the STAC item
	item := stac.NewItem(itemID, collectionID, stacVersion)

	// Set geometry
	geom, err := granule.GetGeometry()
	if err != nil {
		return nil, fmt.Errorf("failed to get geometry: %w", err)
	}
	if geom != nil {
		item.Geometry = json.RawMessage(geom)
		// Calculate bbox from geometry
		item.Bbox = calculateBbox(geom)
	}

	// Set temporal properties
	startTime, _ := granule.GetStartTime()
	endTime, _ := granule.GetEndTime()

	if !startTime.IsZero() {
		item.Properties["datetime"] = nil // Use start_datetime/end_datetime instead
		item.Properties["start_datetime"] = startTime.Format(time.RFC3339)
		if !endTime.IsZero() {
			item.Properties["end_datetime"] = endTime.Format(time.RFC3339)
		} else {
			item.Properties["end_datetime"] = startTime.Format(time.RFC3339)
		}
	} else if !endTime.IsZero() {
		item.Properties["datetime"] = endTime.Format(time.RFC3339)
	}

	// Set platform and instrument
	if len(granule.Platforms) > 0 {
		platform := granule.Platforms[0]
		item.Properties["platform"] = strings.ToLower(platform.ShortName)
		item.Properties["constellation"] = getConstellation(platform.ShortName)

		if len(platform.Instruments) > 0 {
			instruments := make([]string, len(platform.Instruments))
			for i, inst := range platform.Instruments {
				instruments[i] = strings.ToLower(inst.ShortName)
			}
			item.Properties["instruments"] = instruments
		}
	}

	// Set SAR extension properties from additional attributes
	setSARProperties(granule, item)

	// Set satellite extension properties
	setSatelliteProperties(granule, item)

	// Set processing extension properties
	setProcessingProperties(granule, item)

	// Add assets
	addAssets(granule, item)

	// Add links
	item.Links = append(item.Links,
		&gostac.Link{
			Rel:  "self",
			Href: fmt.Sprintf("%s/collections/%s/items/%s", baseURL, collectionID, itemID),
			Type: "application/geo+json",
		},
		&gostac.Link{
			Rel:  "parent",
			Href: fmt.Sprintf("%s/collections/%s", baseURL, collectionID),
			Type: "application/json",
		},
		&gostac.Link{
			Rel:  "collection",
			Href: fmt.Sprintf("%s/collections/%s", baseURL, collectionID),
			Type: "application/json",
		},
		&gostac.Link{
			Rel:  "root",
			Href: baseURL + "/",
			Type: "application/json",
		},
	)

	// Note: STAC extensions are handled by go-stac during JSON marshaling
	// based on the extension URIs in stac_extensions field. We don't need to
	// set item.Extensions directly as that expects registered extension objects.

	return item, nil
}

// setSARProperties sets SAR extension properties from CMR additional attributes.
func setSARProperties(granule *UMMGranule, item *stac.Item) {
	// Polarization
	if pols := granule.GetAdditionalAttribute("POLARIZATION"); len(pols) > 0 {
		polarizations := make([]string, len(pols))
		for i, p := range pols {
			polarizations[i] = strings.ToUpper(p)
		}
		item.Properties["sar:polarizations"] = polarizations
	}

	// Beam mode / instrument mode
	if modes := granule.GetAdditionalAttribute("BEAM_MODE"); len(modes) > 0 {
		item.Properties["sar:instrument_mode"] = modes[0]
	} else if modes := granule.GetAdditionalAttribute("BEAM_MODE_TYPE"); len(modes) > 0 {
		item.Properties["sar:instrument_mode"] = modes[0]
	}

	// Frequency band (SAR sensors)
	if len(granule.Platforms) > 0 {
		platform := strings.ToLower(granule.Platforms[0].ShortName)
		switch {
		case strings.Contains(platform, "sentinel-1"):
			item.Properties["sar:frequency_band"] = "C"
			item.Properties["sar:center_frequency"] = 5.405
		case strings.Contains(platform, "alos"):
			item.Properties["sar:frequency_band"] = "L"
			item.Properties["sar:center_frequency"] = 1.27
		case strings.Contains(platform, "ers"):
			item.Properties["sar:frequency_band"] = "C"
			item.Properties["sar:center_frequency"] = 5.3
		case strings.Contains(platform, "radarsat"):
			item.Properties["sar:frequency_band"] = "C"
			item.Properties["sar:center_frequency"] = 5.405
		case strings.Contains(platform, "uavsar"):
			item.Properties["sar:frequency_band"] = "L"
			item.Properties["sar:center_frequency"] = 1.2575
		}
	}

	// Look direction
	if dirs := granule.GetAdditionalAttribute("LOOK_DIRECTION"); len(dirs) > 0 {
		item.Properties["sar:looks_range"] = dirs[0]
	}

	// Product type
	if types := granule.GetAdditionalAttribute("PROCESSING_TYPE"); len(types) > 0 {
		item.Properties["sar:product_type"] = types[0]
	}
}

// setSatelliteProperties sets satellite extension properties.
func setSatelliteProperties(granule *UMMGranule, item *stac.Item) {
	// Orbit state (ascending/descending)
	if dirs := granule.GetAdditionalAttribute("ASCENDING_DESCENDING"); len(dirs) > 0 {
		dir := strings.ToLower(dirs[0])
		if dir == "a" || dir == "ascending" {
			item.Properties["sat:orbit_state"] = "ascending"
		} else if dir == "d" || dir == "descending" {
			item.Properties["sat:orbit_state"] = "descending"
		}
	}

	// Get orbit info from spatial extent
	if granule.SpatialExtent != nil &&
		granule.SpatialExtent.HorizontalSpatialDomain != nil &&
		granule.SpatialExtent.HorizontalSpatialDomain.Orbit != nil {
		orbit := granule.SpatialExtent.HorizontalSpatialDomain.Orbit
		if orbit.StartDirection == "A" {
			item.Properties["sat:orbit_state"] = "ascending"
		} else if orbit.StartDirection == "D" {
			item.Properties["sat:orbit_state"] = "descending"
		}
	}

	// Relative orbit (path number)
	if paths := granule.GetAdditionalAttribute("PATH_NUMBER"); len(paths) > 0 {
		// Try to parse as int
		if len(paths[0]) > 0 {
			item.Properties["sat:relative_orbit"] = paths[0]
		}
	}

	// Absolute orbit
	if orbits := granule.GetAdditionalAttribute("ORBIT_NUMBER"); len(orbits) > 0 {
		item.Properties["sat:absolute_orbit"] = orbits[0]
	}

	// From orbit calculated spatial domains
	if len(granule.OrbitCalculatedSpatialDomains) > 0 {
		orbit := granule.OrbitCalculatedSpatialDomains[0]
		if orbit.OrbitNumber != nil {
			item.Properties["sat:absolute_orbit"] = *orbit.OrbitNumber
		}
	}
}

// setProcessingProperties sets processing extension properties.
func setProcessingProperties(granule *UMMGranule, item *stac.Item) {
	// Processing level
	if levels := granule.GetAdditionalAttribute("PROCESSING_LEVEL"); len(levels) > 0 {
		item.Properties["processing:level"] = levels[0]
	} else if levels := granule.GetAdditionalAttribute("PROCESSING_TYPE"); len(levels) > 0 {
		item.Properties["processing:level"] = levels[0]
	}

	// Processing datetime
	if granule.DataGranule != nil && granule.DataGranule.ProductionDateTime != "" {
		if t, err := parseTime(granule.DataGranule.ProductionDateTime); err == nil {
			item.Properties["processing:datetime"] = t.Format(time.RFC3339)
		}
	}

	// Processing software/facility
	item.Properties["processing:facility"] = "ASF DAAC"
}

// addAssets adds assets to the STAC item.
func addAssets(granule *UMMGranule, item *stac.Item) {
	// Data asset
	dataURL := granule.GetDataURL()
	if dataURL != "" {
		item.Assets["data"] = &gostac.Asset{
			Href:  dataURL,
			Title: "Data",
			Type:  "application/zip",
			Roles: []string{"data"},
		}
	}

	// Browse/thumbnail asset
	browseURL := granule.GetBrowseURL()
	if browseURL != "" {
		item.Assets["thumbnail"] = &gostac.Asset{
			Href:  browseURL,
			Title: "Thumbnail",
			Type:  "image/png",
			Roles: []string{"thumbnail"},
		}
	}

	// Add other related URLs as assets
	for _, relURL := range granule.RelatedUrls {
		if relURL.Type == "GET DATA" || relURL.Type == "GET RELATED VISUALIZATION" {
			continue // Already added
		}

		// Create asset key from type
		key := strings.ToLower(strings.ReplaceAll(relURL.Type, " ", "_"))
		if _, exists := item.Assets[key]; exists {
			continue
		}

		mimeType := relURL.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		item.Assets[key] = &gostac.Asset{
			Href:        relURL.URL,
			Title:       relURL.Description,
			Type:        mimeType,
			Description: relURL.Description,
		}
	}
}

// getConstellation returns the constellation name for a platform.
func getConstellation(platform string) string {
	platform = strings.ToLower(platform)
	switch {
	case strings.HasPrefix(platform, "sentinel-1"):
		return "sentinel-1"
	case strings.HasPrefix(platform, "sentinel-2"):
		return "sentinel-2"
	case strings.HasPrefix(platform, "alos"):
		return "alos"
	case strings.HasPrefix(platform, "ers"):
		return "ers"
	case strings.HasPrefix(platform, "radarsat"):
		return "radarsat"
	default:
		return platform
	}
}

// calculateBbox calculates a bounding box from GeoJSON geometry.
func calculateBbox(geom json.RawMessage) []float64 {
	var g struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	}

	if err := json.Unmarshal(geom, &g); err != nil {
		return nil
	}

	switch g.Type {
	case "Point":
		var coords []float64
		if err := json.Unmarshal(g.Coordinates, &coords); err != nil || len(coords) < 2 {
			return nil
		}
		return []float64{coords[0], coords[1], coords[0], coords[1]}

	case "Polygon":
		var coords [][][]float64
		if err := json.Unmarshal(g.Coordinates, &coords); err != nil || len(coords) == 0 {
			return nil
		}

		ring := coords[0]
		if len(ring) == 0 {
			return nil
		}

		minX, minY := ring[0][0], ring[0][1]
		maxX, maxY := minX, minY

		for _, pt := range ring {
			if pt[0] < minX {
				minX = pt[0]
			}
			if pt[0] > maxX {
				maxX = pt[0]
			}
			if pt[1] < minY {
				minY = pt[1]
			}
			if pt[1] > maxY {
				maxY = pt[1]
			}
		}

		return []float64{minX, minY, maxX, maxY}
	}

	return nil
}
