package cmr

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTranslateGranuleToItem(t *testing.T) {
	granule := &UMMGranule{
		GranuleUR: "S1A_IW_SLC__1SDV_20200101T120000_20200101T120100_030000_037000_ABCD",
		CollectionReference: CollectionReference{
			ShortName: "SENTINEL-1A_SLC",
			Version:   "1",
		},
		TemporalExtent: &TemporalExtent{
			RangeDateTime: &RangeDateTime{
				BeginningDateTime: "2020-01-01T12:00:00.000Z",
				EndingDateTime:    "2020-01-01T12:01:00.000Z",
			},
		},
		SpatialExtent: &SpatialExtent{
			HorizontalSpatialDomain: &HorizontalSpatialDomain{
				Geometry: &Geometry{
					BoundingRectangles: []BoundingRectangle{
						{
							WestBoundingCoordinate:  -10.0,
							SouthBoundingCoordinate: 40.0,
							EastBoundingCoordinate:  10.0,
							NorthBoundingCoordinate: 50.0,
						},
					},
				},
			},
		},
		Platforms: []Platform{
			{
				ShortName: "SENTINEL-1A",
				Instruments: []Instrument{
					{ShortName: "C-SAR"},
				},
			},
		},
		AdditionalAttributes: []AdditionalAttribute{
			{Name: "POLARIZATION", Values: []string{"VV", "VH"}},
			{Name: "BEAM_MODE", Values: []string{"IW"}},
			{Name: "ASCENDING_DESCENDING", Values: []string{"ASCENDING"}},
		},
		RelatedUrls: []RelatedURL{
			{
				URL:  "https://example.com/data.zip",
				Type: "GET DATA",
			},
			{
				URL:  "https://example.com/browse.png",
				Type: "GET RELATED VISUALIZATION",
			},
		},
	}

	item, err := TranslateGranuleToItem(granule, "sentinel-1", "https://stac.example.com", "1.0.0")
	if err != nil {
		t.Fatalf("TranslateGranuleToItem() error = %v", err)
	}

	// Check basic properties
	if item.Id != granule.GranuleUR {
		t.Errorf("Item ID = %s, want %s", item.Id, granule.GranuleUR)
	}

	if item.Collection != "sentinel-1" {
		t.Errorf("Item Collection = %s, want sentinel-1", item.Collection)
	}

	// Check platform
	if platform, ok := item.Properties["platform"].(string); !ok || platform != "sentinel-1a" {
		t.Errorf("Item platform = %v, want sentinel-1a", item.Properties["platform"])
	}

	// Check SAR properties
	if pols, ok := item.Properties["sar:polarizations"].([]string); !ok || len(pols) != 2 {
		t.Errorf("Item sar:polarizations = %v, want [VV VH]", item.Properties["sar:polarizations"])
	}

	if mode, ok := item.Properties["sar:instrument_mode"].(string); !ok || mode != "IW" {
		t.Errorf("Item sar:instrument_mode = %v, want IW", item.Properties["sar:instrument_mode"])
	}

	// Check satellite properties
	if orbitState, ok := item.Properties["sat:orbit_state"].(string); !ok || orbitState != "ascending" {
		t.Errorf("Item sat:orbit_state = %v, want ascending", item.Properties["sat:orbit_state"])
	}

	// Check assets
	if dataAsset, ok := item.Assets["data"]; !ok {
		t.Error("Item missing data asset")
	} else if dataAsset.Href != "https://example.com/data.zip" {
		t.Errorf("Data asset href = %s, want https://example.com/data.zip", dataAsset.Href)
	}

	if thumbAsset, ok := item.Assets["thumbnail"]; !ok {
		t.Error("Item missing thumbnail asset")
	} else if thumbAsset.Href != "https://example.com/browse.png" {
		t.Errorf("Thumbnail asset href = %s, want https://example.com/browse.png", thumbAsset.Href)
	}

	// Check bbox
	if item.Bbox == nil || len(item.Bbox) != 4 {
		t.Errorf("Item bbox = %v, want 4 elements", item.Bbox)
	}

	// Check links
	if len(item.Links) < 4 {
		t.Errorf("Item links count = %d, want at least 4", len(item.Links))
	}
}

