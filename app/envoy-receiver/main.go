package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/skandragon/gohealthcheck/health"
)

// App holds application state, if any
type App struct{}

var (
	healthchecker = health.MakeHealth()
)

func loggingMiddleware(next http.Handler) http.Handler {
	return handlers.LoggingHandler(os.Stdout, next)
}

func (a *App) routes(mux *mux.Router) {
	mux.Use(loggingMiddleware)

	mux.HandleFunc("/health", healthchecker.HTTPHandler()).Methods(http.MethodGet)
}

func main() {
	go healthchecker.RunCheckers(15)

	m := mux.NewRouter()
	app := &App{}
	app.routes(m)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", 3000),
		Handler: m,
	}

	log.Fatal(srv.ListenAndServe())
}
