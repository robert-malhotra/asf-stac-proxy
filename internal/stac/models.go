// Package stac provides STAC API types and utilities, wrapping planetlabs/go-stac
// for core types and adding API-specific types.
package stac

import (
	gostac "github.com/planetlabs/go-stac"
)

// Re-export core types from planetlabs/go-stac for convenience
type (
	Item       = gostac.Item
	Collection = gostac.Collection
	Catalog    = gostac.Catalog
	Asset      = gostac.Asset
	Link       = gostac.Link
	Provider   = gostac.Provider
	Extent     = gostac.Extent
)

// ItemCollection represents a STAC ItemCollection (GeoJSON FeatureCollection)
// This extends the standard FeatureCollection with STAC-specific pagination fields.
type ItemCollection struct {
	Type           string          `json:"type"` // "FeatureCollection"
	Features       []*gostac.Item  `json:"features"`
	Links          []*gostac.Link  `json:"links"`
	NumberMatched  *int            `json:"numberMatched,omitempty"`
	NumberReturned int             `json:"numberReturned"`
	Context        *Context        `json:"context,omitempty"`
}

// Context provides additional metadata about the response (STAC Context extension)
type Context struct {
	Returned int  `json:"returned"`
	Limit    int  `json:"limit,omitempty"`
	Matched  *int `json:"matched,omitempty"`
}

// NewItemCollection creates a new ItemCollection with the given items.
func NewItemCollection(items []*gostac.Item) *ItemCollection {
	return &ItemCollection{
		Type:           "FeatureCollection",
		Features:       items,
		Links:          make([]*gostac.Link, 0),
		NumberReturned: len(items),
	}
}

// AddLink adds a link to the ItemCollection.
func (ic *ItemCollection) AddLink(rel, href, mediaType string) {
	ic.Links = append(ic.Links, &gostac.Link{
		Rel:  rel,
		Href: href,
		Type: mediaType,
	})
}

// SetContext sets the context metadata for the ItemCollection.
func (ic *ItemCollection) SetContext(returned, limit int, matched *int) {
	ic.Context = &Context{
		Returned: returned,
		Limit:    limit,
		Matched:  matched,
	}
	if matched != nil {
		ic.NumberMatched = matched
	}
}

// NewItem creates a new STAC Item with the given ID and collection.
func NewItem(id, collection, version string) *gostac.Item {
	return &gostac.Item{
		Version:    version,
		Id:         id,
		Collection: collection,
		Properties: make(map[string]any),
		Assets:     make(map[string]*gostac.Asset),
		Links:      make([]*gostac.Link, 0),
	}
}

// NewCollection creates a new STAC Collection with the given ID.
func NewCollection(id, title, description, version string) *gostac.Collection {
	return &gostac.Collection{
		Version:     version,
		Id:          id,
		Title:       title,
		Description: description,
		Links:       make([]*gostac.Link, 0),
		Assets:      make(map[string]*gostac.Asset),
		Summaries:   make(map[string]any),
	}
}

// NewCatalog creates a new STAC Catalog for the landing page.
func NewCatalog(id, title, description, version string) *gostac.Catalog {
	return &gostac.Catalog{
		Version:     version,
		Id:          id,
		Title:       title,
		Description: description,
		Links:       make([]*gostac.Link, 0),
	}
}

// CollectionsList represents a list of collections response.
type CollectionsList struct {
	Collections []*gostac.Collection `json:"collections"`
	Links       []*gostac.Link       `json:"links"`
}

// NewCollectionsList creates a new CollectionsList.
func NewCollectionsList(collections []*gostac.Collection) *CollectionsList {
	return &CollectionsList{
		Collections: collections,
		Links:       make([]*gostac.Link, 0),
	}
}

// Conformance represents the conformance classes response.
type Conformance struct {
	ConformsTo []string `json:"conformsTo"`
}

// LandingPage represents the STAC API landing page response.
type LandingPage struct {
	Type        string         `json:"type"` // "Catalog"
	Id          string         `json:"id"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	StacVersion string         `json:"stac_version"`
	ConformsTo  []string       `json:"conformsTo,omitempty"`
	Links       []*gostac.Link `json:"links"`
}

// NewLandingPage creates a new landing page response.
func NewLandingPage(id, title, description, version string, conformsTo []string) *LandingPage {
	return &LandingPage{
		Type:        "Catalog",
		Id:          id,
		Title:       title,
		Description: description,
		StacVersion: version,
		ConformsTo:  conformsTo,
		Links:       make([]*gostac.Link, 0),
	}
}

// AddLink adds a link to the landing page.
func (lp *LandingPage) AddLink(rel, href, mediaType string) {
	lp.Links = append(lp.Links, &gostac.Link{
		Rel:  rel,
		Href: href,
		Type: mediaType,
	})
}

// Standard STAC conformance URIs
const (
	ConformanceCore          = "https://api.stacspec.org/v1.0.0/core"
	ConformanceOGCFeatures   = "https://api.stacspec.org/v1.0.0/ogcapi-features"
	ConformanceItemSearch    = "https://api.stacspec.org/v1.0.0/item-search"
	ConformanceFilter        = "https://api.stacspec.org/v1.0.0/item-search#filter"
	ConformanceOGCFeatCore   = "http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/core"
	ConformanceOGCFeatGeoJSON = "http://www.opengis.net/spec/ogcapi-features-1/1.0/conf/geojson"
)

// DefaultConformance returns the default conformance classes for the proxy.
func DefaultConformance() []string {
	return []string{
		ConformanceCore,
		ConformanceOGCFeatures,
		ConformanceItemSearch,
		ConformanceOGCFeatCore,
		ConformanceOGCFeatGeoJSON,
	}
}

// STAC extension URIs
const (
	ExtensionSAR        = "https://stac-extensions.github.io/sar/v1.0.0/schema.json"
	ExtensionSat        = "https://stac-extensions.github.io/sat/v1.0.0/schema.json"
	ExtensionProcessing = "https://stac-extensions.github.io/processing/v1.0.0/schema.json"
	ExtensionView       = "https://stac-extensions.github.io/view/v1.0.0/schema.json"
)
