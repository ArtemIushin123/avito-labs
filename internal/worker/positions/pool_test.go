package positions

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/ArtemIushin123/template/internal/infra/telemetry"
	"github.com/ArtemIushin123/template/internal/model"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func workerMetrics(t *testing.T) (*telemetry.PositionRequests, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events, err := telemetry.NewPositionRequests(logger, provider.Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	return events, reader
}

func activeWorkers(t *testing.T, reader *sdkmetric.ManualReader) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == "tripgo.position_requests.workers.active" {
				data, ok := metric.Data.(metricdata.Gauge[int64])
				if ok && len(data.DataPoints) == 1 {
					return data.DataPoints[0].Value
				}
				t.Fatalf("unexpected active worker gauge: %+v", metric.Data)
			}
		}
	}
	t.Fatal("active worker gauge is missing")
	return -1
}

func TestPoolLimitsConcurrencyAndDrains(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const workers, tasks = 3, 8
		events, reader := workerMetrics(t)
		queue := NewQueue(tasks, events)
		want := make(map[uuid.UUID]bool)
		for range tasks {
			trip := model.StaleTrip{TripID: uuid.New()}
			want[trip.TripID] = true
			if got := queue.Enqueue(t.Context(), trip); got != Queued {
				t.Fatalf("enqueue = %v", got)
			}
		}
		received := make(chan uuid.UUID, tasks)
		release := make(chan struct{})
		gateway := NewMockpushGateway(gomock.NewController(t))
		gateway.EXPECT().RequestPosition(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, trip model.StaleTrip) error {
			received <- trip.TripID
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}).Times(8)
		pool := StartPool(t.Context(), queue, gateway, events, slog.New(slog.DiscardHandler), noop.NewTracerProvider().Tracer("test"), workers, time.Second)
		defer pool.cancel()
		synctest.Wait()
		if len(received) != workers {
			t.Fatalf("running calls = %d, want %d", len(received), workers)
		}
		if got := activeWorkers(t, reader); got != workers {
			t.Errorf("active worker metric = %d, want %d", got, workers)
		}

		firstTrip := <-received
		received <- firstTrip
		queue.Cancel(firstTrip)
		if got := queue.Enqueue(t.Context(), model.StaleTrip{TripID: firstTrip}); got != Duplicate {
			t.Fatalf("coordinate released task during send: %v", got)
		}

		close(release)
		if err := pool.Shutdown(t.Context()); err != nil {
			t.Fatal(err)
		}
		close(received)
		for id := range received {
			if !want[id] {
				t.Errorf("unexpected or repeated call for %s", id)
			}
			delete(want, id)
		}
		if len(want) != 0 {
			t.Errorf("%d tasks were not processed", len(want))
		}
		if got := activeWorkers(t, reader); got != 0 {
			t.Errorf("active worker metric after shutdown = %d", got)
		}
	})
}

func TestPoolContinuesAfterFailureAndLinksSpans(t *testing.T) {
	events, _ := workerMetrics(t)
	queue := NewQueue(3, events)
	spans := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	tracer := provider.Tracer("test")
	checkCtx, checkSpan := tracer.Start(t.Context(), "stale check")
	failed, cancelled, last := uuid.New(), uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{failed, cancelled, last} {
		queue.Enqueue(checkCtx, model.StaleTrip{TripID: id})
	}
	checkSpan.End()
	queue.Cancel(cancelled)
	var calls []uuid.UUID
	gateway := NewMockpushGateway(gomock.NewController(t))
	gateway.EXPECT().RequestPosition(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, trip model.StaleTrip) error {
		calls = append(calls, trip.TripID)
		if trip.TripID == failed {
			return errors.New("push unavailable")
		}
		return nil
	}).Times(2)
	pool := StartPool(t.Context(), queue, gateway, events, slog.New(slog.DiscardHandler), tracer, 1, time.Second)
	defer pool.cancel()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := pool.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls, []uuid.UUID{failed, last}) {
		t.Fatalf("calls = %v, want failed and last only", calls)
	}
	ended := spans.Ended()
	if len(ended) != 3 {
		t.Fatalf("ended spans = %d, want check and two tasks", len(ended))
	}
	for _, span := range ended[1:] {
		links := span.Links()
		if span.Parent().IsValid() || len(links) != 1 || !links[0].SpanContext.Equal(checkSpan.SpanContext()) {
			t.Fatalf("task span parent = %v, links = %v", span.Parent(), links)
		}
	}
	if ended[1].Status().Code != codes.Error || ended[2].Status().Code == codes.Error {
		t.Fatalf("unexpected worker span statuses: %v and %v", ended[1].Status(), ended[2].Status())
	}
}

func TestPoolTaskTimeoutAndContextCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue, _ := testQueue(t, 1)
		events, _ := workerMetrics(t)
		queue.Enqueue(t.Context(), model.StaleTrip{TripID: uuid.New()})
		ended := make(chan error, 1)
		gateway := NewMockpushGateway(gomock.NewController(t))
		gateway.EXPECT().RequestPosition(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ model.StaleTrip) error {
			<-ctx.Done()
			ended <- ctx.Err()
			return ctx.Err()
		}).Times(1)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		pool := StartPool(ctx, queue, gateway, events, slog.New(slog.DiscardHandler), noop.NewTracerProvider().Tracer("test"), 1, 5*time.Second)
		defer pool.cancel()
		started := time.Now()
		if err := <-ended; !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 5*time.Second {
			t.Fatalf("attempt ended with %v after %s", err, time.Since(started))
		}
		cancel()
		<-pool.done
	})
}

func TestPoolCancelsInFlightOnShutdownDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue, _ := testQueue(t, 1)
		events, _ := workerMetrics(t)
		queue.Enqueue(t.Context(), model.StaleTrip{TripID: uuid.New()})
		entered := make(chan struct{})
		ended := make(chan error, 1)
		gateway := NewMockpushGateway(gomock.NewController(t))
		gateway.EXPECT().RequestPosition(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ model.StaleTrip) error {
			close(entered)
			<-ctx.Done()
			ended <- ctx.Err()
			return ctx.Err()
		}).Times(1)
		pool := StartPool(t.Context(), queue, gateway, events, slog.New(slog.DiscardHandler), noop.NewTracerProvider().Tracer("test"), 1, time.Hour)
		defer pool.cancel()
		<-entered
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		if err := pool.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("shutdown = %v", err)
		}
		if err := <-ended; !errors.Is(err, context.Canceled) {
			t.Fatalf("in-flight request error = %v", err)
		}
		<-pool.done
	})
}
