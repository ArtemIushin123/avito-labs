package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

var queryCollection = regexp.MustCompile(`(?i)^\s*(?:INSERT\s+INTO|UPDATE|DELETE\s+FROM|SELECT\s+.+?\s+FROM)\s+([a-z_][a-z_0-9]*)\b`)

type queryTracer struct {
	*otelpgx.Tracer
	duration metric.Float64Histogram
}

type queryMeasurementKey struct{}

type queryMeasurement struct {
	started    time.Time
	operation  string
	collection string
}

func newQueryTracer(tracerProvider trace.TracerProvider, meterProvider metric.MeterProvider) (*queryTracer, error) {
	meter := meterProvider.Meter("trip-service/postgres")
	duration, err := meter.Float64Histogram("db.client.operation.duration",
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10),
	)
	if err != nil {
		return nil, fmt.Errorf("create query duration histogram: %w", err)
	}
	return &queryTracer{
		Tracer: otelpgx.NewTracer(
			otelpgx.WithTracerProvider(tracerProvider),
			otelpgx.WithMeterProvider(noop.NewMeterProvider()),
		),
		duration: duration,
	}, nil
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	measurement := queryMeasurement{
		started:   time.Now(),
		operation: otelpgx.SQLOperationName(ctx, data.SQL),
	}
	if match := queryCollection.FindStringSubmatch(data.SQL); len(match) > 0 {
		measurement.collection = strings.ToLower(match[1])
	}

	ctx = t.Tracer.TraceQueryStart(ctx, conn, data)
	if measurement.collection != "" {
		trace.SpanFromContext(ctx).SetAttributes(attribute.String("db.collection.name", measurement.collection))
	}
	return context.WithValue(ctx, queryMeasurementKey{}, measurement)
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	measurement, ok := ctx.Value(queryMeasurementKey{}).(queryMeasurement)
	if ok {
		attrs := []attribute.KeyValue{attribute.String("db.system.name", "postgresql")}
		if measurement.operation != "" {
			attrs = append(attrs, attribute.String("db.operation.name", measurement.operation))
		}
		if measurement.collection != "" {
			attrs = append(attrs, attribute.String("db.collection.name", measurement.collection))
		}
		if data.Err != nil {
			errorType := attribute.String("error.type", queryErrorType(data.Err))
			attrs = append(attrs, errorType)
			trace.SpanFromContext(ctx).SetAttributes(errorType)
		}
		t.duration.Record(ctx, time.Since(measurement.started).Seconds(), metric.WithAttributes(attrs...))
	}

	t.Tracer.TraceQueryEnd(ctx, conn, data)
}

func queryErrorType(err error) string {
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr):
		return pgErr.Code
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return fmt.Sprintf("%T", err)
	}
}
