package translate

import (
	"encoding/json"
	"testing"

	"github.com/rkm/asf-stac-proxy/internal/asf"
)

func TestTranslateASFFeatureToItem_NilFeature(t *testing.T) {
	_, err := TranslateASFFeatureToItem(nil, "sentinel-1", "https://example.com", "1.0.0")
	if err == nil {
		t.Fatal("Expected error for nil feature, got nil")
	}
}

func TestTranslateASFFeatureToItem_MissingID(t *testing.T) {
	feature := &asf.ASFFeature{
		Type:       "Feature",
		Properties: asf.ASFProperties{},
	}

	_, err := TranslateASFFeatureToItem(feature, "sentinel-1", "https://example.com", "1.0.0")
	if err == nil {
		t.Fatal("Expected error for missing ID, got nil")
	}
}

func TestTranslateASFFeatureToItem_Basic(t *testing.T) {
	feature := &asf.ASFFeature{
		Type: "Feature",
		Geometry: &asf.Geometry{
			Type:        "Polygon",
			Coordinates: json.RawMessage(`[[[-122.0, 37.0], [-121.0, 37.0], [-121.0, 38.0], [-122.0, 38.0], [-122.0, 37.0]]]`),
		},
		Properties: asf.ASFProperties{
			SceneName:       "S1A_IW_SLC__1SDV_20230615T140000",
			FileID:          "S1A_IW_SLC__1SDV_20230615T140000-SLC",
			Platform:        "Sentinel-1A",
			Instrument:      "C-SAR",
			BeamModeType:    "IW",
			Polarization:    "VV+VH",
			FlightDirection: "ASCENDING",
			ProcessingLevel: "SLC",
			StartTime:       "2023-06-15T14:00:00.000000",
			StopTime:        "2023-06-15T14:01:00.000000",
		},
	}

	item, err := TranslateASFFeatureToItem(feature, "sentinel-1", "https://example.com", "1.0.0")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check ID
	if item.Id != "S1A_IW_SLC__1SDV_20230615T140000-SLC" {
		t.Errorf("Expected ID S1A_IW_SLC__1SDV_20230615T140000-SLC, got %s", item.Id)
	}

	// Check collection
	if item.Collection != "sentinel-1" {
		t.Errorf("Expected collection sentinel-1, got %s", item.Collection)
	}

	// Check geometry is set
	if item.Geometry == nil {
		t.Error("Expected geometry to be set")
	}

	// Check bbox is computed
	if item.Bbox == nil {
		t.Error("Expected bbox to be computed")
	}
}

func TestTranslateASFFeatureToItem_PlatformMapping(t *testing.T) {
	feature := &asf.ASFFeature{
		Type: "Feature",
		Properties: asf.ASFProperties{
			FileID:   "test-id",
			Platform: "Sentinel-1A",
		},
	}

	item, err := TranslateASFFeatureToItem(feature, "sentinel-1", "https://example.com", "1.0.0")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Platform should be lowercase
	if item.Properties["platform"] != "sentinel-1a" {
		t.Errorf("Expected platform sentinel-1a, got %v", item.Properties["platform"])
	}

	// Constellation should be set
	if item.Properties["constellation"] != "sentinel-1" {
		t.Errorf("Expected constellation sentinel-1, got %v", item.Properties["constellation"])
	}
}

func TestTranslateASFFeatureToItem_InstrumentMapping(t *testing.T) {
	feature := &asf.ASFFeature{
		Type: "Feature",
		Properties: asf.ASFProperties{
			FileID:     "test-id",
			Instrument: "C-SAR",
		},
	}

	item, err := TranslateASFFeatureToItem(feature, "sentinel-1", "https://example.com", "1.0.0")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	instruments, ok := item.Properties["instruments"].([]string)
	if !ok {
		t.Fatal("Expected instruments to be []string")
	}
	if len(instruments) != 1 || instruments[0] != "c-sar" {
		t.Errorf("Expected instruments [c-sar], got %v", instruments)
	}
}

