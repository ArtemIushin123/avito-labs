package telemetry

import (
	"context"
	"fmt"
	"sync/atomic"

	"go.opentelemetry.io/otel/metric"
)

type activeTripCounter interface {
	CountActive(ctx context.Context) (int64, error)
}

func ObserveActiveTrips(meter metric.Meter, trips activeTripCounter) (func(), error) {
	var stopped atomic.Bool
	_, err := meter.Int64ObservableGauge("tripgo.trips.active",
		metric.WithUnit("{trip}"),
		metric.WithInt64Callback(func(ctx context.Context, observer metric.Int64Observer) error {
			if stopped.Load() {
				return nil
			}
			count, err := trips.CountActive(ctx)
			if stopped.Load() {
				return nil
			}
			if err != nil {
				return err
			}
			observer.Observe(count)
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create active trip gauge: %w", err)
	}
	return func() { stopped.Store(true) }, nil
}
