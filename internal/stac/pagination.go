package stac

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// MaxInlineCursorSize is the maximum size (in bytes) for inline base64-encoded cursors.
// Cursors larger than this will be stored server-side and referenced by token.
// 2KB is a safe limit that works with all browsers and servers.
const MaxInlineCursorSize = 2048

// ServerSideCursorPrefix identifies cursor tokens that reference server-side storage.
const ServerSideCursorPrefix = "ref:"

// Cursor represents pagination cursor data
type Cursor struct {
	// StartTime is the startTime of the last item in the current page
	// Used to fetch items with startTime <= this value for the next page
	// (ASF API filters by start time with the 'end' parameter)
	StartTime string `json:"st"`
	// Direction: "next" for forward pagination, "prev" for backward
	Direction string `json:"d"`
	// SeenIDs contains IDs of items at the boundary timestamp that have already been returned.
	// This prevents duplicates when multiple items share the same start_datetime.
	SeenIDs []string `json:"seen,omitempty"`
}

// EncodeCursor encodes a cursor to a URL-safe string.
// If a CursorStore is provided and the cursor is large, it will be stored server-side.
// Returns an empty string if the cursor is nil or encoding fails.
func EncodeCursor(cursor *Cursor) string {
	if cursor == nil {
		return ""
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		// This should never happen with a simple struct, but handle gracefully
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}

// EncodeCursorWithStore encodes a cursor, using server-side storage if the cursor is too large.
// Returns the encoded cursor string (either base64 or "ref:<token>").
func EncodeCursorWithStore(cursor *Cursor, store CursorStore) (string, error) {
	if cursor == nil {
		return "", nil
	}

	// First, try inline encoding
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("failed to marshal cursor: %w", err)
	}
	encoded := base64.URLEncoding.EncodeToString(data)

	// If small enough, use inline encoding
	if len(encoded) <= MaxInlineCursorSize {
		return encoded, nil
	}

	// Cursor is too large - store server-side
	if store == nil {
		// No store available, fall back to inline (may cause issues with very long URLs)
		return encoded, nil
	}

	token, err := store.Store(cursor)
	if err != nil {
		// Fall back to inline on store error
		return encoded, nil
	}

	return ServerSideCursorPrefix + token, nil
}

// DecodeCursor decodes a cursor from a URL-safe string
func DecodeCursor(encoded string) (*Cursor, error) {
	if encoded == "" {
		return nil, nil
	}
	data, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	var cursor Cursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, err
	}
	return &cursor, nil
}

// DecodeCursorWithStore decodes a cursor, retrieving from server-side storage if needed.
func DecodeCursorWithStore(encoded string, store CursorStore) (*Cursor, error) {
	if encoded == "" {
		return nil, nil
	}

	// Check if this is a server-side cursor reference
	if strings.HasPrefix(encoded, ServerSideCursorPrefix) {
		if store == nil {
			return nil, ErrCursorNotFound
		}
		token := strings.TrimPrefix(encoded, ServerSideCursorPrefix)
		return store.Retrieve(token)
	}

	// Inline cursor - decode normally
	return DecodeCursor(encoded)
}

// IsServerSideCursor returns true if the cursor string references server-side storage.
func IsServerSideCursor(encoded string) bool {
	return strings.HasPrefix(encoded, ServerSideCursorPrefix)
}

// ItemTimeInfo contains item ID and timestamp for pagination
type ItemTimeInfo struct {
	ID        string
	StartTime time.Time
}

// CursorPaginationInfo holds information needed for cursor-based pagination
type CursorPaginationInfo struct {
	BaseURL       string
	Limit         int
	ReturnedCount int
	QueryParams   url.Values // Original query parameters (without cursor)
	// Items contains ID and start_datetime of each item in the results
	// Used to generate the cursor with boundary IDs for deduplication
	Items []ItemTimeInfo
	// CurrentCursor is the cursor used for the current request (nil if first page)
	CurrentCursor *Cursor
	// CursorStore is used for server-side cursor storage when cursors are too large.
	// If nil, cursors are always encoded inline (may cause URL length issues).
	CursorStore CursorStore
	// BackendHasMoreData indicates the backend returned a full page of results,
	// suggesting more data exists. Used for pagination decisions after filtering.
	BackendHasMoreData bool
}

