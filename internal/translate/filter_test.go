package translate

import (
	"testing"

	"github.com/rkm/asf-stac-proxy/internal/asf"
)

func TestTranslateCQL2Filter_NilFilter(t *testing.T) {
	params := &asf.SearchParams{}
	err := TranslateCQL2Filter(nil, params)
	if err != nil {
		t.Errorf("Expected no error for nil filter, got %v", err)
	}
}

func TestTranslateCQL2Filter_InvalidFilterType(t *testing.T) {
	params := &asf.SearchParams{}
	err := TranslateCQL2Filter("not a map", params)
	if err == nil {
		t.Fatal("Expected error for non-map filter, got nil")
	}
}

func TestTranslateCQL2Filter_MissingOp(t *testing.T) {
	params := &asf.SearchParams{}
	filter := map[string]any{
		"args": []any{},
	}
	err := TranslateCQL2Filter(filter, params)
	if err == nil {
		t.Fatal("Expected error for missing 'op' field, got nil")
	}
}

func TestTranslateCQL2Filter_MissingArgs(t *testing.T) {
	params := &asf.SearchParams{}
	filter := map[string]any{
		"op": "=",
	}
	err := TranslateCQL2Filter(filter, params)
	if err == nil {
		t.Fatal("Expected error for missing 'args' field, got nil")
	}
}

