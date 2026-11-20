package positions

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"

	"github.com/ArtemIushin123/template/internal/model"
)

type fakeTicker struct {
	ticks   chan time.Time
	stopped chan struct{}
}

func newFakeTicker() *fakeTicker {
	return &fakeTicker{ticks: make(chan time.Time, 1), stopped: make(chan struct{})}
}

func (t *fakeTicker) Ticks() <-chan time.Time {
	return t.ticks
}

func (t *fakeTicker) Stop() {
	close(t.stopped)
}

func TestScannerSerialTicksAndEnqueueResults(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const staleAfter = 45 * time.Second
		queue, stats := testQueue(t, 1)
		tick := newFakeTicker()
		trips := []model.StaleTrip{{TripID: uuid.New()}, {TripID: uuid.New()}, {TripID: uuid.New()}}
		calls := make(chan time.Duration, 3)
		unblock := make(chan struct{})
		source := NewMockstaleTripSource(gomock.NewController(t))
		source.EXPECT().ListStaleTrips(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, threshold time.Duration) ([]model.StaleTrip, error) {
			calls <- threshold
			select {
			case <-unblock:
				return trips, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}).Times(2)
		spans := tracetest.NewSpanRecorder()
		provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
		scanner := startScanner(t.Context(), queue, source, slog.New(slog.NewTextHandler(io.Discard, nil)),
			provider.Tracer("test"), tick, staleAfter)
		defer scanner.cancel()
		synctest.Wait()
		if len(calls) != 0 {
			t.Fatal("scanner queried before the first tick")
		}

		tick.ticks <- time.Now()
		synctest.Wait()
		if len(calls) != 1 {
			t.Fatalf("query calls = %d, want 1", len(calls))
		}
		tick.ticks <- time.Now()
		synctest.Wait()
		if len(calls) != 1 {
			t.Fatal("second query started before the first completed")
		}

		close(unblock)
		synctest.Wait()
		if len(calls) != 2 {
			t.Fatalf("query calls = %d, want 2", len(calls))
		}
		for range 2 {
			if got := <-calls; got != staleAfter {
				t.Errorf("staleness threshold = %s, want %s", got, staleAfter)
			}
		}
		if stats.results[string(Queued)] != 1 || stats.results[string(Duplicate)] != 1 || stats.results[string(Dropped)] != 4 {
			t.Fatalf("two ticks recorded %v, want queued=1, duplicate=1, dropped=4", stats.results)
		}
		if err := scanner.Shutdown(t.Context()); err != nil {
			t.Fatal(err)
		}
		<-tick.stopped

		item, ok := queue.next(t.Context())
		ended := spans.Ended()
		if !ok || item.trip != trips[0] || len(ended) != 2 || !item.checkSpan.Equal(ended[0].SpanContext()) {
			t.Fatalf("unexpected first task or trace: item=%+v, spans=%d", item, len(ended))
		}
	})
}

func TestScannerStopsInFlightQuery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue, _ := testQueue(t, 1)
		tick := newFakeTicker()
		entered := make(chan struct{})
		exited := make(chan struct{})
		source := NewMockstaleTripSource(gomock.NewController(t))
		source.EXPECT().ListStaleTrips(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ time.Duration) ([]model.StaleTrip, error) {
			close(entered)
			<-ctx.Done()
			close(exited)
			return []model.StaleTrip{{TripID: uuid.New()}}, nil
		}).Times(1)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		scanner := startScanner(ctx, queue, source, slog.New(slog.NewTextHandler(io.Discard, nil)),
			noop.NewTracerProvider().Tracer("test"), tick, time.Minute)
		defer scanner.cancel()
		tick.ticks <- time.Now()
		<-entered
		cancel()
		<-scanner.done

		<-exited
		<-tick.stopped
		if len(queue.tasks) != 0 {
			t.Fatal("scanner enqueued work after its context was cancelled")
		}
	})
}

func TestScannerContinuesAfterQueryError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		queue, _ := testQueue(t, 1)
		tick := newFakeTicker()
		spans := tracetest.NewSpanRecorder()
		provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		var calls int
		source := NewMockstaleTripSource(gomock.NewController(t))
		source.EXPECT().ListStaleTrips(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, time.Duration) ([]model.StaleTrip, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("query failed")
			}
			return []model.StaleTrip{{TripID: uuid.New()}}, nil
		}).Times(2)
		scanner := startScanner(t.Context(), queue, source, logger, provider.Tracer("test"), tick, time.Minute)
		defer scanner.cancel()
		tick.ticks <- time.Now()
		synctest.Wait()
		tick.ticks <- time.Now()
		synctest.Wait()
		if err := scanner.Shutdown(t.Context()); err != nil {
			t.Fatal(err)
		}
		ended := spans.Ended()
		if len(queue.tasks) != 1 || len(ended) != 2 || ended[0].Status().Code != codes.Error || !strings.Contains(logs.String(), "level=WARN") {
			t.Fatalf("query recovery: queued=%d, spans=%d, logs=%q", len(queue.tasks), len(ended), logs.String())
		}
	})
}
