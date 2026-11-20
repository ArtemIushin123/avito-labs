package positions

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type scanTicker struct {
	*time.Ticker
}

func (t scanTicker) Ticks() <-chan time.Time {
	return t.C
}

type Scanner struct {
	queue      *Queue
	source     staleTripSource
	logger     *slog.Logger
	tracer     trace.Tracer
	staleAfter time.Duration
	cancel     context.CancelFunc
	done       chan struct{}
}

func StartScanner(
	ctx context.Context,
	queue *Queue,
	source staleTripSource,
	logger *slog.Logger,
	tracer trace.Tracer,
	interval, staleAfter time.Duration,
) *Scanner {
	return startScanner(ctx, queue, source, logger, tracer, scanTicker{time.NewTicker(interval)}, staleAfter)
}

func startScanner(
	ctx context.Context,
	queue *Queue,
	source staleTripSource,
	logger *slog.Logger,
	tracer trace.Tracer,
	tick ticker,
	staleAfter time.Duration,
) *Scanner {
	ctx, cancel := context.WithCancel(ctx)
	scanner := &Scanner{
		queue:      queue,
		source:     source,
		logger:     logger,
		tracer:     tracer,
		staleAfter: staleAfter,
		cancel:     cancel,
		done:       make(chan struct{}),
	}
	go scanner.run(ctx, tick)
	return scanner
}

func (s *Scanner) Shutdown(ctx context.Context) error {
	s.cancel()
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Scanner) run(ctx context.Context, tick ticker) {
	s.logger.InfoContext(ctx, "position scanner started")
	defer close(s.done)
	defer s.logger.InfoContext(ctx, "position scanner stopped")
	defer s.cancel()
	defer tick.Stop()

	ticks := tick.Ticks()
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok || ctx.Err() != nil {
				return
			}
			s.scan(ctx)
		}
	}
}

func (s *Scanner) scan(ctx context.Context) {
	ctx, span := s.tracer.Start(ctx, "stale trip check")
	defer span.End()

	trips, err := s.source.ListStaleTrips(ctx, s.staleAfter)
	if err != nil {
		if ctx.Err() == nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "list stale trips failed")
			s.logger.WarnContext(ctx, "stale trip check failed", "error", err)
		}
		return
	}
	for _, trip := range trips {
		if ctx.Err() != nil {
			return
		}
		s.queue.Enqueue(ctx, trip)
	}
}
