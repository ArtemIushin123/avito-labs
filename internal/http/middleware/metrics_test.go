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
	r.Use(middleware.Recover)
	r.Use(httpMetrics)
	r.Get(tripRoute, h)
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
	const tripPath = "/api/v1/trips/123"

	tests := []struct {
		name       string
		method     string
		path       string
		handler    http.HandlerFunc
		wantStatus string
		wantRoute  string
		wantMethod string
	}{
		{
			name: "ok", method: http.MethodGet, path: tripPath,
			handler:    func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
			wantStatus: "200", wantRoute: tripRoute, wantMethod: http.MethodGet,
		},
		{
			name: "client error on known route", method: http.MethodGet, path: "/api/v1/trips/not-a-uuid",
			handler:    func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadRequest) },
			wantStatus: "400", wantRoute: tripRoute, wantMethod: http.MethodGet,
		},
		{
			name: "not found path", method: http.MethodGet, path: "/no/such/path",
			wantStatus: "404", wantRoute: "unmatched", wantMethod: http.MethodGet,
		},
		{
			name: "unsupported method on known path", method: http.MethodPut, path: tripPath,
			wantStatus: "405", wantRoute: tripRoute, wantMethod: http.MethodPut,
		},
		{
			name: "arbitrary method on known path", method: "BREW", path: tripPath,
			wantStatus: "405", wantRoute: tripRoute, wantMethod: "OTHER",
		},
		{
			name: "panic", method: http.MethodGet, path: tripPath,
			handler:    func(http.ResponseWriter, *http.Request) { panic("boom") },
			wantStatus: "500", wantRoute: tripRoute, wantMethod: http.MethodGet,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, reader := newMeteredRouter(t, tt.handler)

			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(tt.method, tt.path, nil))

			metrics := collect(t, reader)

			hist, ok := metrics["http.server.request.duration"].Data.(metricdata.Histogram[float64])
			if !ok || len(hist.DataPoints) != 1 {
				t.Fatalf("http.server.request.duration: want one data point, got %+v", metrics["http.server.request.duration"])
			}
			dp := hist.DataPoints[0]
			if dp.Count != 1 {
				t.Errorf("duration count = %d, want 1", dp.Count)
			}
			if got := attr(dp.Attributes, "http.route"); got != tt.wantRoute {
				t.Errorf("http.route = %q, want %q", got, tt.wantRoute)
			}
			if got := attr(dp.Attributes, "http.request.method"); got != tt.wantMethod {
				t.Errorf("http.request.method = %q, want %q", got, tt.wantMethod)
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
			if got := attr(sum.DataPoints[0].Attributes, "http.route"); got != tt.wantRoute {
				t.Errorf("active_requests http.route = %q, want %q", got, tt.wantRoute)
			}
			if got := attr(sum.DataPoints[0].Attributes, "http.request.method"); got != tt.wantMethod {
				t.Errorf("active_requests http.request.method = %q, want %q", got, tt.wantMethod)
			}
		})
	}
}
