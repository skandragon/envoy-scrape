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
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/skandragon/gohealthcheck/health"
)

// App holds application shared state.
type App struct {
	healthchecker *health.Health
	db            influxdb2.Client
	secret        string
}

func loggingMiddleware(next http.Handler) http.Handler {
	return handlers.LoggingHandler(os.Stdout, next)
}

func (a *App) routes(mux *mux.Router) {
	mux.Use(loggingMiddleware)

	mux.HandleFunc("/health", a.healthchecker.HTTPHandler()).Methods(http.MethodGet)
	mux.HandleFunc("/api/v1/inverters", a.envoyReceive).Methods(http.MethodPost)
}

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

func (a *App) envoyReceive(w http.ResponseWriter, req *http.Request) {
	foundSecret := req.Header.Get("x-flameorg-auth")
	if foundSecret != a.secret {
		http.Error(w, "Forbidden", http.StatusForbidden)
		log.Println("Forbidden")
		return
	}

	w.Header().Set("content-type", "application/json")

	defer req.Body.Close()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		log.Printf("While reading body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	sub := submission{}
	err = json.Unmarshal(body, &sub)
	if err != nil {
		log.Printf("While cracking json open: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	a.process(sub)
	w.WriteHeader(http.StatusOK)
}

func (a *App) process(sub submission) {
	writeAPI := a.db.WriteAPI("flame", "envoy")
	errorsCh := writeAPI.Errors()
	go func() {
		for err := range errorsCh {
			log.Printf("influx write error: %v", err)
		}
	}()

	for _, i := range sub.Inverters {
		tags := map[string]string{
			"envoySerial": sub.EnvoySerial,
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

func main() {
	listenAddress := flag.String("listenAddress", ":3100", "listen on this address/port")

	flag.Parse()

	influxToken, found := os.LookupEnv("ENVOY_INFLUX_TOKEN")
	if !found {
		log.Fatal("ENVOY_INFLUX_TOKEN envar not set")
	}

	influxURL, found := os.LookupEnv("ENVOY_INFLUX_URL")
	if !found {
		log.Fatal("ENVOY_INFLUX_URL envar not set")
	}

	secret, found := os.LookupEnv("ENVOY_RECEIVER_SECRET")
	if !found {
		log.Fatal("ENVOY_RECEIVER_SECRET envar not set")
	}

	dbopt := influxdb2.DefaultOptions().SetBatchSize(20)
	app := &App{
		healthchecker: health.MakeHealth(),
		db:            influxdb2.NewClientWithOptions(influxURL, influxToken, dbopt),
		secret:        secret,
	}
	go app.healthchecker.RunCheckers(15)

	m := mux.NewRouter()
	app.routes(m)

	srv := &http.Server{
		Addr:         *listenAddress,
		Handler:      m,
		ReadTimeout:  5 * time.Second,
		IdleTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	log.Printf("Starting HTTP listener on %s", *listenAddress)
	log.Fatal(srv.ListenAndServe())
}
