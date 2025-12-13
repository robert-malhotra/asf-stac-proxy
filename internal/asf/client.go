package asf

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// Client handles communication with the ASF Search API
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewClient creates a new ASF API client
func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
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

// WithLogger sets a custom logger for the client
func (c *Client) WithLogger(logger *slog.Logger) *Client {
	c.logger = logger
	return c
}

// Search performs a search against the ASF API
func (c *Client) Search(ctx context.Context, params SearchParams) (*ASFGeoJSONResponse, error) {
	// Build the search URL
	searchURL, err := c.buildSearchURL(params)
	if err != nil {
		return nil, fmt.Errorf("failed to build search URL: %w", err)
	}

	c.logger.DebugContext(ctx, "executing ASF search",
		slog.String("url", searchURL),
	)

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "asf-stac-proxy/1.0")

	// Execute the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.ErrorContext(ctx, "ASF API request failed",
			slog.String("error", err.Error()),
			slog.String("url", searchURL),
		)
		return nil, fmt.Errorf("ASF API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for non-200 status codes
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.logger.ErrorContext(ctx, "ASF API returned non-200 status",
			slog.Int("status_code", resp.StatusCode),
			slog.String("response_body", string(body)),
		)
		return nil, fmt.Errorf("ASF API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var result ASFGeoJSONResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.logger.ErrorContext(ctx, "failed to decode ASF response",
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("failed to decode ASF response: %w", err)
	}

	c.logger.DebugContext(ctx, "ASF search completed",
		slog.Int("feature_count", len(result.Features)),
	)

	return &result, nil
}

// GetGranule retrieves a single granule by name or fileID.
// The itemID can be either a scene name or a fileID (e.g., "sceneName-SLC").
// ASF may return multiple products per scene, so we filter by fileID if provided.
func (c *Client) GetGranule(ctx context.Context, itemID string) (*ASFFeature, error) {
	c.logger.DebugContext(ctx, "fetching granule",
		slog.String("item_id", itemID),
	)

	// Create search params - granule_list matches scene name patterns
	// Note: ASF doesn't allow maxResults with granule_list
	params := SearchParams{
		GranuleList: []string{itemID},
		Output:      "geojson",
	}

	// Execute the search
	result, err := c.Search(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to search for granule: %w", err)
	}

	// Check if we got any results
	if len(result.Features) == 0 {
		c.logger.WarnContext(ctx, "granule not found",
			slog.String("item_id", itemID),
		)
		return nil, fmt.Errorf("granule not found: %s", itemID)
	}

	// If we got exactly one result, return it
	if len(result.Features) == 1 {
		c.logger.DebugContext(ctx, "granule fetched successfully",
			slog.String("item_id", itemID),
		)
		return &result.Features[0], nil
	}

	// Multiple results - try to find exact match by fileID
	for i := range result.Features {
		if result.Features[i].Properties.FileID == itemID {
			c.logger.DebugContext(ctx, "granule fetched successfully (matched by fileID)",
				slog.String("item_id", itemID),
			)
			return &result.Features[i], nil
		}
	}

	// No exact match found, return the first result as fallback
	c.logger.DebugContext(ctx, "granule fetched (no exact fileID match, using first result)",
		slog.String("item_id", itemID),
		slog.Int("result_count", len(result.Features)),
	)
	return &result.Features[0], nil
}

// buildSearchURL constructs the full search URL with query parameters
func (c *Client) buildSearchURL(params SearchParams) (string, error) {
	// Parse the base URL
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	// Add the search path
	base.Path = "/services/search/param"

	// Add query parameters
	base.RawQuery = params.ToQueryString()

	return base.String(), nil
}
