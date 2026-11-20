package positions

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"

	"github.com/ArtemIushin123/template/internal/model"
)

type queueRecorder struct {
	mu        sync.Mutex
	results   map[string]int
	size      int64
	cancelled int
}

func testQueue(t *testing.T, size int) (*Queue, *queueRecorder) {
	t.Helper()
	recorder := &queueRecorder{results: make(map[string]int)}
	events := NewMockqueueEvents(gomock.NewController(t))
	events.EXPECT().Enqueued(gomock.Any(), gomock.Any()).Do(func(_ context.Context, result string) {
		recorder.mu.Lock()
		recorder.results[result]++
		recorder.mu.Unlock()
	}).AnyTimes()
	events.EXPECT().QueueSize(gomock.Any(), gomock.Any()).Do(func(_ context.Context, size int64) {
		recorder.mu.Lock()
		recorder.size = size
		recorder.mu.Unlock()
	}).AnyTimes()
	events.EXPECT().Cancelled(gomock.Any()).Do(func(context.Context) {
		recorder.mu.Lock()
		recorder.cancelled++
		recorder.mu.Unlock()
	}).AnyTimes()
	return NewQueue(size, events), recorder
}

func TestQueueDuplicateAndComplete(t *testing.T) {
	q, stats := testQueue(t, 1)
	trip := model.StaleTrip{TripID: uuid.New(), DriverID: uuid.New()}
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}})
	ctx := trace.ContextWithSpanContext(t.Context(), sc)
	if got := q.Enqueue(ctx, trip); got != Queued {
		t.Fatalf("enqueue = %s", got)
	}
	if got := q.Enqueue(ctx, trip); got != Duplicate {
		t.Fatalf("repeated enqueue = %s", got)
	}
	item, ok := q.next(ctx)
	if !ok || item.trip != trip || !item.checkSpan.Equal(sc) {
		t.Fatalf("unexpected task: %+v", item)
	}
	if got := q.Enqueue(ctx, trip); got != Duplicate {
		t.Fatalf("enqueue during processing = %s", got)
	}
	q.complete(item)
	if got := q.Enqueue(ctx, trip); got != Queued {
		t.Fatalf("enqueue after completion = %s", got)
	}
	if stats.results[string(Queued)] != 2 || stats.results[string(Duplicate)] != 2 || stats.size != 1 {
		t.Fatalf("unexpected queue metrics: %+v", stats)
	}
}

func TestQueueOverflowDoesNotBlock(t *testing.T) {
	q, _ := testQueue(t, 1)
	first := model.StaleTrip{TripID: uuid.New()}
	second := model.StaleTrip{TripID: uuid.New()}
	q.Enqueue(t.Context(), first)
	done := make(chan EnqueueResult, 1)
	go func() { done <- q.Enqueue(t.Context(), second) }()
	select {
	case result := <-done:
		if result != Dropped {
			t.Fatalf("full queue result = %s", result)
		}
	case <-time.After(time.Second):
		<-q.tasks
		<-done
		t.Fatal("enqueue blocked on a full queue")
	}
	item, ok := q.next(t.Context())
	if !ok || item.trip != first {
		t.Fatal("first task was lost")
	}
	q.complete(item)
	if result := q.Enqueue(t.Context(), second); result != Queued {
		t.Fatalf("dropped task cannot be queued again: %s", result)
	}
}

func TestQueueCancelledTaskDoesNotReplaceNewTask(t *testing.T) {
	q, stats := testQueue(t, 2)
	trip := model.StaleTrip{TripID: uuid.New()}
	q.Enqueue(t.Context(), trip)
	old := q.pending[trip.TripID]
	q.Cancel(trip.TripID)
	if got := q.Enqueue(t.Context(), trip); got != Queued {
		t.Fatalf("requeue = %s", got)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	current, ok := q.next(ctx)
	if !ok || current == old {
		t.Fatal("old cancelled task was returned for processing")
	}
	q.complete(old)
	if got := q.Enqueue(t.Context(), trip); got != Duplicate {
		t.Fatalf("old completion removed the new task: %s", got)
	}
	q.complete(current)
	q.close()
	if _, ok := q.next(t.Context()); ok {
		t.Fatal("unexpected remaining task")
	}
	if stats.cancelled != 1 || stats.size != 0 {
		t.Fatalf("cancelled=%d, size=%d; want 1 and 0", stats.cancelled, stats.size)
	}
}

func TestQueueCancelledTasksOccupyChannelUntilRead(t *testing.T) {
	q, stats := testQueue(t, 1)
	trip := model.StaleTrip{TripID: uuid.New()}
	q.Enqueue(t.Context(), trip)
	q.Cancel(trip.TripID)
	if got := q.Enqueue(t.Context(), model.StaleTrip{TripID: uuid.New()}); got != Dropped {
		t.Fatalf("cancelled channel entry disappeared before reading: %s", got)
	}
	q.close()
	if _, ok := q.next(t.Context()); ok {
		t.Fatal("cancelled task was returned for processing")
	}
	if stats.cancelled != 1 || len(q.pending) != 0 {
		t.Fatal("cancelled task is still pending")
	}
}

func TestQueueNextHonorsContext(t *testing.T) {
	q, _ := testQueue(t, 1)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, ok := q.next(ctx); ok {
		t.Fatal("next returned a task after cancellation")
	}
}

func TestQueueConcurrentDuplicate(t *testing.T) {
	const producers = 64
	q, stats := testQueue(t, producers)
	trip := model.StaleTrip{TripID: uuid.New()}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range producers {
		wg.Go(func() {
			<-start
			q.Enqueue(t.Context(), trip)
		})
	}
	close(start)
	wg.Wait()
	if stats.results[string(Queued)] != 1 || stats.results[string(Duplicate)] != producers-1 || len(q.tasks) != 1 || len(q.pending) != 1 {
		t.Fatalf("concurrent duplicate: queued=%d, duplicate=%d, channel=%d, pending=%d",
			stats.results[string(Queued)], stats.results[string(Duplicate)], len(q.tasks), len(q.pending))
	}
}

func TestQueueConcurrentEnqueueCancelAndComplete(t *testing.T) {
	q, stats := testQueue(t, 8)
	trip := model.StaleTrip{TripID: uuid.New()}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			item, ok := q.next(t.Context())
			if !ok {
				return
			}
			q.complete(item)
		}
	}()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				q.Enqueue(t.Context(), trip)
				q.Cancel(trip.TripID)
			}
		})
	}
	wg.Wait()
	q.close()
	<-done
	if len(q.pending) != 0 || len(q.tasks) != 0 || stats.size != 0 {
		t.Fatalf("queue did not drain: pending=%d, channel=%d, size=%d", len(q.pending), len(q.tasks), stats.size)
	}
}
