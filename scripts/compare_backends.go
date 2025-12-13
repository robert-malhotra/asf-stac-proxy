// Script to compare ASF and CMR search results for Sentinel-1 data
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	asfBaseURL = "https://api.daac.asf.alaska.edu/services/search/param"
	cmrBaseURL = "https://cmr.earthdata.nasa.gov/search/granules.umm_json"
)

// US bounding box (continental US approximate)
var usBBox = []float64{-125.0, 24.0, -66.0, 50.0}

func main() {
	// Last year date range
	endDate := time.Now()
	startDate := endDate.AddDate(-1, 0, 0)

	fmt.Println("=== Backend Comparison: Sentinel-1 over US (Last Year) ===")
	fmt.Printf("Date range: %s to %s\n", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	fmt.Printf("Bounding box: %v\n\n", usBBox)

	// Query ASF
	fmt.Println("Querying ASF API...")
	asfCount, err := queryASF(startDate, endDate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ASF query failed: %v\n", err)
	} else {
		fmt.Printf("ASF count: %d\n\n", asfCount)
	}

	// Query CMR
	fmt.Println("Querying CMR API...")
	cmrCount, err := queryCMR(startDate, endDate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "CMR query failed: %v\n", err)
	} else {
		fmt.Printf("CMR count: %d\n\n", cmrCount)
	}

	// Compare
	fmt.Println("=== Comparison ===")
	fmt.Printf("ASF:  %d granules\n", asfCount)
	fmt.Printf("CMR:  %d granules\n", cmrCount)
	if asfCount == cmrCount {
		fmt.Println("✓ Counts match!")
	} else {
		diff := asfCount - cmrCount
		pct := float64(diff) / float64(asfCount) * 100
		fmt.Printf("✗ Difference: %d (%.2f%%)\n", diff, pct)
		fmt.Println("\nNote: Differences may occur due to:")
		fmt.Println("  - Different indexing/update times between ASF and CMR")
		fmt.Println("  - CMR short_name coverage (may need additional short names)")
		fmt.Println("  - Processing level differences")
	}
}

func queryASF(start, end time.Time) (int, error) {
	// Build WKT polygon from bbox
	wkt := fmt.Sprintf("POLYGON((%f %f,%f %f,%f %f,%f %f,%f %f))",
		usBBox[0], usBBox[1], // SW
		usBBox[2], usBBox[1], // SE
		usBBox[2], usBBox[3], // NE
		usBBox[0], usBBox[3], // NW
		usBBox[0], usBBox[1], // SW (close)
	)

	// Data processing levels (excluding METADATA_* types)
	// This matches asf_processing_levels in collections/sentinel-1.json
	dataLevels := []string{"SLC", "GRD_HD", "GRD_MD", "GRD_FD", "RAW", "OCN"}

	params := url.Values{}
	params.Set("dataset", "SENTINEL-1")
	params.Set("intersectsWith", wkt)
	params.Set("start", start.Format("2006-01-02T15:04:05Z"))
	params.Set("end", end.Format("2006-01-02T15:04:05Z"))
	params.Set("output", "count")

	// Add processing levels as comma-separated value
	params.Set("processingLevel", strings.Join(dataLevels, ","))

	reqURL := asfBaseURL + "?" + params.Encode()

	resp, err := http.Get(reqURL)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read body failed: %w", err)
	}

	var count int
	if err := json.Unmarshal(body, &count); err != nil {
		var countStr string
		if err := json.Unmarshal(body, &countStr); err != nil {
			return 0, fmt.Errorf("parse count failed: %w, body: %s", err, string(body))
		}
		fmt.Sscanf(countStr, "%d", &count)
	}

	return count, nil
}

func queryCMR(start, end time.Time) (int, error) {
	// CMR uses bounding_box format: west,south,east,north
	bbox := fmt.Sprintf("%f,%f,%f,%f", usBBox[0], usBBox[1], usBBox[2], usBBox[3])

	// Temporal format: start,end
	temporal := fmt.Sprintf("%s,%s",
		start.Format("2006-01-02T15:04:05Z"),
		end.Format("2006-01-02T15:04:05Z"),
	)

	// Query each short name and sum (CMR doesn't support OR on short_name in count)
	// Full list of Sentinel-1 data products in CMR (excluding OPERA, ARIA, metadata)
	shortNames := []string{
		// SLC
		"SENTINEL-1A_SLC", "SENTINEL-1B_SLC", "SENTINEL-1C_SLC",
		// RAW
		"SENTINEL-1A_RAW", "SENTINEL-1B_RAW", "SENTINEL-1C_RAW",
		// GRD - Dual Pol
		"SENTINEL-1A_DP_GRD_FULL", "SENTINEL-1A_DP_GRD_HIGH", "SENTINEL-1A_DP_GRD_MEDIUM",
		"SENTINEL-1B_DP_GRD_HIGH", "SENTINEL-1B_DP_GRD_MEDIUM",
		"SENTINEL-1C_DP_GRD_HIGH", "SENTINEL-1C_DP_GRD_MEDIUM",
		// GRD - Single Pol
		"SENTINEL-1A_SP_GRD_FULL", "SENTINEL-1A_SP_GRD_HIGH", "SENTINEL-1A_SP_GRD_MEDIUM",
		"SENTINEL-1B_SP_GRD_HIGH", "SENTINEL-1B_SP_GRD_MEDIUM",
		"SENTINEL-1C_SP_GRD_HIGH", "SENTINEL-1C_SP_GRD_MEDIUM",
		// OCN
		"SENTINEL-1A_OCN", "SENTINEL-1B_OCN", "SENTINEL-1C_OCN",
	}

	totalCount := 0
	for _, sn := range shortNames {
		params := url.Values{}
		params.Set("provider", "ASF")
		params.Set("short_name", sn)
		params.Set("bounding_box", bbox)
		params.Set("temporal", temporal)
		params.Set("page_size", "0") // Just get count

		reqURL := cmrBaseURL + "?" + params.Encode()

		resp, err := http.Get(reqURL)
		if err != nil {
			return 0, fmt.Errorf("request failed for %s: %w", sn, err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return 0, fmt.Errorf("status %d for %s: %s", resp.StatusCode, sn, string(body))
		}

		// Get count from CMR-Hits header
		hitsHeader := resp.Header.Get("CMR-Hits")
		var hits int
		fmt.Sscanf(hitsHeader, "%d", &hits)

		resp.Body.Close()

		fmt.Printf("  %s: %d\n", sn, hits)
		totalCount += hits
	}

	return totalCount, nil
}
