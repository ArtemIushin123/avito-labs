package positions

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/ArtemIushin123/template/internal/model"
)

type EnqueueResult string

const (
	Queued    EnqueueResult = "queued"
	Duplicate EnqueueResult = "duplicate"
	Dropped   EnqueueResult = "dropped"
)

type task struct {
	trip       model.StaleTrip
	checkSpan  trace.SpanContext
	processing bool
}

type Queue struct {
	mu      sync.Mutex
	tasks   chan *task
	pending map[uuid.UUID]*task
	events  queueEvents
	closed  bool
}

func NewQueue(size int, events queueEvents) *Queue {
	return &Queue{
		tasks:   make(chan *task, size),
		pending: make(map[uuid.UUID]*task),
		events:  events,
	}
}

func (q *Queue) Enqueue(ctx context.Context, trip model.StaleTrip) EnqueueResult {
	q.mu.Lock()
	result := Dropped
	if _, exists := q.pending[trip.TripID]; exists {
		result = Duplicate
	} else if !q.closed {
		item := &task{trip: trip, checkSpan: trace.SpanContextFromContext(ctx)}
		select {
		case q.tasks <- item:
			q.pending[trip.TripID] = item
			result = Queued
		default:
		}
	}
	q.events.QueueSize(ctx, int64(len(q.tasks)))
	q.mu.Unlock()
	q.events.Enqueued(ctx, string(result))
	return result
}

func (q *Queue) Cancel(tripID uuid.UUID) {
	q.mu.Lock()
	if item := q.pending[tripID]; item != nil && !item.processing {
		delete(q.pending, tripID)
	}
	q.mu.Unlock()
}

func (q *Queue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		close(q.tasks)
		q.closed = true
	}
}

func (q *Queue) next(ctx context.Context) (*task, bool) {
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return nil, false
		case item, ok := <-q.tasks:
			if !ok {
				return nil, false
			}
			q.mu.Lock()
			valid := q.pending[item.trip.TripID] == item
			if valid {
				item.processing = true
			}
			q.events.QueueSize(ctx, int64(len(q.tasks)))
			q.mu.Unlock()
			if valid {
				return item, true
			}
			q.events.Cancelled(ctx)
		}
	}
	return nil, false
}

func (q *Queue) complete(item *task) {
	q.mu.Lock()
	if q.pending[item.trip.TripID] == item {
		delete(q.pending, item.trip.TripID)
	}
	q.mu.Unlock()
}
