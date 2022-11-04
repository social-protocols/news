package main

import (
	"log"
	"net/http"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"github.com/johnwarden/httperror/v2"
)

// Register various metrics.
// Metric name may contain labels in Prometheus format - see below.

var (
	generateFrontpageErrorsTotal = metrics.NewCounter(`errors_total{type="generateFrontpage"}`)
	updateQNRanksErrorsTotal     = metrics.NewCounter(`errors_total{type="updateQNRanks"}`)
	crawlErrorsTotal             = metrics.NewCounter(`errors_total{type="crawl"}`)
	requestErrorsTotal           = metrics.NewCounter(`errors_total{type="request"}`)
	crawlDuration                = metrics.NewHistogram("crawl_duration_seconds")

	upvotesTotal = metrics.NewCounter(`upvotes`)
)

var generateFrontpageMetrics map[string]*metrics.Histogram

func init() {
	generateFrontpageMetrics = make(map[string]*metrics.Histogram)
	for _, ranking := range []string{"hntop", "quality"} {
		generateFrontpageMetrics[ranking] = metrics.NewHistogram(`generate_frontpage_duration_seconds{ranking="` + ranking + `"}`)
	}
}

func servePrometheusMetrics() {
	mux := http.NewServeMux()

	// Export all the registered metrics in Prometheus format at `/metrics` http path.
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, req *http.Request) {
		metrics.WritePrometheus(w, true)
	})

	log.Fatal(http.ListenAndServe(":9091", mux))
}

func prometheusMiddleware[P any](routeName string, h httperror.XHandler[P]) httperror.XHandlerFunc[P] {
	// Register summary with a single label.
	requestDuration := metrics.NewHistogram(`requests_duration_seconds{route="` + routeName + `"}`)

	return func(w http.ResponseWriter, r *http.Request, p P) error {
		startTime := time.Now()

		err := h.Serve(w, r, p)

		requestDuration.UpdateDuration(startTime)

		return err
	}
}