// BuildCursorPaginationLinks generates next/prev links using cursor-based pagination
func BuildCursorPaginationLinks(info CursorPaginationInfo) []*Link {
	links := make([]*Link, 0, 2)

	// Generate next link if:
	// 1. BackendHasMoreData is true (backend returned a full page), OR
	// 2. We returned a full page after filtering (fallback for backward compatibility)
	hasMoreData := info.BackendHasMoreData || info.ReturnedCount >= info.Limit
	if hasMoreData && len(info.Items) > 0 {
		// Find the MINIMUM startTime from all items on this page
		// This is important because ASF's ordering may not be strictly by startTime
		minTime := info.Items[0].StartTime
		for _, item := range info.Items {
			if item.StartTime.Before(minTime) {
				minTime = item.StartTime
			}
		}

		// Collect all item IDs at the minimum timestamp (items with the same start_datetime)
		// These need to be tracked in the cursor to avoid returning them again
		var boundaryIDs []string
		for _, item := range info.Items {
			if item.StartTime.Equal(minTime) {
				boundaryIDs = append(boundaryIDs, item.ID)
			}
		}

		// IMPORTANT: If the cursor timestamp hasn't changed, we need to ACCUMULATE
		// the SeenIDs from the previous cursor. This handles the case where more items
		// share the same timestamp than fit on a single page.
		// Example: 300 items at T1, page size 250
		//   - Page 1: returns 250, cursor has seen=[250 IDs], st=T1
		//   - Page 2: returns 50 (filtered from 300), must keep all 300 IDs in cursor
		if info.CurrentCursor != nil && info.CurrentCursor.StartTime != "" {
			prevCursorTime, err := time.Parse(time.RFC3339, info.CurrentCursor.StartTime)
			if err == nil && prevCursorTime.Equal(minTime) {
				// Same timestamp - accumulate SeenIDs from previous cursor
				// Use a map to deduplicate
				seenSet := make(map[string]bool)
				for _, id := range info.CurrentCursor.SeenIDs {
					seenSet[id] = true
				}
				for _, id := range boundaryIDs {
					seenSet[id] = true
				}
				// Rebuild boundaryIDs with all accumulated IDs
				boundaryIDs = make([]string, 0, len(seenSet))
				for id := range seenSet {
					boundaryIDs = append(boundaryIDs, id)
				}
			}
		}

		cursor := &Cursor{
			StartTime: minTime.Format(time.RFC3339),
			Direction: "next",
			SeenIDs:   boundaryIDs,
		}

		// Build URL with cursor (may use server-side storage for large cursors)
		nextURL := buildCursorURLWithStore(info.BaseURL, info.QueryParams, cursor, info.Limit, info.CursorStore)
		links = append(links, &Link{
			Rel:  "next",
			Href: nextURL,
			Type: "application/geo+json",
		})
	}

	// Note: "prev" links are more complex with cursor-based pagination
	// and require storing more state. For simplicity, we only support forward pagination.
	// Users can restart from the beginning by removing the cursor parameter.

	return links
}

// buildCursorURL constructs a URL with the cursor parameter (always inline).
func buildCursorURL(baseURL string, params url.Values, cursor *Cursor, limit int) string {
	return buildCursorURLWithStore(baseURL, params, cursor, limit, nil)
}

// buildCursorURLWithStore constructs a URL with the cursor parameter.
// If the cursor is large and a store is provided, it will use server-side storage.
func buildCursorURLWithStore(baseURL string, params url.Values, cursor *Cursor, limit int, store CursorStore) string {
	// Clone the params to avoid modifying the original
	newParams := url.Values{}
	for key, values := range params {
		// Skip cursor-related params from original request
		if key == "cursor" || key == "page" {
			continue
		}
		for _, value := range values {
			newParams.Add(key, value)
		}
	}

	// Add the cursor (may use server-side storage for large cursors)
	if cursor != nil {
		encoded, err := EncodeCursorWithStore(cursor, store)
		if err == nil && encoded != "" {
			newParams.Set("cursor", encoded)
		}
	}

	// Ensure limit is set
	if limit > 0 {
		newParams.Set("limit", strconv.Itoa(limit))
	}

	// Build the URL
	if len(newParams) > 0 {
		return baseURL + "?" + newParams.Encode()
	}
	return baseURL
}

