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
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type inverterReport struct {
	SerialNumber    string `json:"serialNumber,omitempty"`
	LastReportDate  int    `json:"lastReportDate,omitempty"`
	DevType         int    `json:"devType,omitempty"`
	LastReportWatts int    `json:"lastReportWatts,omitempty"`
	MaxReportWatts  int    `json:"maxReportWatts,omitempty"`
}

// meterInterval is both the meter poll rate and the OTLP export interval.
const meterInterval = 15 * time.Second

type meterInfo struct {
	EID             int64  `json:"eid"`
	State           string `json:"state"`
	MeasurementType string `json:"measurementType"`
}

// meterReading is the whole-meter total; per-phase "channels" are ignored.
type meterReading struct {
	EID           int64   `json:"eid"`
	ActivePower   float64 `json:"activePower"`
	ApparentPower float64 `json:"apparentPower"`
	ReactivePower float64 `json:"reactivePower"`
	PwrFactor     float64 `json:"pwrFactor"`
	Voltage       float64 `json:"voltage"`
	Current       float64 `json:"current"`
	Freq          float64 `json:"freq"`
	ActEnergyDlvd float64 `json:"actEnergyDlvd"`
	ActEnergyRcvd float64 `json:"actEnergyRcvd"`
}

// layout is the subset of Enlighten's array layout we use; the full JSON is
// exported as a log record for dashboards.
type layout struct {
	Arrays []struct {
		Label   string `json:"label"`
		Modules []struct {
			Inverter struct {
				SerialNum string `json:"serial_num"`
			} `json:"inverter"`
		} `json:"modules"`
	} `json:"arrays"`
}

const enlighten = "https://enlighten.enphaseenergy.com"

var (
	printLayout = flag.Bool("layout", false, "print the Enlighten array layout JSON and exit")

	serial = flag.String("serial", "", "serial number of the Envoy")
	host   = flag.String("host", "", "the hostname or IP address of the Envoy")
	siteID = flag.String("site", "", "site ID grouping one or more Envoys")

	envoyClient = &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
)

