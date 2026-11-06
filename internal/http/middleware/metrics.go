package middleware

import (
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func NewMetrics(meter metric.Meter) (func(http.Handler) http.Handler, error) {
	duration, err := meter.Float64Histogram("http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Длительность обработки HTTP-запроса"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10),
	)
	if err != nil {
		return nil, fmt.Errorf("create http.server.request.duration: %w", err)
	}

	active, err := meter.Int64UpDownCounter("http.server.active_requests",
		metric.WithUnit("{request}"),
		metric.WithDescription("Запросы, которые обрабатываются прямо сейчас"),
	)
	if err != nil {
		return nil, fmt.Errorf("create http.server.active_requests: %w", err)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			method := attribute.String("http.request.method", r.Method)
			route := attribute.String("http.route", routePattern(r))

			active.Add(ctx, 1, metric.WithAttributes(method, route))
			rec := &statusRecorder{ResponseWriter: w}
			start := time.Now()
			finished := false

			defer func() {
				active.Add(ctx, -1, metric.WithAttributes(method, route))

				status := rec.code()
				if !finished {
					status = http.StatusInternalServerError
				}
				duration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(
					method, route, attribute.Int("http.response.status_code", status),
				))
			}()

			next.ServeHTTP(rec, r)
			finished = true
		})
	}, nil
}
