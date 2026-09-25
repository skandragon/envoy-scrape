# envoy-scrape

Scrape per-inverter data from an Enphase Envoy solar controller and export it
as OpenTelemetry metrics.

Once a minute the scraper fetches `/api/v1/production/inverters` and records:

| Metric | Unit | Meaning |
|---|---|---|
| `solar.envoy.inverter.power` | W | last reported power |
| `solar.envoy.inverter.power.max` | W | max reported power |
| `solar.envoy.inverter.last_report` | s | unix time of the inverter's last report |

Attributes: `site.id`, `inverter.serial`, `inverter.type`.

## Configuration

- `ENVOY_HOST` - hostname or IP of the Envoy
- `ENVOY_SERIAL` - serial number of the Envoy
- `ENVOY_SITE_ID` - site ID shared by all Envoys at one site (exported as `site.id`)
- `ENVOY_TOKEN` - owner token from https://entrez.enphaseenergy.com/ (valid one year)
- `OTEL_EXPORTER_OTLP_ENDPOINT` - OTLP/gRPC endpoint (default `localhost:4317`), plus any other standard `OTEL_*` variables
