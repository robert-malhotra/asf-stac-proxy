package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Set required environment variables
	os.Setenv("STAC_BASE_URL", "https://example.com")
	defer os.Unsetenv("STAC_BASE_URL")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Test defaults
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected default host 0.0.0.0, got %s", cfg.Server.Host)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}

	if cfg.ASF.BaseURL != "https://api.daac.asf.alaska.edu" {
		t.Errorf("expected default ASF base URL, got %s", cfg.ASF.BaseURL)
	}

	if cfg.STAC.Version != "1.0.0" {
		t.Errorf("expected default STAC version 1.0.0, got %s", cfg.STAC.Version)
	}

	if cfg.Features.DefaultLimit != 10 {
		t.Errorf("expected default limit 10, got %d", cfg.Features.DefaultLimit)
	}

	if cfg.Logging.Level != "info" {
		t.Errorf("expected default log level info, got %s", cfg.Logging.Level)
	}
}

func TestLoadWithCustomValues(t *testing.T) {
	// Set custom environment variables
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("SERVER_READ_TIMEOUT", "60s")
	os.Setenv("ASF_TIMEOUT", "45s")
	os.Setenv("STAC_BASE_URL", "https://stac.example.com")
	os.Setenv("STAC_VERSION", "1.0.0-rc.1")
	os.Setenv("FEATURE_DEFAULT_LIMIT", "25")
	os.Setenv("FEATURE_MAX_LIMIT", "500")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("LOG_FORMAT", "text")

	defer func() {
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("SERVER_READ_TIMEOUT")
		os.Unsetenv("ASF_TIMEOUT")
		os.Unsetenv("STAC_BASE_URL")
		os.Unsetenv("STAC_VERSION")
		os.Unsetenv("FEATURE_DEFAULT_LIMIT")
		os.Unsetenv("FEATURE_MAX_LIMIT")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("LOG_FORMAT")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}

	if cfg.Server.ReadTimeout != 60*time.Second {
		t.Errorf("expected read timeout 60s, got %s", cfg.Server.ReadTimeout)
	}

	if cfg.ASF.Timeout != 45*time.Second {
		t.Errorf("expected ASF timeout 45s, got %s", cfg.ASF.Timeout)
	}

	if cfg.STAC.BaseURL != "https://stac.example.com" {
		t.Errorf("expected STAC base URL https://stac.example.com, got %s", cfg.STAC.BaseURL)
	}

	if cfg.Features.DefaultLimit != 25 {
		t.Errorf("expected default limit 25, got %d", cfg.Features.DefaultLimit)
	}

	if cfg.Features.MaxLimit != 500 {
		t.Errorf("expected max limit 500, got %d", cfg.Features.MaxLimit)
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.Logging.Level)
	}

	if cfg.Logging.Format != "text" {
		t.Errorf("expected log format text, got %s", cfg.Logging.Format)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *Config
		wantError bool
	}{
		{
			name: "valid config",
			cfg: &Config{
				Server: ServerConfig{
					Host:            "0.0.0.0",
					Port:            8080,
					ReadTimeout:     30 * time.Second,
					WriteTimeout:    60 * time.Second,
					ShutdownTimeout: 10 * time.Second,
				},
				Backend: BackendConfig{
					Type: "asf",
				},
				ASF: ASFConfig{
					BaseURL: "https://api.daac.asf.alaska.edu",
					Timeout: 30 * time.Second,
				},
				CMR: CMRConfig{
					BaseURL:  "https://cmr.earthdata.nasa.gov/search",
					Provider: "ASF",
					Timeout:  30 * time.Second,
				},
				STAC: STACConfig{
					Version: "1.0.0",
					BaseURL: "https://stac.example.com",
				},
				Features: FeatureConfig{
					DefaultLimit: 10,
					MaxLimit:     250,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
				},
			},
			wantError: false,
		},
		{
			name: "valid config with CMR backend",
			cfg: &Config{
				Server: ServerConfig{
					Host:            "0.0.0.0",
					Port:            8080,
					ReadTimeout:     30 * time.Second,
					WriteTimeout:    60 * time.Second,
					ShutdownTimeout: 10 * time.Second,
				},
				Backend: BackendConfig{
					Type: "cmr",
				},
				ASF: ASFConfig{
					BaseURL: "https://api.daac.asf.alaska.edu",
					Timeout: 30 * time.Second,
				},
				CMR: CMRConfig{
					BaseURL:  "https://cmr.earthdata.nasa.gov/search",
					Provider: "ASF",
					Timeout:  30 * time.Second,
				},
				STAC: STACConfig{
					Version: "1.0.0",
					BaseURL: "https://stac.example.com",
				},
				Features: FeatureConfig{
					DefaultLimit: 10,
					MaxLimit:     250,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
				},
			},
			wantError: false,
		},
		{
			name: "invalid port",
			cfg: &Config{
				Server: ServerConfig{
					Port:            0,
					ReadTimeout:     30 * time.Second,
					WriteTimeout:    60 * time.Second,
					ShutdownTimeout: 10 * time.Second,
				},
				Backend: BackendConfig{
					Type: "asf",
				},
				ASF: ASFConfig{
					BaseURL: "https://api.daac.asf.alaska.edu",
					Timeout: 30 * time.Second,
				},
				CMR: CMRConfig{
					BaseURL:  "https://cmr.earthdata.nasa.gov/search",
					Provider: "ASF",
					Timeout:  30 * time.Second,
				},
				STAC: STACConfig{
					Version: "1.0.0",
					BaseURL: "https://stac.example.com",
				},
				Features: FeatureConfig{
					DefaultLimit: 10,
					MaxLimit:     250,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
				},
			},
			wantError: true,
		},
		{
			name: "invalid backend type",
			cfg: &Config{
				Server: ServerConfig{
					Port:            8080,
					ReadTimeout:     30 * time.Second,
					WriteTimeout:    60 * time.Second,
					ShutdownTimeout: 10 * time.Second,
				},
				Backend: BackendConfig{
					Type: "invalid",
				},
				ASF: ASFConfig{
					BaseURL: "https://api.daac.asf.alaska.edu",
					Timeout: 30 * time.Second,
				},
				CMR: CMRConfig{
					BaseURL:  "https://cmr.earthdata.nasa.gov/search",
					Provider: "ASF",
					Timeout:  30 * time.Second,
				},
				STAC: STACConfig{
					Version: "1.0.0",
					BaseURL: "https://stac.example.com",
				},
				Features: FeatureConfig{
					DefaultLimit: 10,
					MaxLimit:     250,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
				},
			},
			wantError: true,
		},
		{
			name: "missing STAC base URL",
			cfg: &Config{
				Server: ServerConfig{
					Port:            8080,
					ReadTimeout:     30 * time.Second,
					WriteTimeout:    60 * time.Second,
					ShutdownTimeout: 10 * time.Second,
				},
				Backend: BackendConfig{
					Type: "asf",
				},
				ASF: ASFConfig{
					BaseURL: "https://api.daac.asf.alaska.edu",
					Timeout: 30 * time.Second,
				},
				CMR: CMRConfig{
					BaseURL:  "https://cmr.earthdata.nasa.gov/search",
					Provider: "ASF",
					Timeout:  30 * time.Second,
				},
				STAC: STACConfig{
					Version: "1.0.0",
					BaseURL: "",
				},
				Features: FeatureConfig{
					DefaultLimit: 10,
					MaxLimit:     250,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
				},
			},
			wantError: true,
		},
		{
			name: "invalid log level",
			cfg: &Config{
				Server: ServerConfig{
					Port:            8080,
					ReadTimeout:     30 * time.Second,
					WriteTimeout:    60 * time.Second,
					ShutdownTimeout: 10 * time.Second,
				},
				Backend: BackendConfig{
					Type: "asf",
				},
				ASF: ASFConfig{
					BaseURL: "https://api.daac.asf.alaska.edu",
					Timeout: 30 * time.Second,
				},
				CMR: CMRConfig{
					BaseURL:  "https://cmr.earthdata.nasa.gov/search",
					Provider: "ASF",
					Timeout:  30 * time.Second,
				},
				STAC: STACConfig{
					Version: "1.0.0",
					BaseURL: "https://stac.example.com",
				},
				Features: FeatureConfig{
					DefaultLimit: 10,
					MaxLimit:     250,
				},
				Logging: LoggingConfig{
					Level:  "invalid",
					Format: "json",
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestServerConfigAddress(t *testing.T) {
	cfg := ServerConfig{
		Host: "localhost",
		Port: 3000,
	}

	addr := cfg.Address()
	expected := "localhost:3000"
	if addr != expected {
		t.Errorf("Address() = %s, expected %s", addr, expected)
	}
}
