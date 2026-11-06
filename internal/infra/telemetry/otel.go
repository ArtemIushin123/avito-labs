package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func Setup(ctx context.Context, logger *slog.Logger, version string) (func(context.Context) error, error) {
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Warn("opentelemetry export failed", "error", err)
	}))

	res, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attribute.String("service.version", version)),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, fmt.Errorf("build telemetry resource: %w", err)
	}

	spanExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("create span exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(spanExporter),
		sdktrace.WithResource(res),
	)

	metricReader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("create metric reader: %w", err),
			tracerProvider.Shutdown(ctx),
		)
	}
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(metricReader),
		sdkmetric.WithResource(res),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	shutdown := func(ctx context.Context) error {
		return errors.Join(
			tracerProvider.Shutdown(ctx),
			meterProvider.Shutdown(ctx),
		)
	}
	return shutdown, nil
}
