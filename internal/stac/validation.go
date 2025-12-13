package stac

import (
	"fmt"
	"strings"
	"time"
)

// ValidateSearchRequest validates a STAC search request
func ValidateSearchRequest(req *SearchRequest) error {
	if req == nil {
		return fmt.Errorf("search request cannot be nil")
	}

	// Validate bbox if provided
	if len(req.BBox) > 0 {
		if err := ValidateBBox(req.BBox); err != nil {
			return fmt.Errorf("invalid bbox: %w", err)
		}
	}

	// Validate datetime if provided
	if req.DateTime != "" {
		if err := ValidateDatetime(req.DateTime); err != nil {
			return fmt.Errorf("invalid datetime: %w", err)
		}
	}

	// Cannot specify both bbox and intersects
	if len(req.BBox) > 0 && len(req.Intersects) > 0 {
		return fmt.Errorf("cannot specify both bbox and intersects")
	}

	// Validate limit
	if req.Limit < 0 {
		return fmt.Errorf("limit must be non-negative, got %d", req.Limit)
	}

	// Validate collections (basic check for empty strings)
	for i, coll := range req.Collections {
		if strings.TrimSpace(coll) == "" {
			return fmt.Errorf("collection at index %d cannot be empty", i)
		}
	}

	// Validate IDs (basic check for empty strings)
	for i, id := range req.IDs {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("id at index %d cannot be empty", i)
		}
	}

	return nil
}

// ValidateBBox validates a bounding box
func ValidateBBox(bbox []float64) error {
	if len(bbox) != 4 && len(bbox) != 6 {
		return fmt.Errorf("bbox must have 4 or 6 coordinates, got %d", len(bbox))
	}

	// For 2D bbox: [west, south, east, north]
	if len(bbox) == 4 {
		west, south, east, north := bbox[0], bbox[1], bbox[2], bbox[3]

		// Validate longitude bounds
		if west < -180 || west > 180 {
			return fmt.Errorf("west longitude must be between -180 and 180, got %f", west)
		}
		if east < -180 || east > 180 {
			return fmt.Errorf("east longitude must be between -180 and 180, got %f", east)
		}

		// Validate latitude bounds
		if south < -90 || south > 90 {
			return fmt.Errorf("south latitude must be between -90 and 90, got %f", south)
		}
		if north < -90 || north > 90 {
			return fmt.Errorf("north latitude must be between -90 and 90, got %f", north)
		}

		// Validate spatial relationships
		if west > east {
			return fmt.Errorf("west longitude (%f) must be less than or equal to east longitude (%f)", west, east)
		}
		if south > north {
			return fmt.Errorf("south latitude (%f) must be less than or equal to north latitude (%f)", south, north)
		}
	}

	// For 3D bbox: [west, south, min_elev, east, north, max_elev]
	if len(bbox) == 6 {
		west, south, minElev, east, north, maxElev := bbox[0], bbox[1], bbox[2], bbox[3], bbox[4], bbox[5]

		// Validate longitude bounds
		if west < -180 || west > 180 {
			return fmt.Errorf("west longitude must be between -180 and 180, got %f", west)
		}
		if east < -180 || east > 180 {
			return fmt.Errorf("east longitude must be between -180 and 180, got %f", east)
		}

		// Validate latitude bounds
		if south < -90 || south > 90 {
			return fmt.Errorf("south latitude must be between -90 and 90, got %f", south)
		}
		if north < -90 || north > 90 {
			return fmt.Errorf("north latitude must be between -90 and 90, got %f", north)
		}

		// Validate spatial relationships
		if west > east {
			return fmt.Errorf("west longitude (%f) must be less than or equal to east longitude (%f)", west, east)
		}
		if south > north {
			return fmt.Errorf("south latitude (%f) must be less than or equal to north latitude (%f)", south, north)
		}
		if minElev > maxElev {
			return fmt.Errorf("minimum elevation (%f) must be less than or equal to maximum elevation (%f)", minElev, maxElev)
		}
	}

	return nil
}

// ValidateDatetime validates a datetime string according to RFC 3339 / ISO 8601
func ValidateDatetime(dt string) error {
	if dt == "" {
		return fmt.Errorf("datetime cannot be empty")
	}

	// Handle special cases
	if dt == ".." || dt == "../.." {
		// Open interval, valid
		return nil
	}

	// Parse as interval if contains ".."
	if strings.Contains(dt, "/") {
		_, _, err := ParseDatetimeInterval(dt)
		return err
	}

	// Parse as single datetime
	if dt != ".." {
		if _, err := time.Parse(time.RFC3339, dt); err != nil {
			return fmt.Errorf("invalid datetime format, expected RFC 3339: %w", err)
		}
	}

	return nil
}

// ParseDatetimeInterval parses a datetime interval string into start and end times
// Supports formats:
// - "2023-01-01T00:00:00Z/2023-12-31T23:59:59Z" (closed interval)
// - "2023-01-01T00:00:00Z/.." (start time only)
// - "../2023-12-31T23:59:59Z" (end time only)
// - ".." or "../.." (open interval, both nil)
func ParseDatetimeInterval(dt string) (start, end *time.Time, err error) {
	if dt == "" {
		return nil, nil, fmt.Errorf("datetime interval cannot be empty")
	}

	// Handle fully open interval
	if dt == ".." || dt == "../.." {
		return nil, nil, nil
	}

	// Split on "/"
	parts := strings.Split(dt, "/")
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("invalid datetime interval format, expected 'start/end', got: %s", dt)
	}

	startStr := strings.TrimSpace(parts[0])
	endStr := strings.TrimSpace(parts[1])

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

	// Validate that start is before end if both are provided
	if start != nil && end != nil && start.After(*end) {
		return nil, nil, fmt.Errorf("start datetime (%s) must be before or equal to end datetime (%s)", start.Format(time.RFC3339), end.Format(time.RFC3339))
	}

	return start, end, nil
}
