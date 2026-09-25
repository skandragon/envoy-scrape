/*
 * Copyright 2022 Michael Graff.
 *
 * Licensed under the Apache License, Version 2.0 (the "License")
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type inverterReport struct {
	SerialNumber    string `json:"serialNumber,omitempty"`
	LastReportDate  int    `json:"lastReportDate,omitempty"`
	DevType         int    `json:"devType,omitempty"`
	LastReportWatts int    `json:"lastReportWatts,omitempty"`
	MaxReportWatts  int    `json:"maxReportWatts,omitempty"`
}

var (
	serial = flag.String("serial", "", "serial number of the Envoy")
	host   = flag.String("host", "", "the hostname or IP address of the Envoy")

	envoyClient = &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
)

func main() {
	flag.Parse()

	if *serial == "" {
		*serial = os.Getenv("ENVOY_SERIAL")
	}
	if *host == "" {
		*host = os.Getenv("ENVOY_HOST")
	}
	if *host == "" || *serial == "" {
		log.Printf("host and serial must be set")
		flag.Usage()
		os.Exit(-1)
	}
	token, set := os.LookupEnv("ENVOY_TOKEN")
	if !set {
		log.Fatalf("ENVOY_TOKEN is not set")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Endpoint, service name, etc. come from the standard OTEL_* env vars.
	exporter, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		log.Fatalf("otlp exporter: %v", err)
	}
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)))
	defer func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			log.Printf("meter provider shutdown: %v", err)
		}
	}()

	meter := provider.Meter("github.com/skandragon/envoy-scrape")
	power, err := meter.Int64Gauge("solar.envoy.inverter.power", metric.WithUnit("W"),
		metric.WithDescription("Most recent power reported by the inverter"))
	if err != nil {
		log.Fatal(err)
	}
	maxPower, err := meter.Int64Gauge("solar.envoy.inverter.power.max", metric.WithUnit("W"),
		metric.WithDescription("Maximum power reported by the inverter"))
	if err != nil {
		log.Fatal(err)
	}
	lastReport, err := meter.Int64Gauge("solar.envoy.inverter.last_report", metric.WithUnit("s"),
		metric.WithDescription("Unix time of the inverter's last report"))
	if err != nil {
		log.Fatal(err)
	}

	url := fmt.Sprintf("https://%s/api/v1/production/inverters", *host)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		inverters, err := fetch(ctx, token, url)
		if err != nil {
			log.Printf("%v", err)
		}
		for _, i := range inverters {
			attrs := metric.WithAttributes(
				attribute.String("inverter.serial", i.SerialNumber),
				attribute.String("inverter.type", strconv.Itoa(i.DevType)),
			)
			power.Record(ctx, int64(i.LastReportWatts), attrs)
			maxPower.Record(ctx, int64(i.MaxReportWatts), attrs)
			lastReport.Record(ctx, int64(i.LastReportDate), attrs)
		}
		log.Printf("fetch complete. %d inverters", len(inverters))

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func fetch(ctx context.Context, token string, url string) ([]inverterReport, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := envoyClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s: %s", url, resp.Status, body)
	}

	var inverters []inverterReport
	if err := json.Unmarshal(body, &inverters); err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	return inverters, nil
}
