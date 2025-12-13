// Package server provides a public API for embedding the ASF STAC proxy.
package server

import (
	"log/slog"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rkm/asf-stac-proxy/internal/api"
	"github.com/rkm/asf-stac-proxy/internal/asf"
	"github.com/rkm/asf-stac-proxy/internal/backend"
	"github.com/rkm/asf-stac-proxy/internal/cmr"
	"github.com/rkm/asf-stac-proxy/internal/config"
	"github.com/rkm/asf-stac-proxy/internal/stac"
	"github.com/rkm/asf-stac-proxy/internal/translate"
)

// BackendType specifies which upstream data source to use.
type BackendType string

const (
	// BackendASF uses the ASF Search API as the data source.
	BackendASF BackendType = "asf"
	// BackendCMR uses NASA's Common Metadata Repository as the data source.
	BackendCMR BackendType = "cmr"
)

// Options configures the ASF STAC server.
type Options struct {
	// BaseURL is the public-facing URL for self-referential links (required).
	// Example: "https://api.example.com/asf" or "http://localhost:8080"
	BaseURL string

	// Backend specifies which upstream data source to use.
	// Default: BackendASF
	Backend BackendType

	// ASFBaseURL is the ASF Search API base URL.
	// Default: "https://api.daac.asf.alaska.edu"
	ASFBaseURL string

	// CMRBaseURL is the NASA CMR API base URL.
	// Default: "https://cmr.earthdata.nasa.gov/search"
	CMRBaseURL string

	// CMRProvider is the CMR provider ID.
	// Default: "ASF"
	CMRProvider string

	// Timeout is the upstream request timeout.
	// Default: 30s
	Timeout time.Duration

	// Title is the STAC API title.
	// Default: "ASF STAC API"
	Title string

	// Description is the STAC API description.
	// Default: "STAC API proxy for Alaska Satellite Facility"
	Description string

	// DefaultLimit is the default number of items per page.
	// Default: 10
	DefaultLimit int

	// MaxLimit is the maximum number of items per page.
	// Default: 250
	MaxLimit int

	// EnableSearch enables the /search endpoint.
	// Default: true
	EnableSearch bool

	// EnableQueryables enables the /queryables endpoint.
	// Default: true
	EnableQueryables bool

	// CollectionsDir is the path to collection definition JSON files.
	// Default: "" (uses built-in defaults)
	CollectionsDir string

	// Logger is the slog logger to use.
	// Default: slog.Default()
	Logger *slog.Logger
}

// Server is an ASF STAC proxy server that can be embedded in another application.
type Server struct {
	router      chi.Router
	cursorStore *stac.MemoryCursorStore
}

// New creates a new ASF STAC server with the given options.
func New(opts Options) (*Server, error) {
	// Apply defaults
	if opts.Backend == "" {
		opts.Backend = BackendASF
	}
	if opts.ASFBaseURL == "" {
		opts.ASFBaseURL = "https://api.daac.asf.alaska.edu"
	}
	if opts.CMRBaseURL == "" {
		opts.CMRBaseURL = "https://cmr.earthdata.nasa.gov/search"
	}
	if opts.CMRProvider == "" {
		opts.CMRProvider = "ASF"
	}
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.Title == "" {
		opts.Title = "ASF STAC API"
	}
	if opts.Description == "" {
		opts.Description = "STAC API proxy for Alaska Satellite Facility"
	}
	if opts.DefaultLimit == 0 {
		opts.DefaultLimit = 10
	}
	if opts.MaxLimit == 0 {
		opts.MaxLimit = 250
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	// Build internal config
	cfg := &config.Config{
		Backend: config.BackendConfig{
			Type: string(opts.Backend),
		},
		ASF: config.ASFConfig{
			BaseURL: opts.ASFBaseURL,
			Timeout: opts.Timeout,
		},
		CMR: config.CMRConfig{
			BaseURL:  opts.CMRBaseURL,
			Provider: opts.CMRProvider,
			Timeout:  opts.Timeout,
		},
		STAC: config.STACConfig{
			Version:     "1.0.0",
			BaseURL:     opts.BaseURL,
			Title:       opts.Title,
			Description: opts.Description,
		},
		Features: config.FeatureConfig{
			EnableSearch:     opts.EnableSearch,
			EnableQueryables: opts.EnableQueryables,
			DefaultLimit:     opts.DefaultLimit,
			MaxLimit:         opts.MaxLimit,
		},
	}

	// Load collections
	var collections *config.CollectionRegistry
	var err error
	if opts.CollectionsDir != "" {
		collections, err = config.LoadCollections(opts.CollectionsDir)
		if err != nil {
			opts.Logger.Warn("failed to load collections, using empty registry",
				"dir", opts.CollectionsDir,
				"error", err,
			)
			collections = config.NewCollectionRegistry()
		}
	} else {
		collections = config.NewCollectionRegistry()
	}

	// Create translator
	translator := translate.NewTranslator(cfg, collections, opts.Logger)

	// Create cursor store
	cursorStore := stac.NewMemoryCursorStore(1*time.Hour, 5*time.Minute)

	// Create search backend
	var searchBackend backend.SearchBackend
	switch opts.Backend {
	case BackendCMR:
		cmrClient := cmr.NewClient(cfg.CMR.BaseURL, cfg.CMR.Provider, cfg.CMR.Timeout).WithLogger(opts.Logger)
		searchBackend = cmr.NewCMRBackend(cmrClient, collections, cfg, opts.Logger)
		opts.Logger.Info("using CMR backend", "base_url", cfg.CMR.BaseURL, "provider", cfg.CMR.Provider)
	default:
		asfClient := asf.NewClient(cfg.ASF.BaseURL, cfg.ASF.Timeout).WithLogger(opts.Logger)
		searchBackend = backend.NewASFBackend(asfClient, collections, translator, cfg, opts.Logger)
		opts.Logger.Info("using ASF backend", "base_url", cfg.ASF.BaseURL)
	}

	// Create handlers
	handlers := api.NewHandlers(cfg, searchBackend, translator, collections, opts.Logger).
		WithCursorStore(cursorStore)

	// Create router
	router := api.NewRouter(handlers, opts.Logger)

	return &Server{
		router:      router,
		cursorStore: cursorStore,
	}, nil
}

// Router returns the chi.Router for mounting in another application.
func (s *Server) Router() chi.Router {
	return s.router
}

// Close stops background goroutines (cursor cleanup).
func (s *Server) Close() {
	if s.cursorStore != nil {
		s.cursorStore.Stop()
	}
}
