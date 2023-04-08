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
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	flogger "github.com/gofiber/fiber/v2/middleware/logger"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/skandragon/gohealthcheck/health"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

// Server holds application shared state.
type Server struct {
	healthchecker *health.Health
	db            influxdb2.Client
	secret        string
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

func (a *Server) envoyReceive(c *fiber.Ctx) error {
	c.Accepts("application/json")
	c.Type("json", "utf-8")

	foundSecret := c.Get("x-flameorg-auth")
	if foundSecret != a.secret {
		return fiber.ErrForbidden
	}

	sub := submission{}
	if err := c.BodyParser(&sub); err != nil {
		return err
	}

	a.process(sub)
	return nil
}

func (a *Server) process(sub submission) {
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

func WrapHandler(f http.HandlerFunc) func(ctx *fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		fasthttpadaptor.NewFastHTTPHandler(f)(ctx.Context())
		return nil
	}
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
	server := &Server{
		healthchecker: health.MakeHealth(),
		db:            influxdb2.NewClientWithOptions(influxURL, influxToken, dbopt),
		secret:        secret,
	}
	go server.healthchecker.RunCheckers(15)

	app := fiber.New(fiber.Config{
		ReadTimeout:  5 * time.Second,
		IdleTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	})

	app.Use(flogger.New())

	server.healthchecker.HTTPHandler()

	app.Get("/health", WrapHandler(server.healthchecker.HTTPHandler()))
	app.Post("/api/v1/inverters", server.envoyReceive)

	log.Printf("Starting HTTP listener on %s", *listenAddress)
	log.Fatal(app.Listen(*listenAddress))
}