func TestTranslateCQL2Filter_EqualityOperator(t *testing.T) {
	tests := []struct {
		name           string
		filter         map[string]any
		expectBeamMode []string
		expectError    bool
	}{
		{
			name: "beam mode with = operator",
			filter: map[string]any{
				"op": "=",
				"args": []any{
					map[string]any{"property": "sar:instrument_mode"},
					"IW",
				},
			},
			expectBeamMode: []string{"IW"},
			expectError:    false,
		},
		{
			name: "beam mode with eq operator",
			filter: map[string]any{
				"op": "eq",
				"args": []any{
					map[string]any{"property": "sar:instrument_mode"},
					"EW",
				},
			},
			expectBeamMode: []string{"EW"},
			expectError:    false,
		},
		{
			name: "wrong number of arguments",
			filter: map[string]any{
				"op": "=",
				"args": []any{
					map[string]any{"property": "sar:instrument_mode"},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := &asf.SearchParams{}
			err := TranslateCQL2Filter(tt.filter, params)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if len(params.BeamMode) != len(tt.expectBeamMode) {
				t.Errorf("Expected %d beam modes, got %d", len(tt.expectBeamMode), len(params.BeamMode))
			}
			for i, bm := range tt.expectBeamMode {
				if params.BeamMode[i] != bm {
					t.Errorf("Expected beam mode %s, got %s", bm, params.BeamMode[i])
				}
			}
		})
	}
}

func TestTranslateCQL2Filter_InOperator(t *testing.T) {
	tests := []struct {
		name                  string
		filter                map[string]any
		expectPolarizations   []string
		expectProcessingLevel []string
		expectError           bool
	}{
		{
			name: "polarizations in list",
			filter: map[string]any{
				"op": "in",
				"args": []any{
					map[string]any{"property": "sar:polarizations"},
					[]any{"VV", "VH"},
				},
			},
			expectPolarizations: []string{"VV", "VH"},
			expectError:         false,
		},
		{
			name: "processing levels in list",
			filter: map[string]any{
				"op": "in",
				"args": []any{
					map[string]any{"property": "processing:level"},
					[]any{"SLC", "GRD"},
				},
			},
			expectProcessingLevel: []string{"SLC", "GRD"},
			expectError:           false,
		},
		{
			name: "second argument not an array",
			filter: map[string]any{
				"op": "in",
				"args": []any{
					map[string]any{"property": "sar:polarizations"},
					"not an array",
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := &asf.SearchParams{}
			err := TranslateCQL2Filter(tt.filter, params)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if len(tt.expectPolarizations) > 0 {
				if len(params.Polarization) != len(tt.expectPolarizations) {
					t.Errorf("Expected %d polarizations, got %d", len(tt.expectPolarizations), len(params.Polarization))
				}
				for i, pol := range tt.expectPolarizations {
					if params.Polarization[i] != pol {
						t.Errorf("Expected polarization %s, got %s", pol, params.Polarization[i])
					}
				}
			}

			if len(tt.expectProcessingLevel) > 0 {
				if len(params.ProcessingLevel) != len(tt.expectProcessingLevel) {
					t.Errorf("Expected %d processing levels, got %d", len(tt.expectProcessingLevel), len(params.ProcessingLevel))
				}
				for i, pl := range tt.expectProcessingLevel {
					if params.ProcessingLevel[i] != pl {
						t.Errorf("Expected processing level %s, got %s", pl, params.ProcessingLevel[i])
					}
				}
			}
		})
	}
}

func TestTranslateCQL2Filter_AndOperator(t *testing.T) {
	filter := map[string]any{
		"op": "and",
		"args": []any{
			map[string]any{
				"op": "=",
				"args": []any{
					map[string]any{"property": "sar:instrument_mode"},
					"IW",
				},
			},
			map[string]any{
				"op": "=",
				"args": []any{
					map[string]any{"property": "sat:orbit_state"},
					"ascending",
				},
			},
		},
	}

	params := &asf.SearchParams{}
	err := TranslateCQL2Filter(filter, params)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(params.BeamMode) != 1 || params.BeamMode[0] != "IW" {
		t.Errorf("Expected beam mode IW, got %v", params.BeamMode)
	}

	if params.FlightDirection != "ASCENDING" {
		t.Errorf("Expected flight direction ASCENDING, got %s", params.FlightDirection)
	}
}

func TestTranslateCQL2Filter_OrOperator(t *testing.T) {
	filter := map[string]any{
		"op": "or",
		"args": []any{
			map[string]any{
				"op": "=",
				"args": []any{
					map[string]any{"property": "sar:instrument_mode"},
					"IW",
				},
			},
			map[string]any{
				"op": "=",
				"args": []any{
					map[string]any{"property": "sar:instrument_mode"},
					"EW",
				},
			},
		},
	}

	params := &asf.SearchParams{}
	err := TranslateCQL2Filter(filter, params)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// OR on same property accumulates values
	if len(params.BeamMode) != 2 {
		t.Errorf("Expected 2 beam modes, got %d", len(params.BeamMode))
	}
	if params.BeamMode[0] != "IW" || params.BeamMode[1] != "EW" {
		t.Errorf("Expected beam modes [IW, EW], got %v", params.BeamMode)
	}
}

func TestTranslateCQL2Filter_OrbitState(t *testing.T) {
	tests := []struct {
		name            string
		value           string
		expectDirection string
		expectError     bool
	}{
		{
			name:            "ascending lowercase",
			value:           "ascending",
			expectDirection: "ASCENDING",
		},
		{
			name:            "ASCENDING uppercase",
			value:           "ASCENDING",
			expectDirection: "ASCENDING",
		},
		{
			name:            "descending",
			value:           "descending",
			expectDirection: "DESCENDING",
		},
		{
			name:        "invalid orbit state",
			value:       "sideways",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := map[string]any{
				"op": "=",
				"args": []any{
					map[string]any{"property": "sat:orbit_state"},
					tt.value,
				},
			}

			params := &asf.SearchParams{}
			err := TranslateCQL2Filter(filter, params)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if params.FlightDirection != tt.expectDirection {
				t.Errorf("Expected flight direction %s, got %s", tt.expectDirection, params.FlightDirection)
			}
		})
	}
}

func TestTranslateCQL2Filter_RelativeOrbit(t *testing.T) {
	tests := []struct {
		name         string
		value        any
		expectOrbits []int
		expectError  bool
	}{
		{
			name:         "float64 value (JSON default)",
			value:        float64(42),
			expectOrbits: []int{42},
		},
		{
			name:         "int value",
			value:        42,
			expectOrbits: []int{42},
		},
		{
			name:        "string value - should error",
			value:       "42",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := map[string]any{
				"op": "=",
				"args": []any{
					map[string]any{"property": "sat:relative_orbit"},
					tt.value,
				},
			}

			params := &asf.SearchParams{}
			err := TranslateCQL2Filter(filter, params)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if len(params.RelativeOrbit) != len(tt.expectOrbits) {
				t.Errorf("Expected %d orbits, got %d", len(tt.expectOrbits), len(params.RelativeOrbit))
			}
			for i, orbit := range tt.expectOrbits {
				if params.RelativeOrbit[i] != orbit {
					t.Errorf("Expected orbit %d, got %d", orbit, params.RelativeOrbit[i])
				}
			}
		})
	}
}

func TestTranslateCQL2Filter_AbsoluteOrbit(t *testing.T) {
	filter := map[string]any{
		"op": "=",
		"args": []any{
			map[string]any{"property": "sat:absolute_orbit"},
			float64(12345),
		},
	}

	params := &asf.SearchParams{}
	err := TranslateCQL2Filter(filter, params)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(params.AbsoluteOrbit) != 1 || params.AbsoluteOrbit[0] != 12345 {
		t.Errorf("Expected absolute orbit [12345], got %v", params.AbsoluteOrbit)
	}
}

