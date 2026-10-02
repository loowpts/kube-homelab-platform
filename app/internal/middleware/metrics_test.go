package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsUsesRoutePatternAndStatus(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Metrics)
	r.Get("/api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	before := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/api/tasks/{id}", "404"))
	for _, id := range []string{"1", "2", "3"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/tasks/"+id, nil))
	}
	after := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/api/tasks/{id}", "404"))

	// Three different ids end up in one series: the label is the pattern, not the path.
	if got := after - before; got != 3 {
		t.Errorf("requests counted = %v, want 3", got)
	}
}

func TestMetricsDefaultsStatusTo200(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Metrics)
	r.Get("/ok", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	before := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/ok", "200"))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ok", nil))

	if got := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/ok", "200")) - before; got != 1 {
		t.Errorf("requests counted = %v, want 1", got)
	}
}

func TestMetricsCountsRecoveredPanicAs500(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Metrics) // same order as in main.go: Metrics outside Recoverer
	r.Use(chimiddleware.Recoverer)
	r.Get("/boom", func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})

	before := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/boom", "500"))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))

	if got := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/boom", "500")) - before; got != 1 {
		t.Errorf("500 counted = %v, want 1", got)
	}
}
