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
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

type inverterReport struct {
	SerialNumber    string `json:"serialNumber,omitempty"`
	LastReportDate  int    `json:"lastReportDate,omitempty"`
	DevType         int    `json:"devType,omitempty"`
	LastReportWatts int    `json:"lastReportWatts,omitempty"`
	MaxReportWatts  int    `json:"maxReportWatts,omitempty"`
}

var (
	serial      = flag.String("serial", "", "serial number of the Envoy")
	host        = flag.String("host", "", "the hostname or IP address of the Envoy")
	influxToken = flag.String("influxToken", "", "the token for InfluxDB")
	influxURL   = flag.String("influxURL", "http://10.45.220.3:8086", "the URL for InfluxDB")

	knownInverters = map[string]inverterReport{}
	envoyClient    *http.Client
	db             influxdb2.Client
)

func updateInverters(i inverterReport) bool {
	old, found := knownInverters[i.SerialNumber]
	if !found {
		knownInverters[i.SerialNumber] = i
		return true
	}
	if old.LastReportDate == i.LastReportDate {
		return false
	}
	knownInverters[i.SerialNumber] = i
	return true
}

func main() {
	ctx := context.Background()

	envoyClient = makeClient()

	flag.Parse()

	if *serial == "" {
		*serial = os.Getenv("ENVOY_SERIAL")
	}
	if *host == "" {
		*host = os.Getenv("ENVOY_HOST")
	}

	x, found := os.LookupEnv("ENVOY_INFLUX_TOKEN")
	if found {
		*influxToken = x
	}

	u, found := os.LookupEnv("ENVOY_INFLUX_URL")
	if found {
		*influxURL = u
	}

	if *serial == "" || *host == "" || *influxToken == "" {
		flag.Usage()
		os.Exit(-1)
	}

	token, set := os.LookupEnv("ENVOY_TOKEN")
	if !set {
		log.Fatalf("ENVOY_TOKEN is not set")
	}

	dbopt := influxdb2.DefaultOptions().SetBatchSize(20)
	db = influxdb2.NewClientWithOptions(*influxURL, *influxToken, dbopt)

	envoyURL := fmt.Sprintf("https://%s", *host)

	first := true
	for {
		if !first {
			time.Sleep(time.Minute)
		}

		body, err := makeRequest(ctx, token, envoyURL+"/api/v1/production/inverters")
		if err != nil {
			log.Printf("%v", err)
			first = false
			continue
		}

		inverters := []inverterReport{}
		err = json.Unmarshal(body, &inverters)
		if err != nil {
			log.Printf("%v", err)
			first = false
			continue
		}

		updatedInverters := []inverterReport{}

		for _, inverter := range inverters {
			updated := updateInverters(inverter)
			if updated && !first {
				updatedInverters = append(updatedInverters, inverter)
			}
		}

		if len(updatedInverters) > 0 {
			process(*serial, updatedInverters)
		}

		first = false
		log.Printf("fetch complete.  %d inverters, %d updates", len(inverters), len(updatedInverters))
	}
}

func makeClient() *http.Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &http.Client{Transport: tr}
}

func makeRequest(ctx context.Context, token string, address string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := envoyClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return content, nil
}

func process(serial string, items []inverterReport) {
	writeAPI := db.WriteAPI("flame", "envoy")
	errorsCh := writeAPI.Errors()
	go func() {
		for err := range errorsCh {
			log.Printf("influx write error: %v", err)
		}
	}()

	for _, i := range items {
		tags := map[string]string{
			"envoySerial": serial,
			"serial":      i.SerialNumber,
			"type":        fmt.Sprintf("%d", i.DevType),
		}
		fields := map[string]interface{}{
			"power":    i.LastReportWatts,
			"maxPower": i.MaxReportWatts,
		}
		ts := time.Unix(int64(i.LastReportDate), 0)
		p := influxdb2.NewPoint("inverterPower", tags, fields, ts)
		writeAPI.WritePoint(p)
	}

	writeAPI.Flush()
}
