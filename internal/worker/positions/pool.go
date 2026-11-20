package positions

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Pool struct {
	queue   *Queue
	gateway pushGateway
	events  workerEvents
	tracer  trace.Tracer
	timeout time.Duration
	cancel  context.CancelFunc
	done    chan struct{}
}

func StartPool(
	ctx context.Context,
	queue *Queue,
	gateway pushGateway,
	events workerEvents,
	logger *slog.Logger,
	tracer trace.Tracer,
	workers int,
	timeout time.Duration,
) *Pool {
	ctx, cancel := context.WithCancel(ctx)
	pool := &Pool{
		queue:   queue,
		gateway: gateway,
		events:  events,
		tracer:  tracer,
		timeout: timeout,
		cancel:  cancel,
		done:    make(chan struct{}),
	}
	logger.InfoContext(ctx, "position workers started", "workers", workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() { pool.runWorker(ctx) })
	}
	go func() {
		wg.Wait()
		logger.InfoContext(ctx, "position workers stopped")
		cancel()
		close(pool.done)
	}()
	return pool
}

func (p *Pool) Shutdown(ctx context.Context) error {
	if ctx.Err() != nil {
		p.cancel()
	}
	p.queue.close()
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		p.cancel()
		return ctx.Err()
	}
}

func (p *Pool) runWorker(ctx context.Context) {
	for {
		item, ok := p.queue.next(ctx)
		if !ok {
			return
		}
		p.process(ctx, item)
	}
}

func (p *Pool) process(ctx context.Context, item *task) {
	defer p.queue.complete(item)
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	ctx, span := p.tracer.Start(ctx, "position request",
		trace.WithNewRoot(),
		trace.WithLinks(trace.Link{SpanContext: item.checkSpan}),
	)
	defer span.End()

	p.events.WorkersActive(1)
	defer p.events.WorkersActive(-1)
	if err := p.gateway.RequestPosition(ctx, item.trip); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "push request failed")
	}
}
