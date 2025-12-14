package translate

import (
	"testing"
	"time"
)

func TestParseASFTime(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectTime  time.Time
		expectError bool
	}{
		{
			name:       "ASF format with microseconds",
			input:      "2023-06-15T14:30:45.123456",
			expectTime: time.Date(2023, 6, 15, 14, 30, 45, 123456000, time.UTC),
		},
		{
			name:       "RFC3339 format",
			input:      "2023-06-15T14:30:45Z",
			expectTime: time.Date(2023, 6, 15, 14, 30, 45, 0, time.UTC),
		},
		{
			name:       "RFC3339Nano format",
			input:      "2023-06-15T14:30:45.123456789Z",
			expectTime: time.Date(2023, 6, 15, 14, 30, 45, 123456789, time.UTC),
		},
		{
			name:       "without timezone",
			input:      "2023-06-15T14:30:45",
			expectTime: time.Date(2023, 6, 15, 14, 30, 45, 0, time.UTC),
		},
		{
			name:       "with whitespace",
			input:      "  2023-06-15T14:30:45Z  ",
			expectTime: time.Date(2023, 6, 15, 14, 30, 45, 0, time.UTC),
		},
		{
			name:        "empty string",
			input:       "",
			expectError: true,
		},
		{
			name:        "invalid format",
			input:       "not a date",
			expectError: true,
		},
		{
			name:        "partial date",
			input:       "2023-06-15",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseASFTime(tt.input)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if !result.Equal(tt.expectTime) {
				t.Errorf("Expected time %v, got %v", tt.expectTime, result)
			}

			// Result should always be in UTC
			if result.Location() != time.UTC {
				t.Errorf("Expected UTC location, got %v", result.Location())
			}
		})
	}
}

func TestFormatSTACTime(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{
			name:     "UTC time",
			input:    time.Date(2023, 6, 15, 14, 30, 45, 0, time.UTC),
			expected: "2023-06-15T14:30:45Z",
		},
		{
			name:     "time with nanoseconds (truncated)",
			input:    time.Date(2023, 6, 15, 14, 30, 45, 123456789, time.UTC),
			expected: "2023-06-15T14:30:45Z",
		},
		{
			name:     "zero time",
			input:    time.Time{},
			expected: "0001-01-01T00:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatSTACTime(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestFormatSTACTime_NonUTC(t *testing.T) {
	// Test that non-UTC times are converted to UTC
	est, _ := time.LoadLocation("America/New_York")
	input := time.Date(2023, 6, 15, 10, 30, 45, 0, est) // 10:30 EST = 14:30 UTC (in summer, EDT is UTC-4)

	result := FormatSTACTime(input)

	// The formatted time should be in UTC
	if result != "2023-06-15T14:30:45Z" {
		t.Errorf("Expected UTC conversion, got %s", result)
	}
}

func TestParseDateTimeInterval_EmptyString(t *testing.T) {
	start, end, err := ParseDateTimeInterval("")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if start != nil || end != nil {
		t.Error("Expected nil start and end for empty string")
	}
}

func TestParseDateTimeInterval_SingleDatetime(t *testing.T) {
	input := "2023-06-15T14:30:45Z"

	start, end, err := ParseDateTimeInterval(input)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if start == nil || end == nil {
		t.Fatal("Expected non-nil start and end")
	}

	expectedTime := time.Date(2023, 6, 15, 14, 30, 45, 0, time.UTC)
	if !start.Equal(expectedTime) {
		t.Errorf("Expected start %v, got %v", expectedTime, *start)
	}
	if !end.Equal(expectedTime) {
		t.Errorf("Expected end %v, got %v", expectedTime, *end)
	}
}

func TestParseDateTimeInterval_ClosedInterval(t *testing.T) {
	input := "2023-06-01T00:00:00Z/2023-06-30T23:59:59Z"

	start, end, err := ParseDateTimeInterval(input)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if start == nil || end == nil {
		t.Fatal("Expected non-nil start and end")
	}

	expectedStart := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2023, 6, 30, 23, 59, 59, 0, time.UTC)

	if !start.Equal(expectedStart) {
		t.Errorf("Expected start %v, got %v", expectedStart, *start)
	}
	if !end.Equal(expectedEnd) {
		t.Errorf("Expected end %v, got %v", expectedEnd, *end)
	}
}

func TestParseDateTimeInterval_OpenStartInterval(t *testing.T) {
	input := "../2023-06-30T23:59:59Z"

	start, end, err := ParseDateTimeInterval(input)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if start != nil {
		t.Errorf("Expected nil start, got %v", *start)
	}
	if end == nil {
		t.Fatal("Expected non-nil end")
	}

	expectedEnd := time.Date(2023, 6, 30, 23, 59, 59, 0, time.UTC)
	if !end.Equal(expectedEnd) {
		t.Errorf("Expected end %v, got %v", expectedEnd, *end)
	}
}

func TestParseDateTimeInterval_OpenEndInterval(t *testing.T) {
	input := "2023-06-01T00:00:00Z/.."

	start, end, err := ParseDateTimeInterval(input)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if start == nil {
		t.Fatal("Expected non-nil start")
	}
	if end != nil {
		t.Errorf("Expected nil end, got %v", *end)
	}

	expectedStart := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	if !start.Equal(expectedStart) {
		t.Errorf("Expected start %v, got %v", expectedStart, *start)
	}
}

func TestParseDateTimeInterval_WithWhitespace(t *testing.T) {
	input := "  2023-06-01T00:00:00Z / 2023-06-30T23:59:59Z  "

	start, end, err := ParseDateTimeInterval(input)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if start == nil || end == nil {
		t.Fatal("Expected non-nil start and end")
	}
}

func TestParseDateTimeInterval_InvalidSingleDatetime(t *testing.T) {
	input := "not-a-date"

	_, _, err := ParseDateTimeInterval(input)
	if err == nil {
		t.Fatal("Expected error for invalid datetime, got nil")
	}
}

func TestParseDateTimeInterval_InvalidStartDatetime(t *testing.T) {
	input := "not-a-date/2023-06-30T23:59:59Z"

	_, _, err := ParseDateTimeInterval(input)
	if err == nil {
		t.Fatal("Expected error for invalid start datetime, got nil")
	}
}

func TestParseDateTimeInterval_InvalidEndDatetime(t *testing.T) {
	input := "2023-06-01T00:00:00Z/not-a-date"

	_, _, err := ParseDateTimeInterval(input)
	if err == nil {
		t.Fatal("Expected error for invalid end datetime, got nil")
	}
}

func TestParseDateTimeInterval_TooManyParts(t *testing.T) {
	input := "2023-06-01T00:00:00Z/2023-06-15T00:00:00Z/2023-06-30T23:59:59Z"

	_, _, err := ParseDateTimeInterval(input)
	if err == nil {
		t.Fatal("Expected error for interval with too many parts, got nil")
	}
}