func TestTranslateASFFeatureToItem_SARExtension(t *testing.T) {
	feature := &asf.ASFFeature{
		Type: "Feature",
		Properties: asf.ASFProperties{
			FileID:          "test-id",
			Platform:        "Sentinel-1A",
			BeamModeType:    "IW",
			Polarization:    "VV+VH",
			ProcessingLevel: "SLC",
			LookDirection:   "RIGHT",
		},
	}

	item, err := TranslateASFFeatureToItem(feature, "sentinel-1", "https://example.com", "1.0.0")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check SAR extension properties
	if item.Properties["sar:instrument_mode"] != "IW" {
		t.Errorf("Expected sar:instrument_mode IW, got %v", item.Properties["sar:instrument_mode"])
	}

	if item.Properties["sar:frequency_band"] != "C" {
		t.Errorf("Expected sar:frequency_band C, got %v", item.Properties["sar:frequency_band"])
	}

	polarizations, ok := item.Properties["sar:polarizations"].([]string)
	if !ok {
		t.Fatal("Expected sar:polarizations to be []string")
	}
	if len(polarizations) != 2 || polarizations[0] != "VV" || polarizations[1] != "VH" {
		t.Errorf("Expected sar:polarizations [VV, VH], got %v", polarizations)
	}

	if item.Properties["sar:product_type"] != "SLC" {
		t.Errorf("Expected sar:product_type SLC, got %v", item.Properties["sar:product_type"])
	}

	if item.Properties["sar:observation_direction"] != "right" {
		t.Errorf("Expected sar:observation_direction right, got %v", item.Properties["sar:observation_direction"])
	}
}

func TestTranslateASFFeatureToItem_SatelliteExtension(t *testing.T) {
	relOrbit := 42
	absOrbit := 12345

	feature := &asf.ASFFeature{
		Type: "Feature",
		Properties: asf.ASFProperties{
			FileID:          "test-id",
			FlightDirection: "ASCENDING",
			RelativeOrbit:   &relOrbit,
			AbsoluteOrbit:   &absOrbit,
		},
	}

	item, err := TranslateASFFeatureToItem(feature, "sentinel-1", "https://example.com", "1.0.0")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check satellite extension properties
	if item.Properties["sat:orbit_state"] != "ascending" {
		t.Errorf("Expected sat:orbit_state ascending, got %v", item.Properties["sat:orbit_state"])
	}

	if item.Properties["sat:relative_orbit"] != 42 {
		t.Errorf("Expected sat:relative_orbit 42, got %v", item.Properties["sat:relative_orbit"])
	}

	if item.Properties["sat:absolute_orbit"] != 12345 {
		t.Errorf("Expected sat:absolute_orbit 12345, got %v", item.Properties["sat:absolute_orbit"])
	}
}

func TestTranslateASFFeatureToItem_ViewExtension(t *testing.T) {
	offNadir := 23.5

	feature := &asf.ASFFeature{
		Type: "Feature",
		Properties: asf.ASFProperties{
			FileID:        "test-id",
			OffNadirAngle: &offNadir,
		},
	}

	item, err := TranslateASFFeatureToItem(feature, "sentinel-1", "https://example.com", "1.0.0")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if item.Properties["view:off_nadir"] != 23.5 {
		t.Errorf("Expected view:off_nadir 23.5, got %v", item.Properties["view:off_nadir"])
	}
}

func TestTranslateASFFeatureToItem_TemporalProperties(t *testing.T) {
	feature := &asf.ASFFeature{
		Type: "Feature",
		Properties: asf.ASFProperties{
			FileID:    "test-id",
			StartTime: "2023-06-15T14:00:00.000000",
			StopTime:  "2023-06-15T14:01:00.000000",
		},
	}

	item, err := TranslateASFFeatureToItem(feature, "sentinel-1", "https://example.com", "1.0.0")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// datetime should be nil for time ranges
	if item.Properties["datetime"] != nil {
		t.Errorf("Expected datetime to be nil, got %v", item.Properties["datetime"])
	}

	// start_datetime and end_datetime should be set
	if item.Properties["start_datetime"] == nil {
		t.Error("Expected start_datetime to be set")
	}
	if item.Properties["end_datetime"] == nil {
		t.Error("Expected end_datetime to be set")
	}
}