func TestUMMGranule_GetStartTime(t *testing.T) {
	tests := []struct {
		name    string
		granule *UMMGranule
		want    time.Time
		wantErr bool
	}{
		{
			name: "range datetime",
			granule: &UMMGranule{
				TemporalExtent: &TemporalExtent{
					RangeDateTime: &RangeDateTime{
						BeginningDateTime: "2020-01-01T12:00:00.000Z",
					},
				},
			},
			want: time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "single datetime",
			granule: &UMMGranule{
				TemporalExtent: &TemporalExtent{
					SingleDateTime: "2020-06-15T08:30:00Z",
				},
			},
			want: time.Date(2020, 6, 15, 8, 30, 0, 0, time.UTC),
		},
		{
			name: "no temporal extent",
			granule: &UMMGranule{},
			want:    time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.granule.GetStartTime()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetStartTime() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("GetStartTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUMMGranule_GetGeometry(t *testing.T) {
	tests := []struct {
		name         string
		granule      *UMMGranule
		wantType     string
		wantNilGeom  bool
	}{
		{
			name: "bounding rectangle",
			granule: &UMMGranule{
				SpatialExtent: &SpatialExtent{
					HorizontalSpatialDomain: &HorizontalSpatialDomain{
						Geometry: &Geometry{
							BoundingRectangles: []BoundingRectangle{
								{
									WestBoundingCoordinate:  -10,
									SouthBoundingCoordinate: 40,
									EastBoundingCoordinate:  10,
									NorthBoundingCoordinate: 50,
								},
							},
						},
					},
				},
			},
			wantType: "Polygon",
		},
		{
			name: "polygon",
			granule: &UMMGranule{
				SpatialExtent: &SpatialExtent{
					HorizontalSpatialDomain: &HorizontalSpatialDomain{
						Geometry: &Geometry{
							GPolygons: []GPolygon{
								{
									Boundary: Boundary{
										Points: []Point{
											{Longitude: -10, Latitude: 40},
											{Longitude: 10, Latitude: 40},
											{Longitude: 10, Latitude: 50},
											{Longitude: -10, Latitude: 50},
										},
									},
								},
							},
						},
					},
				},
			},
			wantType: "Polygon",
		},
		{
			name: "point",
			granule: &UMMGranule{
				SpatialExtent: &SpatialExtent{
					HorizontalSpatialDomain: &HorizontalSpatialDomain{
						Geometry: &Geometry{
							Points: []Point{
								{Longitude: 0, Latitude: 45},
							},
						},
					},
				},
			},
			wantType: "Point",
		},
		{
			name:        "no spatial extent",
			granule:     &UMMGranule{},
			wantNilGeom: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geom, err := tt.granule.GetGeometry()
			if err != nil {
				t.Fatalf("GetGeometry() error = %v", err)
			}

			if tt.wantNilGeom {
				if geom != nil {
					t.Errorf("GetGeometry() = %v, want nil", geom)
				}
				return
			}

			if geom == nil {
				t.Error("GetGeometry() = nil, want geometry")
				return
			}

			// Parse the GeoJSON to check the type
			var g struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(geom, &g); err != nil {
				t.Fatalf("failed to parse geometry: %v", err)
			}

			if g.Type != tt.wantType {
				t.Errorf("GetGeometry() type = %s, want %s", g.Type, tt.wantType)
			}
		})
	}
}

func TestGetAdditionalAttribute(t *testing.T) {
	granule := &UMMGranule{
		AdditionalAttributes: []AdditionalAttribute{
			{Name: "POLARIZATION", Values: []string{"VV", "VH"}},
			{Name: "BEAM_MODE", Values: []string{"IW"}},
		},
	}

	// Test existing attribute
	pols := granule.GetAdditionalAttribute("POLARIZATION")
	if len(pols) != 2 || pols[0] != "VV" || pols[1] != "VH" {
		t.Errorf("GetAdditionalAttribute(POLARIZATION) = %v, want [VV VH]", pols)
	}

	// Test non-existing attribute
	missing := granule.GetAdditionalAttribute("NONEXISTENT")
	if missing != nil {
		t.Errorf("GetAdditionalAttribute(NONEXISTENT) = %v, want nil", missing)
	}
}
