package middleware

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

func Tracing() func(http.Handler) http.Handler {
	traced := otelhttp.NewMiddleware("http.server",
		otelhttp.WithMeterProvider(noop.NewMeterProvider()),
	)

	return func(next http.Handler) http.Handler {
		return traced(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)

			route := routePattern(r)
			span := trace.SpanFromContext(r.Context())
			if route == unmatchedRoute {
				span.SetName(r.Method)
				return
			}
			span.SetName(r.Method + " " + route)
			span.SetAttributes(attribute.String("http.route", route))
		}))
	}
}
