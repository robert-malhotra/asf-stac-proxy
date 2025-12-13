# ASF-STAC Proxy

A Go service that exposes Alaska Satellite Facility (ASF) SAR data through a standards-compliant [STAC API](https://stacspec.org/). Supports both ASF Search API and NASA CMR as backends.

## Features

- **STAC API 1.0.0** compliant endpoints
- **Dual backend support**: ASF Search API or NASA CMR
- **6 SAR collections**: Sentinel-1, ALOS-PALSAR, RADARSAT-1, ERS-1/2, UAVSAR, OPERA-S1
- **SAR extensions**: Full support for sar, sat, and processing STAC extensions
- **Queryables endpoint**: JSON Schema with collection-specific enums
- **Cursor-based pagination**: CMR native or server-side for ASF
- **Minimal Docker image**: ~8MB scratch-based container

## Quick Start

```bash
# Run with Docker
docker run -p 8080:8080 -e STAC_BASE_URL=http://localhost:8080 asf-stac-proxy

# Or build and run locally
make run

# Run with CMR backend
make run-cmr
```

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /` | Landing page |
| `GET /conformance` | Conformance classes |
| `GET /collections` | List all collections |
| `GET /collections/{id}` | Collection metadata |
| `GET /collections/{id}/items` | Items in collection |
| `GET /collections/{id}/items/{itemId}` | Single item |
| `GET,POST /search` | Cross-collection search |
| `GET /queryables` | Global queryables |
| `GET /collections/{id}/queryables` | Collection queryables |
| `GET /health` | Health check |

## Collections

| Collection | Platform | Band | Temporal Range |
|------------|----------|------|----------------|
| `sentinel-1` | Sentinel-1A/B/C | C | 2014 → present |
| `opera-s1` | Sentinel-1A/B | C | 2014 → present |
| `alos-palsar` | ALOS | L | 2006 → 2011 |
| `radarsat-1` | RADARSAT-1 | C | 1995 → 2013 |
| `ers` | ERS-1, ERS-2 | C | 1991 → 2011 |
| `uavsar` | G-III aircraft | L | 2008 → present |

## Search Examples

```bash
# Search Sentinel-1 SLC over Alaska
curl "http://localhost:8080/search?collections=sentinel-1&bbox=-170,50,-130,72&datetime=2024-01-01/2024-12-31"

# POST search with GeoJSON
curl -X POST http://localhost:8080/search \
  -H "Content-Type: application/json" \
  -d '{
    "collections": ["sentinel-1"],
    "bbox": [-125, 24, -66, 50],
    "datetime": "2024-01-01/2024-12-31",
    "limit": 10
  }'
```

## Queryables

All collections support these queryable properties:

| Property | Type | Description |
|----------|------|-------------|
| `datetime` | string | Datetime or range |
| `bbox` | array | Bounding box |
| `intersects` | geometry | GeoJSON geometry |
| `sar:instrument_mode` | string | IW, EW, SM, WV, etc. |
| `sar:polarizations` | array | VV, VH, HH, HV |
| `sat:orbit_state` | string | ascending, descending |
| `sat:relative_orbit` | integer | Relative orbit number |
| `processing:level` | string | SLC, GRD_HD, RAW, etc. |
| `platform` | string | sentinel-1a, alos, etc. |

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `STAC_BASE_URL` | *required* | Public URL of this service |
| `BACKEND_TYPE` | `asf` | Backend: `asf` or `cmr` |
| `SERVER_PORT` | `8080` | Listen port |
| `LOG_LEVEL` | `info` | debug, info, warn, error |
| `LOG_FORMAT` | `json` | json, text |
| `FEATURE_DEFAULT_LIMIT` | `10` | Default results per page |
| `FEATURE_MAX_LIMIT` | `250` | Max results per page |
| `ASF_BASE_URL` | `https://api.daac.asf.alaska.edu` | ASF API URL |
| `CMR_BASE_URL` | `https://cmr.earthdata.nasa.gov/search` | CMR API URL |
| `CMR_PROVIDER` | `ASF` | CMR provider |

## Development

```bash
make help          # Show all commands
make test          # Run tests
make build         # Build binary
make run           # Run with ASF backend
make run-cmr       # Run with CMR backend
make lint          # Run linter
make docker-build  # Build Docker image
make compare       # Compare ASF vs CMR results
```

## Docker

```bash
# Build image
make docker-build

# Run container
docker run -d -p 8080:8080 \
  -e STAC_BASE_URL=https://your-domain.com \
  -e BACKEND_TYPE=cmr \
  asf-stac-proxy:latest

# Multi-arch build (amd64 + arm64)
make docker-build-multiarch
```

## Architecture

```
STAC Client → Router → Handler → Backend → ASF API / CMR API
                                    ↓
STAC Client ← Handler ← Translator ← Response
```

The proxy translates:
- `bbox` → WKT POLYGON (ASF) / bounding_box (CMR)
- `datetime` → start/end (ASF) / temporal (CMR)
- `collections` → dataset (ASF) / short_name (CMR)
- `sar:*` filters → beamMode, polarization, etc.

## License

MIT
