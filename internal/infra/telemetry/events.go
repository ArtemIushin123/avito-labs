package telemetry

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/ArtemIushin123/template/internal/domain/trip"
)

type TripEvents struct {
	logger    *slog.Logger
	created   metric.Int64Counter
	completed metric.Int64Counter
}

func NewTripEvents(logger *slog.Logger, meter metric.Meter) (*TripEvents, error) {
	created, err := meter.Int64Counter("tripgo.trips.created",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Созданные поездки"),
	)
	if err != nil {
		return nil, fmt.Errorf("create tripgo.trips.created counter: %w", err)
	}

	completed, err := meter.Int64Counter("tripgo.trips.completed",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Завершённые поездки"),
	)
	if err != nil {
		return nil, fmt.Errorf("create tripgo.trips.completed counter: %w", err)
	}

	return &TripEvents{logger: logger, created: created, completed: completed}, nil
}

// TripCreated вызывается usecase после успешного коммита.
// Пишем только trip_id: id пассажира и водителя — персональные данные (conventions.md §5).
func (e *TripEvents) TripCreated(ctx context.Context, tr *trip.Trip) {
	e.created.Add(ctx, 1)
	e.logger.InfoContext(ctx, "trip created", "trip_id", tr.ID.String())
}

func (e *TripEvents) TripCompleted(ctx context.Context, tr *trip.Trip) {
	e.completed.Add(ctx, 1)
	e.logger.InfoContext(ctx, "trip completed", "trip_id", tr.ID.String())
}
