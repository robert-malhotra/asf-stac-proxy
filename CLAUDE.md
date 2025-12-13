# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

ASF-STAC Proxy is a Go service that acts as a translation layer between STAC (SpatioTemporal Asset Catalog) API clients and the Alaska Satellite Facility (ASF) Search API. It exposes ASF's SAR data catalog through a standards-compliant STAC API.

## Build and Run Commands

```bash
# Build
go build -o asf-stac-proxy ./cmd/server

# Run
./asf-stac-proxy

# Run tests
go test ./...

# Run single test
go test -v -run TestName ./path/to/package

# Lint
golangci-lint run

# Docker build (uses Go 1.25)
docker build -t asf-stac-proxy .

# Docker run
docker run -p 8080:8080 -e BASE_URL=http://localhost:8080 asf-stac-proxy
```

## Architecture

### Core Components

1. **HTTP Server/Router** (`internal/api/`) - Uses chi router to handle incoming STAC API requests
2. **Handlers** (`internal/api/handlers.go`) - Implement STAC endpoint logic for landing page, conformance, collections, items, and search
3. **Translation Layer** (`internal/translate/`) - Converts between STAC request/response formats and ASF formats
4. **ASF Client** (`internal/asf/`) - HTTP client for the ASF Search API
5. **STAC Models** (`internal/stac/`) - STAC data structures and validation
6. **Configuration** (`internal/config/`) - Environment-based configuration

### Request Flow

```
STAC Client → Router → Handler → Translator → ASF Client → ASF API
                                     ↓
STAC Client ← Handler ← Translator ← ASF Response
```

### Key Translations

- **STAC `bbox`** → ASF `intersectsWith` (WKT POLYGON)
- **STAC `datetime`** → ASF `start` + `end`
- **STAC `intersects`** → ASF `intersectsWith` (GeoJSON to WKT)
- **STAC `collections`** → ASF `dataset`
- **STAC `ids`** → ASF `granule_list`

### STAC Collections

Collections are statically defined in `collections/*.json` and map to ASF datasets:
- `sentinel-1` → SENTINEL-1
- `sentinel-1-bursts` → SLC-BURST
- `opera-s1` → OPERA-S1
- `alos-palsar` → ALOS PALSAR
- And others (see design.md for full mapping)

## Configuration

Key environment variables:
- `BASE_URL` (required) - Public URL of this service
- `ASF_BASE_URL` - ASF API base URL (default: `https://api.daac.asf.alaska.edu`)
- `PORT` - Listen port (default: `8080`)
- `DEFAULT_LIMIT` / `MAX_LIMIT` - Pagination limits (default: 10/250)

## STAC Extensions

The proxy supports these STAC extensions for SAR data:
- SAR Extension (`sar:instrument_mode`, `sar:polarizations`, etc.)
- Satellite Extension (`sat:orbit_state`, `sat:relative_orbit`, etc.)
- Processing Extension (`processing:level`)

## Key Design Decisions

- **Stateless proxy**: No caching or persistence
- **Offset-based pagination**: Uses opaque page tokens encoding offset/limit
- **Static collections**: Collection metadata is configured, not fetched from ASF
- **Authentication pass-through**: No auth handling in proxy
