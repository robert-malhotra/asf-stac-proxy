// Package config provides configuration management for the ASF-STAC proxy service.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v10"
)

// Config holds the complete application configuration loaded from environment variables.
type Config struct {
	Server   ServerConfig   `envPrefix:"SERVER_"`
	Backend  BackendConfig  `envPrefix:"BACKEND_"`
	ASF      ASFConfig      `envPrefix:"ASF_"`
	CMR      CMRConfig      `envPrefix:"CMR_"`
	STAC     STACConfig     `envPrefix:"STAC_"`
	Features FeatureConfig  `envPrefix:"FEATURE_"`
	Logging  LoggingConfig  `envPrefix:"LOG_"`
}

// ServerConfig contains HTTP server configuration.
type ServerConfig struct {
	Host            string        `env:"HOST" envDefault:"0.0.0.0"`
	Port            int           `env:"PORT" envDefault:"8080"`
	ReadTimeout     time.Duration `env:"READ_TIMEOUT" envDefault:"30s"`
	WriteTimeout    time.Duration `env:"WRITE_TIMEOUT" envDefault:"60s"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`
}

// BackendConfig contains backend selection configuration.
type BackendConfig struct {
	// Type specifies which backend to use: "asf" or "cmr"
	Type string `env:"TYPE" envDefault:"asf"`
}

// ASFConfig contains ASF API client configuration.
type ASFConfig struct {
	BaseURL string        `env:"BASE_URL" envDefault:"https://api.daac.asf.alaska.edu"`
	Timeout time.Duration `env:"TIMEOUT" envDefault:"30s"`
}

// CMRConfig contains CMR API client configuration.
type CMRConfig struct {
	BaseURL  string        `env:"BASE_URL" envDefault:"https://cmr.earthdata.nasa.gov/search"`
	Provider string        `env:"PROVIDER" envDefault:"ASF"`
	Timeout  time.Duration `env:"TIMEOUT" envDefault:"30s"`
}

// STACConfig contains STAC API metadata configuration.
type STACConfig struct {
	Version     string `env:"VERSION" envDefault:"1.0.0"`
	BaseURL     string `env:"BASE_URL"` // Public-facing URL (required)
	Title       string `env:"TITLE" envDefault:"ASF STAC API"`
	Description string `env:"DESCRIPTION" envDefault:"STAC API proxy for Alaska Satellite Facility"`
}

// FeatureConfig contains feature flags and limits.
type FeatureConfig struct {
	EnableSearch     bool `env:"ENABLE_SEARCH" envDefault:"true"`
	EnableQueryables bool `env:"ENABLE_QUERYABLES" envDefault:"true"`
	DefaultLimit     int  `env:"DEFAULT_LIMIT" envDefault:"10"`
	MaxLimit         int  `env:"MAX_LIMIT" envDefault:"250"`
}

// LoggingConfig contains logging configuration.
type LoggingConfig struct {
	Level  string `env:"LEVEL" envDefault:"info"`
	Format string `env:"FORMAT" envDefault:"json"`
}

// Load parses configuration from environment variables.
// It returns an error if required fields are missing or invalid.
func Load() (*Config, error) {
	cfg := &Config{}

	opts := env.Options{
		RequiredIfNoDef: true,
	}

	if err := env.ParseWithOptions(cfg, opts); err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	// Validate server config
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535, got %d", c.Server.Port)
	}

	if c.Server.ReadTimeout <= 0 {
		return fmt.Errorf("server read timeout must be positive, got %s", c.Server.ReadTimeout)
	}

	if c.Server.WriteTimeout <= 0 {
		return fmt.Errorf("server write timeout must be positive, got %s", c.Server.WriteTimeout)
	}

	if c.Server.ShutdownTimeout <= 0 {
		return fmt.Errorf("server shutdown timeout must be positive, got %s", c.Server.ShutdownTimeout)
	}

	// Validate backend config
	if c.Backend.Type != "asf" && c.Backend.Type != "cmr" {
		return fmt.Errorf("backend type must be 'asf' or 'cmr', got %q", c.Backend.Type)
	}

	// Validate ASF config
	if c.ASF.BaseURL == "" {
		return fmt.Errorf("ASF base URL is required")
	}

	if c.ASF.Timeout <= 0 {
		return fmt.Errorf("ASF timeout must be positive, got %s", c.ASF.Timeout)
	}

	// Validate CMR config
	if c.CMR.BaseURL == "" {
		return fmt.Errorf("CMR base URL is required")
	}

	if c.CMR.Timeout <= 0 {
		return fmt.Errorf("CMR timeout must be positive, got %s", c.CMR.Timeout)
	}

	// Validate STAC config
	if c.STAC.BaseURL == "" {
		return fmt.Errorf("STAC base URL is required")
	}

	if c.STAC.Version == "" {
		return fmt.Errorf("STAC version is required")
	}

	// Validate feature config
	if c.Features.DefaultLimit < 1 {
		return fmt.Errorf("default limit must be at least 1, got %d", c.Features.DefaultLimit)
	}

	if c.Features.MaxLimit < c.Features.DefaultLimit {
		return fmt.Errorf("max limit (%d) must be >= default limit (%d)", c.Features.MaxLimit, c.Features.DefaultLimit)
	}

	// Validate logging config
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level %q, must be one of: debug, info, warn, error", c.Logging.Level)
	}

	validLogFormats := map[string]bool{
		"json": true,
		"text": true,
	}
	if !validLogFormats[c.Logging.Format] {
		return fmt.Errorf("invalid log format %q, must be one of: json, text", c.Logging.Format)
	}

	return nil
}

// Address returns the server listen address in the format "host:port".
func (s *ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}
