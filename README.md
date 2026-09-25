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

Every 15 seconds it also reads `/ivp/meters/readings` and, for each enabled CT
meter, records whole-meter totals:

| Metric | Unit |
|---|---|
| `solar.envoy.meter.power` | W |
| `solar.envoy.meter.power.apparent` | VA |
| `solar.envoy.meter.power.reactive` | var |
| `solar.envoy.meter.power_factor` | 1 |
| `solar.envoy.meter.voltage` | V |
| `solar.envoy.meter.current` | A |
| `solar.envoy.meter.frequency` | Hz |
| `solar.envoy.meter.energy.delivered` | Wh (lifetime) |
| `solar.envoy.meter.energy.received` | Wh (lifetime) |

Attributes: `site.id`, `meter.type` (`production`, `net-consumption`, ...).
Metrics are exported every 15 seconds.

## Configuration

- `ENVOY_HOST` - hostname or IP of the Envoy
- `ENVOY_SERIAL` - serial number of the Envoy
- `ENVOY_SITE_ID` - site ID shared by all Envoys at one site (exported as `site.id`)
- `ENVOY_TOKEN` - owner token from https://entrez.enphaseenergy.com/ (valid one year)
- `OTEL_EXPORTER_OTLP_ENDPOINT` - OTLP/gRPC endpoint (default `localhost:4317`), plus any other standard `OTEL_*` variables
