package stac

import "testing"

func TestMapSTACFieldToASFSort(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:     "datetime",
			input:    "datetime",
			expected: "startTime",
		},
		{
			name:     "start_datetime",
			input:    "start_datetime",
			expected: "startTime",
		},
		{
			name:     "properties.datetime",
			input:    "properties.datetime",
			expected: "startTime",
		},
		{
			name:     "properties.start_datetime",
			input:    "properties.start_datetime",
			expected: "startTime",
		},
		{
			name:     "end_datetime",
			input:    "end_datetime",
			expected: "stopTime",
		},
		{
			name:     "properties.end_datetime",
			input:    "properties.end_datetime",
			expected: "stopTime",
		},
		{
			name:     "platform",
			input:    "platform",
			expected: "platform",
		},
		{
			name:     "properties.platform",
			input:    "properties.platform",
			expected: "platform",
		},
		{
			name:     "collection",
			input:    "collection",
			expected: "dataset",
		},
		{
			name:        "unsupported field",
			input:       "unknown_field",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MapSTACFieldToASFSort(tt.input)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestMapSTACFieldToCMRSort(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		direction SortDirection
		expected  string
	}{
		{
			name:      "datetime ascending",
			field:     "datetime",
			direction: SortAsc,
			expected:  "start_date",
		},
		{
			name:      "datetime descending",
			field:     "datetime",
			direction: SortDesc,
			expected:  "-start_date",
		},
		{
			name:      "start_datetime ascending",
			field:     "start_datetime",
			direction: SortAsc,
			expected:  "start_date",
		},
		{
			name:      "end_datetime ascending",
			field:     "end_datetime",
			direction: SortAsc,
			expected:  "end_date",
		},
		{
			name:      "end_datetime descending",
			field:     "end_datetime",
			direction: SortDesc,
			expected:  "-end_date",
		},
		{
			name:      "platform ascending",
			field:     "platform",
			direction: SortAsc,
			expected:  "platform",
		},
		{
			name:      "platform descending",
			field:     "platform",
			direction: SortDesc,
			expected:  "-platform",
		},
		{
			name:      "unknown field falls back to start_date ascending",
			field:     "unknown_field",
			direction: SortAsc,
			expected:  "start_date",
		},
		{
			name:      "unknown field falls back to start_date descending",
			field:     "unknown_field",
			direction: SortDesc,
			expected:  "-start_date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapSTACFieldToCMRSort(tt.field, tt.direction)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}