// ApplyCursorToDatetime modifies the datetime range based on the cursor
// Returns the new end time constraint for the query
// Note: ASF API's 'end' parameter filters by start time (acquisition start) as startTime < end
func ApplyCursorToDatetime(cursor *Cursor, existingEnd *time.Time) *time.Time {
	if cursor == nil || cursor.StartTime == "" {
		return existingEnd
	}

	cursorTime, err := time.Parse(time.RFC3339, cursor.StartTime)
	if err != nil {
		return existingEnd
	}

	// For "next" direction, we want items with startTime <= cursor time
	// ASF's 'end' parameter is exclusive (startTime < end), so add 1 second
	// to include items at the cursor timestamp. SeenIDs will filter duplicates.
	cursorTime = cursorTime.Add(time.Second)

	// Use the earlier of existingEnd and cursorTime
	if existingEnd == nil {
		return &cursorTime
	}
	if cursorTime.Before(*existingEnd) {
		return &cursorTime
	}
	return existingEnd
}

// FilterSeenItems removes items that are in the cursor's SeenIDs list
// This is called after fetching results to eliminate duplicates from the previous page
func FilterSeenItems[T any](items []T, getID func(T) string, cursor *Cursor) []T {
	if cursor == nil || len(cursor.SeenIDs) == 0 {
		return items
	}

	// Build a set of seen IDs for fast lookup
	seenSet := make(map[string]bool, len(cursor.SeenIDs))
	for _, id := range cursor.SeenIDs {
		seenSet[id] = true
	}

	// Filter out seen items
	result := make([]T, 0, len(items))
	for _, item := range items {
		if !seenSet[getID(item)] {
			result = append(result, item)
		}
	}

	return result
}

// Legacy page-based pagination (kept for compatibility)

// PaginationInfo holds information needed to generate pagination links
type PaginationInfo struct {
	BaseURL       string
	CurrentPage   int
	Limit         int
	TotalCount    *int // nil if unknown
	ReturnedCount int
	QueryParams   url.Values // Original query parameters
}

// BuildPaginationLinks generates next and prev links based on pagination info
// Deprecated: Use BuildCursorPaginationLinks for new implementations
func BuildPaginationLinks(info PaginationInfo) []*Link {
	links := make([]*Link, 0, 2)

	// Generate previous link if not on first page
	if info.CurrentPage > 1 {
		prevURL := buildPageURL(info.BaseURL, info.QueryParams, info.CurrentPage-1)
		links = append(links, &Link{
			Rel:  "prev",
			Href: prevURL,
			Type: "application/geo+json",
		})
	}

	// Generate next link if:
	// 1. We returned the full limit (suggests more results may exist), OR
	// 2. We know the total count and can calculate there are more pages
	hasNextPage := false

	if info.TotalCount != nil {
		// We know the total count, so we can calculate if there's a next page
		totalPages := (*info.TotalCount + info.Limit - 1) / info.Limit // Ceiling division
		hasNextPage = info.CurrentPage < totalPages
	} else {
		// We don't know the total count, so we guess based on returned results
		// If we got a full page, there might be more
		hasNextPage = info.ReturnedCount >= info.Limit
	}

	if hasNextPage {
		nextURL := buildPageURL(info.BaseURL, info.QueryParams, info.CurrentPage+1)
		links = append(links, &Link{
			Rel:  "next",
			Href: nextURL,
			Type: "application/geo+json",
		})
	}

	return links
}

// buildPageURL constructs a URL with the given page number
func buildPageURL(baseURL string, params url.Values, page int) string {
	// Clone the params to avoid modifying the original
	newParams := url.Values{}
	for key, values := range params {
		for _, value := range values {
			newParams.Add(key, value)
		}
	}

	// Set the page parameter
	newParams.Set("page", strconv.Itoa(page))

	// Build the URL
	if len(newParams) > 0 {
		return baseURL + "?" + newParams.Encode()
	}
	return baseURL
}
