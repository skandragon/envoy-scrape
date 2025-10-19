# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based monitoring system for Enphase Envoy solar controllers that scrapes inverter data and stores it in InfluxDB. The project is a single-binary application that connects to an Envoy solar controller's API, polls inverter data every minute, and writes updates to InfluxDB for Grafana visualization.

## Architecture

- **Single binary**: All code is in `main.go` (no package structure)
- **Data flow**: Envoy API → HTTP client (with TLS InsecureSkipVerify) → In-memory deduplication → InfluxDB
- **Deduplication**: Uses `knownInverters` map to track state and only write to InfluxDB when `LastReportDate` changes
- **Authentication**: Uses bearer token authentication via `ENVOY_TOKEN` environment variable
- **Polling**: Fetches from `/api/v1/production/inverters` endpoint every 60 seconds

## Build Commands

```bash
# Local development build (creates bin/envoy-scraper)
make local

# Run tests with race detector
make test

# Build multi-arch Docker images and push
make images

# Clean build artifacts
make clean
```

## Running Locally

Required environment variables:
- `ENVOY_TOKEN` - Bearer token for Envoy API authentication
- `ENVOY_HOST` - Hostname or IP of the Envoy device
- `ENVOY_SERIAL` - Serial number of the Envoy device
- `ENVOY_INFLUX_TOKEN` - InfluxDB authentication token
- `ENVOY_INFLUX_URL` - InfluxDB URL (defaults to http://10.45.220.3:8086)

```bash
# Build and run locally
make local
./bin/envoy-scraper
```

## Docker Configuration

The Dockerfile uses multi-stage builds with two targets:
- `envoy-scraper-image` - The polling scraper (this is the active component)
- `envoy-receiver-image` - Legacy receiver component (not currently used)

Multi-arch builds target: `linux/amd64,linux/arm64`

## Deployment

Kubernetes deployment configuration is in `kubernetes/deploy-scraper.yaml`. The scraper runs as a single replica deployment with environment variables for configuration.

## Key Technical Details

- Uses `influxdata/influxdb-client-go/v2` for InfluxDB writes
- Data is written to InfluxDB org "flame", bucket "envoy", measurement "inverterPower"
- TLS certificate verification is disabled for Envoy API calls
- InfluxDB writes are batched (batch size: 20)
- Per-inverter data includes: power (lastReportWatts), maxPower (maxReportWatts), serial number, device type
