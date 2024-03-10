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
	"bytes"
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
)

type inverterReport struct {
	SerialNumber    string `json:"serialNumber,omitempty"`
	LastReportDate  int    `json:"lastReportDate,omitempty"`
	DevType         int    `json:"devType,omitempty"`
	LastReportWatts int    `json:"lastReportWatts,omitempty"`
	MaxReportWatts  int    `json:"maxReportWatts,omitempty"`
}

type submission struct {
	EnvoySerial string           `json:"envoySerial,omitempty"`
	Inverters   []inverterReport `json:"inverters,omitempty"`
}

var (
	knownInverters = map[string]inverterReport{}
	envoyClient    *http.Client
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

func sendUpdate(url string, secret string, serial string, i []inverterReport) {
	sub := submission{serial, i}
	data, err := json.Marshal(sub)
	if err != nil {
		log.Printf("%v", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(data))
	if err != nil {
		log.Printf("%v", err)
		return
	}
	req.Header.Add("content-type", "application/json")
	req.Header.Add("x-flameorg-auth", secret)

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("%v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("Got status %d while sending inverter update", resp.StatusCode)
	}
}

func main() {
	ctx := context.Background()

	envoyClient = makeClient()

	serial := flag.String("serial", "", "serial number of the Envoy")
	host := flag.String("host", "", "the hostname or IP address of the Envoy")
	url := flag.String("url", "", "the URL to post data to")
	secret := flag.String("secret", "", "the secret used to post to the receiver")

	flag.Parse()

	if *serial == "" {
		*serial = os.Getenv("ENVOY_SERIAL")
	}
	if *host == "" {
		*host = os.Getenv("ENVOY_HOST")
	}
	if *url == "" {
		*url = os.Getenv("ENVOY_RECEIVER_URL")
	}
	if *secret == "" {
		*secret = os.Getenv("ENVOY_RECEIVER_SECRET")
	}

	if *serial == "" || *host == "" || *url == "" || *secret == "" {
		flag.Usage()
		os.Exit(-1)
	}

	token, set := os.LookupEnv("ENVOY_TOKEN")
	if !set {
		log.Fatalf("ENVOY_TOKEN is not set")
	}

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
			sendUpdate(*url, *secret, *serial, updatedInverters)
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
