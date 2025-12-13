package stac

import (
	"fmt"
	"net/url"
	"testing"
	"time"
)

func TestBuildCursorPaginationLinks_AccumulatesSeenIDs(t *testing.T) {
	// Test scenario: 300 items all have the same timestamp, page size is 100
	// This requires 3 pages to retrieve all items, and SeenIDs must accumulate

	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	// Page 1: First 100 items at baseTime
	page1Items := make([]ItemTimeInfo, 100)
	for i := 0; i < 100; i++ {
		page1Items[i] = ItemTimeInfo{
			ID:        fmt.Sprintf("item-%03d", i),
			StartTime: baseTime,
		}
	}

	info1 := CursorPaginationInfo{
		BaseURL:       "http://example.com/items",
		Limit:         100,
		ReturnedCount: 100,
		QueryParams:   url.Values{},
		Items:         page1Items,
		CurrentCursor: nil, // First page, no cursor
	}

	links1 := BuildCursorPaginationLinks(info1)
	if len(links1) != 1 {
		t.Fatalf("Expected 1 link (next), got %d", len(links1))
	}

	// Decode cursor from page 1
	cursor1, err := decodeCursorFromURL(links1[0].Href)
	if err != nil {
		t.Fatalf("Failed to decode cursor from page 1: %v", err)
	}

	if len(cursor1.SeenIDs) != 100 {
		t.Errorf("Page 1 cursor should have 100 SeenIDs, got %d", len(cursor1.SeenIDs))
	}

	// Page 2: Next 100 items at same baseTime (simulating filtered results)
	page2Items := make([]ItemTimeInfo, 100)
	for i := 0; i < 100; i++ {
		page2Items[i] = ItemTimeInfo{
			ID:        fmt.Sprintf("item-%03d", i+100), // items 100-199
			StartTime: baseTime,
		}
	}

	info2 := CursorPaginationInfo{
		BaseURL:       "http://example.com/items",
		Limit:         100,
		ReturnedCount: 100,
		QueryParams:   url.Values{},
		Items:         page2Items,
		CurrentCursor: cursor1, // Pass cursor from page 1
	}

	links2 := BuildCursorPaginationLinks(info2)
	if len(links2) != 1 {
		t.Fatalf("Expected 1 link (next), got %d", len(links2))
	}

	// Decode cursor from page 2
	cursor2, err := decodeCursorFromURL(links2[0].Href)
	if err != nil {
		t.Fatalf("Failed to decode cursor from page 2: %v", err)
	}

	// CRITICAL: Cursor 2 should have accumulated 200 SeenIDs (100 from page 1 + 100 from page 2)
	if len(cursor2.SeenIDs) != 200 {
		t.Errorf("Page 2 cursor should have 200 SeenIDs (accumulated), got %d", len(cursor2.SeenIDs))
	}

	// Page 3: Last 100 items at same baseTime
	page3Items := make([]ItemTimeInfo, 100)
	for i := 0; i < 100; i++ {
		page3Items[i] = ItemTimeInfo{
			ID:        fmt.Sprintf("item-%03d", i+200), // items 200-299
			StartTime: baseTime,
		}
	}

	info3 := CursorPaginationInfo{
		BaseURL:       "http://example.com/items",
		Limit:         100,
		ReturnedCount: 100,
		QueryParams:   url.Values{},
		Items:         page3Items,
		CurrentCursor: cursor2, // Pass cursor from page 2
	}

	links3 := BuildCursorPaginationLinks(info3)
	if len(links3) != 1 {
		t.Fatalf("Expected 1 link (next), got %d", len(links3))
	}

	// Decode cursor from page 3
	cursor3, err := decodeCursorFromURL(links3[0].Href)
	if err != nil {
		t.Fatalf("Failed to decode cursor from page 3: %v", err)
	}

	// Cursor 3 should have 300 SeenIDs
	if len(cursor3.SeenIDs) != 300 {
		t.Errorf("Page 3 cursor should have 300 SeenIDs (accumulated), got %d", len(cursor3.SeenIDs))
	}
}