func main() {
	flag.Parse()

	email, password := os.Getenv("ENLIGHTEN_EMAIL"), os.Getenv("ENLIGHTEN_PASSWORD")
	if *printLayout {
		body, err := fetchLayout(context.Background(), email, password)
		if err != nil {
			log.Fatal(err)
		}
		os.Stdout.Write(body)
		return
	}

	if *serial == "" {
		*serial = os.Getenv("ENVOY_SERIAL")
	}
	if *host == "" {
		*host = os.Getenv("ENVOY_HOST")
	}
	if *siteID == "" {
		*siteID = os.Getenv("ENVOY_SITE_ID")
	}
	if *host == "" || *serial == "" || *siteID == "" {
		log.Printf("host, serial, and site must be set")
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
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(
		sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(meterInterval))))
	defer func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			log.Printf("meter provider shutdown: %v", err)
		}
	}()

	logExporter, err := otlploggrpc.New(ctx)
	if err != nil {
		log.Fatalf("otlp log exporter: %v", err)
	}
	logProvider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)))
	defer func() {
		if err := logProvider.Shutdown(context.Background()); err != nil {
			log.Printf("logger provider shutdown: %v", err)
		}
	}()
	logger := logProvider.Logger("github.com/skandragon/envoy-scrape")

	meter := provider.Meter("github.com/skandragon/envoy-scrape")
	intGauge := func(name, unit, desc string) metric.Int64Gauge {
		g, err := meter.Int64Gauge(name, metric.WithUnit(unit), metric.WithDescription(desc))
		if err != nil {
			log.Fatal(err)
		}
		return g
	}
	floatGauge := func(name, unit, desc string) metric.Float64Gauge {
		g, err := meter.Float64Gauge(name, metric.WithUnit(unit), metric.WithDescription(desc))
		if err != nil {
			log.Fatal(err)
		}
		return g
	}

	power := intGauge("solar.envoy.inverter.power", "W", "Most recent power reported by the inverter")
	maxPower := intGauge("solar.envoy.inverter.power.max", "W", "Maximum power reported by the inverter")
	lastReport := intGauge("solar.envoy.inverter.last_report", "s", "Unix time of the inverter's last report")

	meterGauges := []struct {
		g     metric.Float64Gauge
		value func(meterReading) float64
	}{
		{floatGauge("solar.envoy.meter.power", "W", "Active power"), func(r meterReading) float64 { return r.ActivePower }},
		{floatGauge("solar.envoy.meter.power.apparent", "VA", "Apparent power"), func(r meterReading) float64 { return r.ApparentPower }},
		{floatGauge("solar.envoy.meter.power.reactive", "var", "Reactive power"), func(r meterReading) float64 { return r.ReactivePower }},
		{floatGauge("solar.envoy.meter.power_factor", "1", "Power factor"), func(r meterReading) float64 { return r.PwrFactor }},
		{floatGauge("solar.envoy.meter.voltage", "V", "RMS voltage"), func(r meterReading) float64 { return r.Voltage }},
		{floatGauge("solar.envoy.meter.current", "A", "RMS current"), func(r meterReading) float64 { return r.Current }},
		{floatGauge("solar.envoy.meter.frequency", "Hz", "Line frequency"), func(r meterReading) float64 { return r.Freq }},
		{floatGauge("solar.envoy.meter.energy.delivered", "Wh", "Lifetime energy delivered"), func(r meterReading) float64 { return r.ActEnergyDlvd }},
		{floatGauge("solar.envoy.meter.energy.received", "Wh", "Lifetime energy received"), func(r meterReading) float64 { return r.ActEnergyRcvd }},
	}

	base := fmt.Sprintf("https://%s", *host)

	// inverter serial -> array label, refreshed daily from Enlighten.
	arrayOf := map[string]string{}
	pollLayout := func() {
		body, err := fetchLayout(ctx, email, password)
		if err != nil {
			log.Printf("layout: %v", err)
			return
		}
		m, err := arrayBySerial(body)
		if err != nil {
			log.Printf("layout: %v", err)
			return
		}
		arrayOf = m
		var rec otellog.Record
		rec.SetTimestamp(time.Now())
		rec.SetEventName("solar.envoy.layout")
		rec.SetSeverity(otellog.SeverityInfo)
		rec.SetBody(attribute.StringValue(string(body)))
		rec.AddAttributes(attribute.String("site.id", *siteID))
		logger.Emit(ctx, rec)
		log.Printf("layout fetched. %d inverters", len(m))
	}

	pollInverters := func() {
		var inverters []inverterReport
		if err := fetch(ctx, token, base+"/api/v1/production/inverters", &inverters); err != nil {
			log.Printf("%v", err)
			return
		}
		for _, i := range inverters {
			kv := []attribute.KeyValue{
				attribute.String("site.id", *siteID),
				attribute.String("inverter.serial", i.SerialNumber),
				attribute.String("inverter.type", strconv.Itoa(i.DevType)),
			}
			if a, ok := arrayOf[i.SerialNumber]; ok {
				kv = append(kv, attribute.String("array.name", a))
			}
			attrs := metric.WithAttributes(kv...)
			power.Record(ctx, int64(i.LastReportWatts), attrs)
			maxPower.Record(ctx, int64(i.MaxReportWatts), attrs)
			lastReport.Record(ctx, int64(i.LastReportDate), attrs)
		}
		log.Printf("fetch complete. %d inverters", len(inverters))
	}

	pollMeters := func() {
		var meters []meterInfo
		if err := fetch(ctx, token, base+"/ivp/meters", &meters); err != nil {
			log.Printf("%v", err)
			return
		}
		var readings []meterReading
		if err := fetch(ctx, token, base+"/ivp/meters/readings", &readings); err != nil {
			log.Printf("%v", err)
			return
		}
		enabled := map[int64]string{}
		for _, m := range meters {
			if m.State == "enabled" {
				enabled[m.EID] = m.MeasurementType
			}
		}
		for _, r := range readings {
			mtype, ok := enabled[r.EID]
			if !ok {
				continue
			}
			attrs := metric.WithAttributes(
				attribute.String("site.id", *siteID),
				attribute.String("meter.type", mtype),
			)
			for _, mg := range meterGauges {
				mg.g.Record(ctx, mg.value(r), attrs)
			}
		}
	}

	inverterTicker := time.NewTicker(time.Minute)
	defer inverterTicker.Stop()
	meterTicker := time.NewTicker(meterInterval)
	defer meterTicker.Stop()
	layoutTicker := time.NewTicker(24 * time.Hour)
	defer layoutTicker.Stop()
	if email == "" {
		layoutTicker.Stop()
	} else {
		pollLayout()
	}
	pollInverters()
	pollMeters()
	for {
		select {
		case <-ctx.Done():
			return
		case <-layoutTicker.C:
			pollLayout()
		case <-inverterTicker.C:
			pollInverters()
		case <-meterTicker.C:
			pollMeters()
		}
	}
}

func fetch(ctx context.Context, token string, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := envoyClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s: %s", url, resp.Status, body)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%s: %w", url, err)
	}
	return nil
}

// fetchLayout logs into Enlighten and returns the raw array layout JSON for
// the account's system. These are Enlighten's undocumented web endpoints.
func fetchLayout(ctx context.Context, email, password string) ([]byte, error) {
	if email == "" || password == "" {
		return nil, fmt.Errorf("ENLIGHTEN_EMAIL and ENLIGHTEN_PASSWORD must be set")
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}

	get := func(req *http.Request) ([]byte, error) {
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: %s", req.URL.Path, resp.Status)
		}
		return body, nil
	}

	form := url.Values{"user[email]": {email}, "user[password]": {password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, enlighten+"/login/login.json", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	body, err := get(req)
	if err != nil {
		return nil, err
	}
	var login struct {
		SystemID int64 `json:"system_id"`
	}
	if err := json.Unmarshal(body, &login); err != nil || login.SystemID == 0 {
		return nil, fmt.Errorf("login: no system_id in response")
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/pv/systems/%d/array_layout_x.json", enlighten, login.SystemID), nil)
	if err != nil {
		return nil, err
	}
	return get(req)
}

func arrayBySerial(body []byte) (map[string]string, error) {
	var l layout
	if err := json.Unmarshal(body, &l); err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, a := range l.Arrays {
		for _, mod := range a.Modules {
			m[mod.Inverter.SerialNum] = a.Label
		}
	}
	return m, nil
}
