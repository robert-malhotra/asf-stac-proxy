package stac

import "fmt"

// SortDirection represents the sort direction.
type SortDirection string

const (
	// SortAsc represents ascending sort order.
	SortAsc SortDirection = "asc"
	// SortDesc represents descending sort order.
	SortDesc SortDirection = "desc"
)

// MapSTACFieldToASFSort maps STAC field names to ASF sort parameters.
// ASF supports: startTime, stopTime, dataset, platform, frame, orbit
func MapSTACFieldToASFSort(stacField string) (string, error) {
	switch stacField {
	case "datetime", "start_datetime", "properties.datetime", "properties.start_datetime":
		return "startTime", nil
	case "end_datetime", "properties.end_datetime":
		return "stopTime", nil
	case "platform", "properties.platform":
		return "platform", nil
	case "collection":
		return "dataset", nil
	default:
		return "", fmt.Errorf("unsupported sort field: %s", stacField)
	}
}

// MapSTACFieldToCMRSort maps STAC field names to CMR sort keys.
// direction should be "asc" or "desc" - for desc, a "-" prefix is added.
func MapSTACFieldToCMRSort(field string, direction SortDirection) string {
	prefix := ""
	if direction == SortDesc {
		prefix = "-"
	}

	switch field {
	case "datetime", "start_datetime", "properties.datetime", "properties.start_datetime":
		return prefix + "start_date"
	case "end_datetime", "properties.end_datetime":
		return prefix + "end_date"
	case "platform", "properties.platform":
		return prefix + "platform"
	default:
		// Default to start_date for unknown fields
		return prefix + "start_date"
	}
}