func TestBuildCursorPaginationLinks_ResetsSeenIDsOnNewTimestamp(t *testing.T) {
	// Test that SeenIDs are NOT accumulated when we move to a new timestamp

	time1 := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	time2 := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC) // Earlier timestamp

	// Page 1: 50 items at time1
	page1Items := make([]ItemTimeInfo, 50)
	for i := 0; i < 50; i++ {
		page1Items[i] = ItemTimeInfo{
			ID:        fmt.Sprintf("item-t1-%03d", i),
			StartTime: time1,
		}
	}

	info1 := CursorPaginationInfo{
		BaseURL:       "http://example.com/items",
		Limit:         50,
		ReturnedCount: 50,
		QueryParams:   url.Values{},
		Items:         page1Items,
		CurrentCursor: nil,
	}

	links1 := BuildCursorPaginationLinks(info1)
	cursor1, _ := decodeCursorFromURL(links1[0].Href)

	if len(cursor1.SeenIDs) != 50 {
		t.Errorf("Page 1 cursor should have 50 SeenIDs, got %d", len(cursor1.SeenIDs))
	}

	// Page 2: 50 items at time2 (different, earlier timestamp)
	page2Items := make([]ItemTimeInfo, 50)
	for i := 0; i < 50; i++ {
		page2Items[i] = ItemTimeInfo{
			ID:        fmt.Sprintf("item-t2-%03d", i),
			StartTime: time2,
		}
	}

	info2 := CursorPaginationInfo{
		BaseURL:       "http://example.com/items",
		Limit:         50,
		ReturnedCount: 50,
		QueryParams:   url.Values{},
		Items:         page2Items,
		CurrentCursor: cursor1,
	}

	links2 := BuildCursorPaginationLinks(info2)
	cursor2, _ := decodeCursorFromURL(links2[0].Href)

	// Since timestamp changed, SeenIDs should only contain the 50 items from page 2
	// NOT accumulated with page 1's 50 items
	if len(cursor2.SeenIDs) != 50 {
		t.Errorf("Page 2 cursor should have 50 SeenIDs (not accumulated since timestamp changed), got %d", len(cursor2.SeenIDs))
	}

	// Verify the timestamp is time2
	if cursor2.StartTime != time2.Format(time.RFC3339) {
		t.Errorf("Page 2 cursor should have timestamp %s, got %s", time2.Format(time.RFC3339), cursor2.StartTime)
	}
}

func TestFilterSeenItems(t *testing.T) {
	items := []struct {
		id   string
		data string
	}{
		{"item-001", "data1"},
		{"item-002", "data2"},
		{"item-003", "data3"},
		{"item-004", "data4"},
	}

	cursor := &Cursor{
		SeenIDs: []string{"item-002", "item-004"},
	}

	filtered := FilterSeenItems(items, func(item struct{ id, data string }) string {
		return item.id
	}, cursor)

	if len(filtered) != 2 {
		t.Errorf("Expected 2 items after filtering, got %d", len(filtered))
	}

	// Check that correct items remain
	expectedIDs := map[string]bool{"item-001": true, "item-003": true}
	for _, item := range filtered {
		if !expectedIDs[item.id] {
			t.Errorf("Unexpected item in filtered results: %s", item.id)
		}
	}
}

// Helper function to decode cursor from URL
func decodeCursorFromURL(urlStr string) (*Cursor, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	cursorParam := u.Query().Get("cursor")
	return DecodeCursor(cursorParam)
}