func TestTranslateCQL2Filter_Platform(t *testing.T) {
	filter := map[string]any{
		"op": "=",
		"args": []any{
			map[string]any{"property": "platform"},
			"Sentinel-1A",
		},
	}

	params := &asf.SearchParams{}
	err := TranslateCQL2Filter(filter, params)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(params.Platform) != 1 || params.Platform[0] != "Sentinel-1A" {
		t.Errorf("Expected platform [Sentinel-1A], got %v", params.Platform)
	}
}

func TestTranslateCQL2Filter_SarProductType(t *testing.T) {
	// sar:product_type maps to processingLevel
	filter := map[string]any{
		"op": "=",
		"args": []any{
			map[string]any{"property": "sar:product_type"},
			"SLC",
		},
	}

	params := &asf.SearchParams{}
	err := TranslateCQL2Filter(filter, params)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(params.ProcessingLevel) != 1 || params.ProcessingLevel[0] != "SLC" {
		t.Errorf("Expected processing level [SLC], got %v", params.ProcessingLevel)
	}
}

func TestTranslateCQL2Filter_UnsupportedProperty(t *testing.T) {
	filter := map[string]any{
		"op": "=",
		"args": []any{
			map[string]any{"property": "unknown:property"},
			"value",
		},
	}

	params := &asf.SearchParams{}
	err := TranslateCQL2Filter(filter, params)
	if err == nil {
		t.Fatal("Expected error for unsupported property, got nil")
	}
}

func TestTranslateCQL2Filter_UnsupportedOperator(t *testing.T) {
	filter := map[string]any{
		"op": "like",
		"args": []any{
			map[string]any{"property": "sar:instrument_mode"},
			"I%",
		},
	}

	params := &asf.SearchParams{}
	err := TranslateCQL2Filter(filter, params)
	if err == nil {
		t.Fatal("Expected error for unsupported operator, got nil")
	}
}

func TestTranslateCQL2Filter_ConflictingOrbitState(t *testing.T) {
	filter := map[string]any{
		"op": "and",
		"args": []any{
			map[string]any{
				"op": "=",
				"args": []any{
					map[string]any{"property": "sat:orbit_state"},
					"ascending",
				},
			},
			map[string]any{
				"op": "=",
				"args": []any{
					map[string]any{"property": "sat:orbit_state"},
					"descending",
				},
			},
		},
	}

	params := &asf.SearchParams{}
	err := TranslateCQL2Filter(filter, params)
	if err == nil {
		t.Fatal("Expected error for conflicting orbit states, got nil")
	}
}

func TestTranslateCQL2Filter_ComplexNestedFilter(t *testing.T) {
	// Complex filter: (beamMode=IW OR beamMode=EW) AND orbit_state=ascending AND relative_orbit in [10, 20]
	filter := map[string]any{
		"op": "and",
		"args": []any{
			map[string]any{
				"op": "or",
				"args": []any{
					map[string]any{
						"op": "=",
						"args": []any{
							map[string]any{"property": "sar:instrument_mode"},
							"IW",
						},
					},
					map[string]any{
						"op": "=",
						"args": []any{
							map[string]any{"property": "sar:instrument_mode"},
							"EW",
						},
					},
				},
			},
			map[string]any{
				"op": "=",
				"args": []any{
					map[string]any{"property": "sat:orbit_state"},
					"ascending",
				},
			},
			map[string]any{
				"op": "in",
				"args": []any{
					map[string]any{"property": "sat:relative_orbit"},
					[]any{float64(10), float64(20)},
				},
			},
		},
	}

	params := &asf.SearchParams{}
	err := TranslateCQL2Filter(filter, params)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check beam modes
	if len(params.BeamMode) != 2 {
		t.Errorf("Expected 2 beam modes, got %d", len(params.BeamMode))
	}

	// Check flight direction
	if params.FlightDirection != "ASCENDING" {
		t.Errorf("Expected ASCENDING, got %s", params.FlightDirection)
	}

	// Check relative orbits
	if len(params.RelativeOrbit) != 2 {
		t.Errorf("Expected 2 relative orbits, got %d", len(params.RelativeOrbit))
	}
	if params.RelativeOrbit[0] != 10 || params.RelativeOrbit[1] != 20 {
		t.Errorf("Expected orbits [10, 20], got %v", params.RelativeOrbit)
	}
}
