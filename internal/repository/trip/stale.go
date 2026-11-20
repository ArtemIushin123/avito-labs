package trip

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ArtemIushin123/template/internal/model"
)

func (r *Repository) ListStaleTrips(ctx context.Context, staleAfter time.Duration) ([]model.StaleTrip, error) {
	const positionTime = "COALESCE(last_position_at, started_at)"
	query, args, err := r.builder.Select("id", "driver_id").
		From("trips").
		Where("status = 'active'").
		Where(positionTime+" < now() - ?::interval", staleAfter).
		OrderBy(positionTime).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build list stale trips: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	rows, err := r.getExecutor(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list stale trips: %w", err)
	}
	trips, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.StaleTrip, error) {
		var candidate model.StaleTrip
		err := row.Scan(&candidate.TripID, &candidate.DriverID)
		return candidate, err
	})
	if err != nil {
		return nil, fmt.Errorf("read stale trips: %w", err)
	}
	return trips, nil
}
