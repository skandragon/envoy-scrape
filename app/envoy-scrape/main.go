package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
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

var knownInverters = map[string]inverterReport{}

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

//const submitURL = "https://iot.flame.org/api/v1/envoy/inverters"
const submitURL = "http://localhost:3000/api/v1/envoy/inverters"

func sendUpdate(serial string, i []inverterReport) {
	sub := submission{serial, i}
	data, err := json.Marshal(sub)
	if err != nil {
		log.Printf("%v", err)
		return
	}
	resp, err := http.Post(submitURL, "application/json", bytes.NewBuffer(data))
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
	username := flag.String("username", "installer", "username to connect to the Envoy")
	serial := flag.String("serial", "", "serial number of the Envoy")
	host := flag.String("host", "", "the hostname or IP address of the Envoy")

	flag.Parse()

	if *serial == "" || *host == "" {
		flag.Usage()
		os.Exit(-1)
	}

	password := makePasswordForSerial(*serial, *username)

	envoyURL := fmt.Sprintf("http://%s", *host)

	first := true
	for {
		if !first {
			time.Sleep(time.Minute)
		}

		_, body, err := digestGet(*username, password, envoyURL+"/api/v1/production/inverters")
		if err != nil {
			log.Printf("Fetching error: %v", err)
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
			sendUpdate(*serial, updatedInverters)
		}

		first = false
		log.Printf("fetch complete")
	}
}