// Package cmr provides a client for NASA's Common Metadata Repository (CMR) API.
package cmr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the default CMR API base URL.
	DefaultBaseURL = "https://cmr.earthdata.nasa.gov/search"

	// DefaultProvider is the default CMR provider for ASF data.
	DefaultProvider = "ASF"

	// DefaultPageSize is the default number of results per page.
	DefaultPageSize = 250

	// MaxPageSize is the maximum page size supported by CMR.
	MaxPageSize = 2000

	// CMRSearchAfterHeader is the header used for cursor-based pagination.
	CMRSearchAfterHeader = "CMR-Search-After"
)

// Client handles communication with the CMR API.
type Client struct {
	baseURL    string
	provider   string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewClient creates a new CMR API client.
func NewClient(baseURL, provider string, timeout time.Duration) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if provider == "" {
		provider = DefaultProvider
	}

	return &Client{
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		provider: provider,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		logger: slog.Default(),
	}
}

// WithLogger sets a custom logger for the client.
func (c *Client) WithLogger(logger *slog.Logger) *Client {
	c.logger = logger
	return c
}

// SearchResult contains the results of a CMR search.
type SearchResult struct {
	Granules        []UMMGranule
	Hits            int
	SearchAfter     string // Cursor for next page
	TookMs          int
}

// Search performs a granule search against CMR.
func (c *Client) Search(ctx context.Context, params *SearchParams) (*SearchResult, error) {
	// Build the search URL
	searchURL := c.baseURL + "/granules.umm_json"

	// Build query parameters
	queryParams := params.ToURLValues()
	queryParams.Set("provider", c.provider)

	c.logger.DebugContext(ctx, "executing CMR search",
		slog.String("url", searchURL),
		slog.String("params", queryParams.Encode()),
	)

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL+"?"+queryParams.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept", "application/vnd.nasa.cmr.umm_results+json")
	req.Header.Set("User-Agent", "asf-stac-proxy/1.0")

	// Add CMR-Search-After header for pagination
	if params.SearchAfter != "" {
		req.Header.Set(CMRSearchAfterHeader, params.SearchAfter)
	}

	// Execute the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.ErrorContext(ctx, "CMR API request failed",
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("CMR API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for non-200 status codes
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.logger.ErrorContext(ctx, "CMR API returned non-200 status",
			slog.Int("status_code", resp.StatusCode),
			slog.String("response_body", string(body)),
		)
		return nil, fmt.Errorf("CMR API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var cmrResp UMMSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&cmrResp); err != nil {
		c.logger.ErrorContext(ctx, "failed to decode CMR response",
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("failed to decode CMR response: %w", err)
	}

	// Extract granules from items
	granules := make([]UMMGranule, 0, len(cmrResp.Items))
	for _, item := range cmrResp.Items {
		granules = append(granules, item.UMM)
	}

	// Get the CMR-Search-After header for pagination
	searchAfter := resp.Header.Get(CMRSearchAfterHeader)

	c.logger.DebugContext(ctx, "CMR search completed",
		slog.Int("hits", cmrResp.Hits),
		slog.Int("returned", len(granules)),
		slog.Bool("has_next", searchAfter != ""),
	)

	return &SearchResult{
		Granules:    granules,
		Hits:        cmrResp.Hits,
		SearchAfter: searchAfter,
		TookMs:      cmrResp.Took,
	}, nil
}

// GetGranule retrieves a single granule by its granule UR (unique reference).
func (c *Client) GetGranule(ctx context.Context, granuleUR string) (*UMMGranule, error) {
	c.logger.DebugContext(ctx, "fetching granule",
		slog.String("granule_ur", granuleUR),
	)

	// Search by granule_ur
	params := &SearchParams{
		GranuleUR: []string{granuleUR},
		PageSize:  1,
	}

	result, err := c.Search(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to search for granule: %w", err)
	}

	if len(result.Granules) == 0 {
		return nil, fmt.Errorf("granule not found: %s", granuleUR)
	}

	return &result.Granules[0], nil
}

// SearchParams represents parameters for CMR granule searches.
type SearchParams struct {
	// Collection identification
	ShortName  []string // Collection short names
	ConceptID  []string // Collection or granule concept IDs

	// Granule identification
	GranuleUR []string // Granule unique references (scene names)

	// Spatial filters
	BoundingBox string // west,south,east,north
	Polygon     string // lon1,lat1,lon2,lat2,...
	Point       string // lon,lat

	// Temporal filters
	Temporal string // start,end in ISO 8601 format

	// SAR-specific (via additional attributes)
	Polarization    []string
	BeamMode        []string
	FlightDirection string
	RelativeOrbit   []int
	ProcessingLevel []string

	// Pagination
	PageSize    int
	SearchAfter string // CMR-Search-After cursor

	// Sorting
	SortKey string // CMR sort key (e.g., "-start_date" for descending)
}

// ToURLValues converts SearchParams to URL query parameters.
func (p *SearchParams) ToURLValues() url.Values {
	values := url.Values{}

	// Collection identification
	for _, sn := range p.ShortName {
		values.Add("short_name", sn)
	}
	for _, cid := range p.ConceptID {
		values.Add("concept_id", cid)
	}

	// Granule identification
	for _, gur := range p.GranuleUR {
		values.Add("granule_ur", gur)
	}

	// Spatial filters
	if p.BoundingBox != "" {
		values.Set("bounding_box", p.BoundingBox)
	}
	if p.Polygon != "" {
		values.Set("polygon", p.Polygon)
	}
	if p.Point != "" {
		values.Set("point", p.Point)
	}

	// Temporal filter
	if p.Temporal != "" {
		values.Set("temporal", p.Temporal)
	}

	// Additional attributes for SAR-specific filters
	// CMR uses attribute[] parameter for additional attributes
	if len(p.Polarization) > 0 {
		for _, pol := range p.Polarization {
			values.Add("attribute[]", fmt.Sprintf("string,POLARIZATION,%s", pol))
		}
	}
	if len(p.BeamMode) > 0 {
		for _, bm := range p.BeamMode {
			values.Add("attribute[]", fmt.Sprintf("string,BEAM_MODE,%s", bm))
		}
	}
	if p.FlightDirection != "" {
		values.Add("attribute[]", fmt.Sprintf("string,ASCENDING_DESCENDING,%s", p.FlightDirection))
	}
	if len(p.RelativeOrbit) > 0 {
		for _, ro := range p.RelativeOrbit {
			values.Add("attribute[]", fmt.Sprintf("int,PATH_NUMBER,%d", ro))
		}
	}
	if len(p.ProcessingLevel) > 0 {
		for _, pl := range p.ProcessingLevel {
			values.Add("attribute[]", fmt.Sprintf("string,PROCESSING_TYPE,%s", pl))
		}
	}

	// Pagination
	if p.PageSize > 0 {
		values.Set("page_size", fmt.Sprintf("%d", p.PageSize))
	} else {
		values.Set("page_size", fmt.Sprintf("%d", DefaultPageSize))
	}

	// Sorting
	if p.SortKey != "" {
		values.Set("sort_key", p.SortKey)
	} else {
		// Default sort by start_date descending (most recent first)
		values.Set("sort_key", "-start_date")
	}

	return values
}
