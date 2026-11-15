package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestQueryTracer_RecordsCollectionAndErrorType(t *testing.T) {
	tests := []struct {
		name       string
		sql        string
		err        error
		operation  string
		collection string
		errorType  string
	}{
		{name: "read trip", sql: "SELECT id FROM trips WHERE id = $1", operation: "SELECT", collection: "trips"},
		{
			name:       "insert status error",
			sql:        "INSERT INTO trip_status_history (trip_id) VALUES ($1)",
			err:        &pgconn.PgError{Code: "23503", Message: "foreign key violation"},
			operation:  "INSERT",
			collection: "trip_status_history",
			errorType:  "23503",
		},
		{name: "commit has no collection", sql: "commit", operation: "COMMIT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spans := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			tracer, err := newQueryTracer(tp, mp)
			if err != nil {
				t.Fatalf("create tracer: %v", err)
			}
			ctx, parent := tp.Tracer("test").Start(t.Context(), "parent")
			defer parent.End()
			queryCtx := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: tt.sql})
			tracer.TraceQueryEnd(queryCtx, nil, pgx.TraceQueryEndData{Err: tt.err})

			ended := spans.Ended()
			if len(ended) != 1 || !ended[0].Parent().Equal(parent.SpanContext()) {
				t.Fatalf("want one finished SQL span under the parent, got %v", ended)
			}
			if tt.err != nil && ended[0].Status().Code != codes.Error {
				t.Errorf("SQL span status = %v, want Error", ended[0].Status())
			}

			spanAttrs := attribute.NewSet(ended[0].Attributes()...)
			metricAttrs := queryDurationAttributes(t, reader)
			for _, attrs := range []attribute.Set{spanAttrs, metricAttrs} {
				checkQueryAttribute(t, attrs, "db.operation.name", tt.operation)
				checkQueryAttribute(t, attrs, "db.collection.name", tt.collection)
				checkQueryAttribute(t, attrs, "error.type", tt.errorType)
			}
		})
	}
}

func queryDurationAttributes(t *testing.T, reader *sdkmetric.ManualReader) attribute.Set {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("collect query duration: %v", err)
	}
	if len(rm.ScopeMetrics) != 1 || len(rm.ScopeMetrics[0].Metrics) != 1 {
		t.Fatalf("want one metric without duplicate otelpgx measurements, got %+v", rm.ScopeMetrics)
	}
	metric := rm.ScopeMetrics[0].Metrics[0]
	hist, ok := metric.Data.(metricdata.Histogram[float64])
	if metric.Name != "db.client.operation.duration" || !ok || len(hist.DataPoints) != 1 || hist.DataPoints[0].Count != 1 {
		t.Fatalf("want one SQL duration sample, got %+v", metric)
	}
	return hist.DataPoints[0].Attributes
}

func checkQueryAttribute(t *testing.T, attrs attribute.Set, name, want string) {
	t.Helper()
	value, ok := attrs.Value(attribute.Key(name))
	if ok != (want != "") || value.AsString() != want {
		t.Errorf("attribute %q = %q (present: %v), want %q", name, value.AsString(), ok, want)
	}
}