func TestTranslateASFFeatureToItem_Links(t *testing.T) {
	feature := &asf.ASFFeature{
		Type: "Feature",
		Properties: asf.ASFProperties{
			FileID: "test-id",
		},
	}

	item, err := TranslateASFFeatureToItem(feature, "sentinel-1", "https://example.com", "1.0.0")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check links
	expectedLinks := map[string]string{
		"self":       "https://example.com/collections/sentinel-1/items/test-id",
		"parent":     "https://example.com/collections/sentinel-1",
		"collection": "https://example.com/collections/sentinel-1",
		"root":       "https://example.com",
	}

	for _, link := range item.Links {
		expectedHref, ok := expectedLinks[link.Rel]
		if ok {
			if link.Href != expectedHref {
				t.Errorf("Expected %s link href %s, got %s", link.Rel, expectedHref, link.Href)
			}
		}
	}
}

func TestTranslateASFFeatureToItem_Assets(t *testing.T) {
	feature := &asf.ASFFeature{
		Type: "Feature",
		Properties: asf.ASFProperties{
			FileID:    "test-id",
			URL:       "https://datapool.asf.alaska.edu/SLC/SA/S1A_test.zip",
			Thumbnail: "https://datapool.asf.alaska.edu/THUMBNAIL/SA/S1A_test_thumb.jpg",
			Browse:    []string{"https://datapool.asf.alaska.edu/BROWSE/SA/S1A_test_browse.png"},
		},
	}

	item, err := TranslateASFFeatureToItem(feature, "sentinel-1", "https://example.com", "1.0.0")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check data asset
	if data, ok := item.Assets["data"]; ok {
		if data.Href != "https://datapool.asf.alaska.edu/SLC/SA/S1A_test.zip" {
			t.Errorf("Unexpected data asset href: %s", data.Href)
		}
		if data.Type != "application/zip" {
			t.Errorf("Expected data asset type application/zip, got %s", data.Type)
		}
	} else {
		t.Error("Expected data asset")
	}

	// Check thumbnail asset
	if thumb, ok := item.Assets["thumbnail"]; ok {
		if thumb.Href != "https://datapool.asf.alaska.edu/THUMBNAIL/SA/S1A_test_thumb.jpg" {
			t.Errorf("Unexpected thumbnail asset href: %s", thumb.Href)
		}
		if thumb.Type != "image/jpeg" {
			t.Errorf("Expected thumbnail type image/jpeg, got %s", thumb.Type)
		}
	} else {
		t.Error("Expected thumbnail asset")
	}

	// Check browse asset
	if browse, ok := item.Assets["browse"]; ok {
		if browse.Href != "https://datapool.asf.alaska.edu/BROWSE/SA/S1A_test_browse.png" {
			t.Errorf("Unexpected browse asset href: %s", browse.Href)
		}
	} else {
		t.Error("Expected browse asset")
	}
}

func TestTranslateASFFeatureToItem_FallbackToSceneName(t *testing.T) {
	feature := &asf.ASFFeature{
		Type: "Feature",
		Properties: asf.ASFProperties{
			SceneName: "S1A_IW_SLC__1SDV_20230615T140000",
			// FileID is empty
		},
	}

	item, err := TranslateASFFeatureToItem(feature, "sentinel-1", "https://example.com", "1.0.0")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if item.Id != "S1A_IW_SLC__1SDV_20230615T140000" {
		t.Errorf("Expected ID from sceneName, got %s", item.Id)
	}
}