func TestMemoryCursorStore(t *testing.T) {
	store := NewMemoryCursorStore(100*time.Millisecond, 50*time.Millisecond)
	defer store.Stop()

	cursor := &Cursor{
		StartTime: "2024-01-15T12:00:00Z",
		Direction: "next",
		SeenIDs:   []string{"item-001", "item-002", "item-003"},
	}

	// Store cursor
	token, err := store.Store(cursor)
	if err != nil {
		t.Fatalf("Failed to store cursor: %v", err)
	}
	if token == "" {
		t.Fatal("Token should not be empty")
	}

	// Retrieve cursor
	retrieved, err := store.Retrieve(token)
	if err != nil {
		t.Fatalf("Failed to retrieve cursor: %v", err)
	}
	if retrieved.StartTime != cursor.StartTime {
		t.Errorf("StartTime mismatch: got %s, want %s", retrieved.StartTime, cursor.StartTime)
	}
	if len(retrieved.SeenIDs) != len(cursor.SeenIDs) {
		t.Errorf("SeenIDs length mismatch: got %d, want %d", len(retrieved.SeenIDs), len(cursor.SeenIDs))
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired or deleted by cleanup
	_, err = store.Retrieve(token)
	if err != ErrCursorExpired && err != ErrCursorNotFound {
		t.Errorf("Expected ErrCursorExpired or ErrCursorNotFound, got %v", err)
	}
}

func TestEncodeCursorWithStore_SmallCursor(t *testing.T) {
	store := NewMemoryCursorStore(1*time.Hour, 5*time.Minute)
	defer store.Stop()

	// Small cursor - should be encoded inline
	cursor := &Cursor{
		StartTime: "2024-01-15T12:00:00Z",
		Direction: "next",
		SeenIDs:   []string{"item-001"},
	}

	encoded, err := EncodeCursorWithStore(cursor, store)
	if err != nil {
		t.Fatalf("Failed to encode cursor: %v", err)
	}

	// Should NOT be a server-side reference
	if IsServerSideCursor(encoded) {
		t.Error("Small cursor should be encoded inline, not stored server-side")
	}

	// Should be decodable
	decoded, err := DecodeCursorWithStore(encoded, store)
	if err != nil {
		t.Fatalf("Failed to decode cursor: %v", err)
	}
	if decoded.StartTime != cursor.StartTime {
		t.Errorf("StartTime mismatch: got %s, want %s", decoded.StartTime, cursor.StartTime)
	}
}

func TestEncodeCursorWithStore_LargeCursor(t *testing.T) {
	store := NewMemoryCursorStore(1*time.Hour, 5*time.Minute)
	defer store.Stop()

	// Create a large cursor that exceeds MaxInlineCursorSize
	// Each ID is ~80 chars, need ~30+ to exceed 2KB
	seenIDs := make([]string, 50)
	for i := 0; i < 50; i++ {
		seenIDs[i] = fmt.Sprintf("S1A_IW_GRDH_1SDV_20241130T171452_20241130T171517_%06d_06F888_B167-GRD_HD", i)
	}

	cursor := &Cursor{
		StartTime: "2024-01-15T12:00:00Z",
		Direction: "next",
		SeenIDs:   seenIDs,
	}

	encoded, err := EncodeCursorWithStore(cursor, store)
	if err != nil {
		t.Fatalf("Failed to encode cursor: %v", err)
	}

	// Should be a server-side reference
	if !IsServerSideCursor(encoded) {
		t.Error("Large cursor should be stored server-side")
	}

	// Should be decodable via store
	decoded, err := DecodeCursorWithStore(encoded, store)
	if err != nil {
		t.Fatalf("Failed to decode cursor: %v", err)
	}
	if decoded.StartTime != cursor.StartTime {
		t.Errorf("StartTime mismatch: got %s, want %s", decoded.StartTime, cursor.StartTime)
	}
	if len(decoded.SeenIDs) != len(cursor.SeenIDs) {
		t.Errorf("SeenIDs length mismatch: got %d, want %d", len(decoded.SeenIDs), len(cursor.SeenIDs))
	}
}

func TestBuildCursorPaginationLinks_WithStore(t *testing.T) {
	store := NewMemoryCursorStore(1*time.Hour, 5*time.Minute)
	defer store.Stop()

	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create items with many boundary IDs (simulating large cursor scenario)
	items := make([]ItemTimeInfo, 50)
	for i := 0; i < 50; i++ {
		items[i] = ItemTimeInfo{
			ID:        fmt.Sprintf("S1A_IW_GRDH_1SDV_20241130T171452_20241130T171517_%06d_06F888_B167-GRD_HD", i),
			StartTime: baseTime,
		}
	}

	info := CursorPaginationInfo{
		BaseURL:       "http://example.com/items",
		Limit:         50,
		ReturnedCount: 50,
		QueryParams:   url.Values{},
		Items:         items,
		CurrentCursor: nil,
		CursorStore:   store,
	}

	links := BuildCursorPaginationLinks(info)
	if len(links) != 1 {
		t.Fatalf("Expected 1 link (next), got %d", len(links))
	}

	// Parse the cursor from the URL
	u, _ := url.Parse(links[0].Href)
	cursorParam := u.Query().Get("cursor")

	// Should be server-side reference due to large size
	if !IsServerSideCursor(cursorParam) {
		t.Log("Cursor:", cursorParam[:100], "...")
		t.Error("Large cursor should use server-side storage")
	}

	// Should be retrievable
	decoded, err := DecodeCursorWithStore(cursorParam, store)
	if err != nil {
		t.Fatalf("Failed to decode cursor: %v", err)
	}
	if len(decoded.SeenIDs) != 50 {
		t.Errorf("Expected 50 SeenIDs, got %d", len(decoded.SeenIDs))
	}
}

func TestBuildCursorPaginationLinks_BackendHasMoreData(t *testing.T) {
	// Test that BackendHasMoreData=true generates "next" link even when ReturnedCount < Limit
	// This is the key fix for pagination when filtering SeenIDs reduces the returned count

	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	// Scenario: Backend returned 100 items, but after filtering SeenIDs we only have 80
	// With BackendHasMoreData=true, we should still get a "next" link
	items := make([]ItemTimeInfo, 80)
	for i := 0; i < 80; i++ {
		items[i] = ItemTimeInfo{
			ID:        fmt.Sprintf("item-%03d", i),
			StartTime: baseTime,
		}
	}

	info := CursorPaginationInfo{
		BaseURL:            "http://example.com/items",
		Limit:              100,
		ReturnedCount:      80, // Less than limit due to filtering
		BackendHasMoreData: true,
		QueryParams:        url.Values{},
		Items:              items,
		CurrentCursor:      nil,
	}

	links := BuildCursorPaginationLinks(info)

	// Should still have "next" link because BackendHasMoreData=true
	if len(links) != 1 {
		t.Fatalf("Expected 1 link (next) with BackendHasMoreData=true, got %d", len(links))
	}
	if links[0].Rel != "next" {
		t.Errorf("Expected 'next' link, got '%s'", links[0].Rel)
	}
}

func TestBuildCursorPaginationLinks_NoMoreDataWhenBothFalse(t *testing.T) {
	// Test that no "next" link is generated when both BackendHasMoreData=false
	// and ReturnedCount < Limit (truly the last page)

	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	// Scenario: Backend returned only 50 items (less than our limit of 100)
	// This means we've reached the end of the data
	items := make([]ItemTimeInfo, 50)
	for i := 0; i < 50; i++ {
		items[i] = ItemTimeInfo{
			ID:        fmt.Sprintf("item-%03d", i),
			StartTime: baseTime,
		}
	}

	info := CursorPaginationInfo{
		BaseURL:            "http://example.com/items",
		Limit:              100,
		ReturnedCount:      50,
		BackendHasMoreData: false, // Backend returned less than requested
		QueryParams:        url.Values{},
		Items:              items,
		CurrentCursor:      nil,
	}

	links := BuildCursorPaginationLinks(info)

	// Should NOT have "next" link
	if len(links) != 0 {
		t.Errorf("Expected no links when truly at last page, got %d links", len(links))
	}
}

func TestBuildCursorPaginationLinks_BackendHasMoreDataWithSeenIDs(t *testing.T) {
	// Test realistic scenario: paginating through items where many share same timestamp
	// Backend over-fetches, we filter SeenIDs, and still need next link

	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	// Simulate: limit=100, backend returned 120 (over-fetch), after filtering we have 95
	// Previous cursor had 20 SeenIDs, so we over-fetched by 20
	previousCursor := &Cursor{
		StartTime: baseTime.Format(time.RFC3339),
		Direction: "next",
		SeenIDs:   make([]string, 20),
	}
	for i := 0; i < 20; i++ {
		previousCursor.SeenIDs[i] = fmt.Sprintf("prev-item-%03d", i)
	}

	// Current page items (after filtering out the 20 SeenIDs)
	items := make([]ItemTimeInfo, 95)
	for i := 0; i < 95; i++ {
		items[i] = ItemTimeInfo{
			ID:        fmt.Sprintf("item-%03d", i+20), // IDs 20-114
			StartTime: baseTime,
		}
	}

	info := CursorPaginationInfo{
		BaseURL:            "http://example.com/items",
		Limit:              100,
		ReturnedCount:      95,                // After filtering, less than limit
		BackendHasMoreData: true,              // Backend returned 120 items (full over-fetch)
		QueryParams:        url.Values{},
		Items:              items,
		CurrentCursor:      previousCursor,
	}

	links := BuildCursorPaginationLinks(info)

	// Should have "next" link because backend had more data
	if len(links) != 1 {
		t.Fatalf("Expected 1 link (next), got %d", len(links))
	}

	// Decode the cursor and verify SeenIDs are accumulated
	cursor, err := decodeCursorFromURL(links[0].Href)
	if err != nil {
		t.Fatalf("Failed to decode cursor: %v", err)
	}

	// Should accumulate: 20 from previous + 95 from current = 115 SeenIDs
	// (all at same timestamp)
	if len(cursor.SeenIDs) != 115 {
		t.Errorf("Expected 115 accumulated SeenIDs, got %d", len(cursor.SeenIDs))
	}
}

func TestBuildCursorPaginationLinks_LastPageAfterFiltering(t *testing.T) {
	// Test edge case: after filtering, we have fewer items than limit,
	// but backend also returned fewer items (truly last page)

	baseTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	// Simulate: limit=100, over-fetch requested 120, but backend only had 80 total
	previousCursor := &Cursor{
		StartTime: baseTime.Format(time.RFC3339),
		Direction: "next",
		SeenIDs:   make([]string, 20),
	}
	for i := 0; i < 20; i++ {
		previousCursor.SeenIDs[i] = fmt.Sprintf("prev-item-%03d", i)
	}

	// After filtering the 20 SeenIDs, we only have 60 unique items
	items := make([]ItemTimeInfo, 60)
	for i := 0; i < 60; i++ {
		items[i] = ItemTimeInfo{
			ID:        fmt.Sprintf("item-%03d", i+20),
			StartTime: baseTime,
		}
	}

	info := CursorPaginationInfo{
		BaseURL:            "http://example.com/items",
		Limit:              100,
		ReturnedCount:      60,
		BackendHasMoreData: false, // Backend returned less than the over-fetch amount
		QueryParams:        url.Values{},
		Items:              items,
		CurrentCursor:      previousCursor,
	}

	links := BuildCursorPaginationLinks(info)

	// Should NOT have "next" link - this is truly the last page
	if len(links) != 0 {
		t.Errorf("Expected no links on last page, got %d", len(links))
	}
}
