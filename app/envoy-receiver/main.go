package main

import (
	"encoding/json"
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
}

func loggingMiddleware(next http.Handler) http.Handler {
	return handlers.LoggingHandler(os.Stdout, next)
}

func (a *App) routes(mux *mux.Router) {
	mux.Use(loggingMiddleware)

	mux.HandleFunc("/health", a.healthchecker.HTTPHandler()).Methods(http.MethodGet)
	mux.HandleFunc("/api/v1/envoy/inverters", a.envoyReceive).Methods(http.MethodPost)
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
	listenAddress := ":3000"

	dbopt := influxdb2.DefaultOptions().SetBatchSize(20)
	token := "***REMOVED***"
	app := &App{
		healthchecker: health.MakeHealth(),
		db:            influxdb2.NewClientWithOptions("http://localhost:8086", token, dbopt),
	}
	go app.healthchecker.RunCheckers(15)

	m := mux.NewRouter()
	app.routes(m)

	srv := &http.Server{
		Addr:    listenAddress,
		Handler: m,
	}

	log.Printf("Starting HTTP listener on %s", listenAddress)
	log.Fatal(srv.ListenAndServe())
}
