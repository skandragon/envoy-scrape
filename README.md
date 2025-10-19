# envoy-scrape

Scrape data locally from a an Envoy solar controller

## Architecture

This system uses two components, one called "envoy-scraper" which runs in the same
network as the Envoy systems being monitored, and "envoy-receiver" which typically
will run on some hosted server somewhere.

Both components are built into a Docker image, and in my environment the scraper
is running in my home Kubernetes cluster, while the receiver runs on my Docker
host on my colo-based machine.

Display is presented via Grafana, and is not documented here.

## Security

The serial number configured for the scraper is stored in the clear, and it transmitted
to the receiver to differentiate between individual Envoy systems.  Additionally,
the microinverter serial numbers are transmitted to the receiver and stored in the
database.

The receiver and scraper use a shared secret to authenticate, which will someday
be converted to a JWT most likely.

## Scraper

The scraper needs to know two things about the Envoy:  its IP address and serial
number.  From the serial, the "installer password" is generated, and used to access
the API to retrieve information from the Envoy.  No configuration is changed on the
Envoy.

The scraper will gather data once per minute, and upload data if it changes only.
It seems that my Envoy only updates data once every five minutes, so this will
result in less redundant data sent to the receiver and less redundant data stored
in the database.

The scraper only gathers a subset of the available data, such as current and
peak output per inverter.  The serial numbers are sent to the receiver, so
analysis would be possible based on detailed data per inverter over time.

## Receiver

The receiver accepts data from one or more scrapers, and stores the data in
an InfluxDB database for Grafana to consume.