func TestParsePolarizations(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "VV+VH",
			input:    "VV+VH",
			expected: []string{"VV", "VH"},
		},
		{
			name:     "HH+HV",
			input:    "HH+HV",
			expected: []string{"HH", "HV"},
		},
		{
			name:     "single polarization",
			input:    "VV",
			expected: []string{"VV"},
		},
		{
			name:     "comma separated",
			input:    "VV,VH",
			expected: []string{"VV", "VH"},
		},
		{
			name:     "space separated",
			input:    "VV VH",
			expected: []string{"VV", "VH"},
		},
		{
			name:     "lowercase input",
			input:    "vv+vh",
			expected: []string{"VV", "VH"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "with extra whitespace",
			input:    " VV + VH ",
			expected: []string{"VV", "VH"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePolarizations(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d polarizations, got %d", len(tt.expected), len(result))
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("Expected polarization %s, got %s", expected, result[i])
				}
			}
		})
	}
}

func TestGetConstellation(t *testing.T) {
	tests := []struct {
		platform     string
		expectedConst string
	}{
		{"Sentinel-1A", "sentinel-1"},
		{"Sentinel-1B", "sentinel-1"},
		{"ALOS", "alos"},
		{"ALOS-2", "alos"},
		{"RADARSAT-1", "radarsat"},
		{"RADARSAT-2", "radarsat"},
		{"ERS-1", "ers"},
		{"ERS-2", "ers"},
		{"Unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			result := getConstellation(tt.platform)
			if result != tt.expectedConst {
				t.Errorf("Expected constellation %s for platform %s, got %s", tt.expectedConst, tt.platform, result)
			}
		})
	}
}

func TestGetFrequencyBand(t *testing.T) {
	tests := []struct {
		platform   string
		instrument string
		expected   string
	}{
		{"Sentinel-1A", "C-SAR", "C"},
		{"RADARSAT-1", "", "C"},
		{"ERS-1", "", "C"},
		{"ALOS", "PALSAR", "L"},
		{"SEASAT", "", "L"},
		{"JERS-1", "", "L"},
		{"UAVSAR", "", "L"},
		{"Unknown", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			result := getFrequencyBand(tt.platform, tt.instrument)
			if result != tt.expected {
				t.Errorf("Expected band %s for platform %s, got %s", tt.expected, tt.platform, result)
			}
		})
	}
}

func TestMapProcessingLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"RAW", "L0"},
		{"L0", "L0"},
		{"SLC", "L1"},
		{"GRD", "L1"},
		{"GRD_HS", "L1"},
		{"GRD_HD", "L1"},
		{"L1", "L1"},
		{"RTC", "L2"},
		{"GUNW", "L2"},
		{"L2", "L2"},
		{"L3", "L3"},
		{"L4", "L4"},
		{"UNKNOWN", "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapProcessingLevel(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %s for input %s, got %s", tt.expected, tt.input, result)
			}
		})
	}
}

func TestGetMediaTypeFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://example.com/file.zip", "application/zip"},
		{"https://example.com/file.tar.gz", "application/gzip"},
		{"https://example.com/file.tgz", "application/gzip"},
		{"https://example.com/file.tif", "image/tiff; application=geotiff"},
		{"https://example.com/file.tiff", "image/tiff; application=geotiff"},
		{"https://example.com/file.jpg", "image/jpeg"},
		{"https://example.com/file.jpeg", "image/jpeg"},
		{"https://example.com/file.png", "image/png"},
		{"https://example.com/file.json", "application/json"},
		{"https://example.com/file.nc", "application/netcdf"},
		{"https://example.com/file.h5", "application/x-hdf5"},
		{"https://example.com/file.hdf5", "application/x-hdf5"},
		{"https://example.com/file.unknown", "application/octet-stream"},
		{"https://example.com/file", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := getMediaTypeFromURL(tt.url)
			if result != tt.expected {
				t.Errorf("Expected %s for URL %s, got %s", tt.expected, tt.url, result)
			}
		})
	}
}
