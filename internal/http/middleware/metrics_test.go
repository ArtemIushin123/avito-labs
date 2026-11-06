package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/ArtemIushin123/template/internal/http/middleware"
)

func newMeteredRouter(t *testing.T, h http.HandlerFunc) (http.Handler, *sdkmetric.ManualReader) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	httpMetrics, err := middleware.NewMetrics(provider.Meter("test"))
	if err != nil {
		t.Fatalf("NewMetrics() error = %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.Recover(discardLogger()))
	r.With(httpMetrics).Get(tripRoute, h)
	return r, reader
}

func collect(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	out := map[string]metricdata.Metrics{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			out[m.Name] = m
		}
	}
	return out
}

func attr(set attribute.Set, key string) string {
	v, _ := set.Value(attribute.Key(key))
	return v.String()
}

func TestMetrics(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus string
	}{
		{name: "ok", handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }, wantStatus: "200"},
		{name: "not found", handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }, wantStatus: "404"},
		{name: "panic", handler: func(http.ResponseWriter, *http.Request) { panic("boom") }, wantStatus: "500"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, reader := newMeteredRouter(t, tt.handler)

			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/trips/123", nil))

			metrics := collect(t, reader)

			hist, ok := metrics["http.server.request.duration"].Data.(metricdata.Histogram[float64])
			if !ok || len(hist.DataPoints) != 1 {
				t.Fatalf("http.server.request.duration: want one data point, got %+v", metrics["http.server.request.duration"])
			}
			dp := hist.DataPoints[0]
			if dp.Count != 1 {
				t.Errorf("duration count = %d, want 1", dp.Count)
			}
			if got := attr(dp.Attributes, "http.route"); got != tripRoute {
				t.Errorf("http.route = %q, want template %q, not the raw path", got, tripRoute)
			}
			if got := attr(dp.Attributes, "http.request.method"); got != http.MethodGet {
				t.Errorf("http.request.method = %q, want GET", got)
			}
			if got := attr(dp.Attributes, "http.response.status_code"); got != tt.wantStatus {
				t.Errorf("http.response.status_code = %q, want %s", got, tt.wantStatus)
			}
			if metrics["http.server.request.duration"].Unit != "s" {
				t.Errorf("unit = %q, want seconds", metrics["http.server.request.duration"].Unit)
			}
			sum, ok := metrics["http.server.active_requests"].Data.(metricdata.Sum[int64])
			if !ok || len(sum.DataPoints) != 1 {
				t.Fatalf("http.server.active_requests: want one data point, got %+v", metrics["http.server.active_requests"])
			}
			if sum.DataPoints[0].Value != 0 {
				t.Errorf("active_requests = %d after the request, want 0", sum.DataPoints[0].Value)
			}
			if got := attr(sum.DataPoints[0].Attributes, "http.route"); got != tripRoute {
				t.Errorf("active_requests http.route = %q, want %q", got, tripRoute)
			}
		})
	}
}
