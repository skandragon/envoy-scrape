# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based monitoring system for Enphase Envoy solar controllers that scrapes inverter data and exports it as OpenTelemetry metrics. The project is a single-binary application that connects to an Envoy solar controller's API, polls inverter data every minute, and records gauges exported via OTLP/gRPC.

## Architecture

- **Single binary**: All code is in `main.go` (no package structure)
- **Data flow**: Envoy API → HTTP client (with TLS InsecureSkipVerify) → OTel Int64Gauges → OTLP/gRPC (periodic reader, 60s)
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
- `OTEL_EXPORTER_OTLP_ENDPOINT` - OTLP/gRPC endpoint (defaults to localhost:4317); other standard `OTEL_*` vars apply

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

The live deployment is in `../kubernetes-clusters/clusters/kubepi/envoy-scraper` (ArgoCD); it sends to the node-local cardinalhq collector-agent. `kubernetes/deploy-scraper.yaml` here is an example. The scraper runs as a single replica deployment with environment variables for configuration.

## Key Technical Details

- Metrics: `solar.envoy.inverter.power`, `solar.envoy.inverter.power.max` (W), `solar.envoy.inverter.last_report` (unix s); attributes `inverter.serial`, `inverter.type`
- TLS certificate verification is disabled for Envoy API calls
