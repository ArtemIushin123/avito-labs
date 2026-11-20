package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type PositionRequests struct {
	logger        *slog.Logger
	total         metric.Int64Counter
	duration      metric.Float64Histogram
	staleChecks   metric.Int64Counter
	queueSize     metric.Int64Gauge
	workersActive atomic.Int64
}

func NewPositionRequests(logger *slog.Logger, meter metric.Meter) (*PositionRequests, error) {
	total, err := meter.Int64Counter("tripgo.position_requests", metric.WithUnit("{request}"))
	if err != nil {
		return nil, fmt.Errorf("create position request counter: %w", err)
	}
	duration, err := meter.Float64Histogram("tripgo.position_requests.duration", metric.WithUnit("s"))
	if err != nil {
		return nil, fmt.Errorf("create position request duration histogram: %w", err)
	}
	staleChecks, err := meter.Int64Counter("tripgo.stale_checks", metric.WithUnit("{trip}"))
	if err != nil {
		return nil, fmt.Errorf("create stale check counter: %w", err)
	}
	queueSize, err := meter.Int64Gauge("tripgo.position_requests.queue.size", metric.WithUnit("{task}"))
	if err != nil {
		return nil, fmt.Errorf("create position queue gauge: %w", err)
	}
	requests := &PositionRequests{logger: logger, total: total, duration: duration, staleChecks: staleChecks, queueSize: queueSize}
	_, err = meter.Int64ObservableGauge("tripgo.position_requests.workers.active",
		metric.WithUnit("{worker}"),
		metric.WithInt64Callback(func(_ context.Context, observer metric.Int64Observer) error {
			observer.Observe(requests.workersActive.Load())
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create active position workers gauge: %w", err)
	}
	return requests, nil
}

func (m *PositionRequests) WorkersActive(delta int64) {
	m.workersActive.Add(delta)
}

func (m *PositionRequests) Enqueued(ctx context.Context, result string) {
	m.staleChecks.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
	if result == "dropped" {
		m.logger.WarnContext(ctx, "position request dropped")
	}
}

func (m *PositionRequests) QueueSize(ctx context.Context, size int64) {
	m.queueSize.Record(ctx, size)
}

func (m *PositionRequests) Cancelled(ctx context.Context) {
	m.total.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "cancelled")))
}

func (m *PositionRequests) Finished(ctx context.Context, requestID string, elapsed time.Duration, err error) {
	result := "success"
	if err != nil {
		result = "error"
		m.logger.WarnContext(ctx, "position request failed", "request_id", requestID, "error", err)
	}
	attrs := metric.WithAttributes(attribute.String("result", result))
	m.total.Add(ctx, 1, attrs)
	m.duration.Record(ctx, elapsed.Seconds(), attrs)
}
