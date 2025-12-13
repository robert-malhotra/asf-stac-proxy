package translate

import (
	"fmt"
	"strings"
	"time"
)

// ASF time formats observed in their API responses.
// ASF uses formats like "2023-06-15T14:00:00.000000" or "2023-06-15T14:00:00Z".
var asfTimeFormats = []string{
	"2006-01-02T15:04:05.000000",     // ASF format with microseconds
	"2006-01-02T15:04:05.999999999",  // With nanoseconds
	time.RFC3339Nano,                 // "2006-01-02T15:04:05.999999999Z07:00"
	time.RFC3339,                     // "2006-01-02T15:04:05Z07:00"
	"2006-01-02T15:04:05Z",           // UTC without offset
	"2006-01-02T15:04:05",            // Without timezone
}

// ParseASFTime parses an ASF timestamp string into a time.Time.
// ASF typically uses format: "2023-06-15T14:00:00.000000"
// Returns time in UTC.
func ParseASFTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time string")
	}

	// Trim whitespace
	s = strings.TrimSpace(s)

	// Try each format
	var lastErr error
	for _, format := range asfTimeFormats {
		t, err := time.Parse(format, s)
		if err == nil {
			// Ensure UTC
			return t.UTC(), nil
		}
		lastErr = err
	}

	return time.Time{}, fmt.Errorf("failed to parse ASF time %q: %w", s, lastErr)
}

// FormatSTACTime formats a time.Time as RFC3339 for STAC.
// STAC uses RFC3339 format: "2023-06-15T14:00:00Z"
func FormatSTACTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// ParseDateTimeInterval parses a STAC datetime parameter which can be:
// - A single RFC3339 datetime: "2023-06-15T14:00:00Z"
// - An open-ended interval: "../2023-06-15T14:00:00Z" or "2023-06-15T14:00:00Z/.."
// - A closed interval: "2023-06-15T14:00:00Z/2023-06-16T14:00:00Z"
// Returns start and end times. Either may be nil for open-ended intervals.
func ParseDateTimeInterval(datetime string) (*time.Time, *time.Time, error) {
	if datetime == "" {
		return nil, nil, nil
	}

	datetime = strings.TrimSpace(datetime)

	// Check if it's an interval (contains "/")
	if !strings.Contains(datetime, "/") {
		// Single datetime - use as both start and end
		t, err := time.Parse(time.RFC3339, datetime)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid datetime format: %w", err)
		}
		return &t, &t, nil
	}

	// Split interval
	parts := strings.Split(datetime, "/")
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("invalid datetime interval format: must be 'start/end'")
	}

	startStr := strings.TrimSpace(parts[0])
	endStr := strings.TrimSpace(parts[1])

	var start, end *time.Time

	// Parse start time
	if startStr != "" && startStr != ".." {
		t, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid start datetime: %w", err)
		}
		start = &t
	}

	// Parse end time
	if endStr != "" && endStr != ".." {
		t, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid end datetime: %w", err)
		}
		end = &t
	}

	return start, end, nil
}
