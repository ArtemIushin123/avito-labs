package trip

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ArtemIushin123/template/internal/domain/trip"
)

func (r *Repository) ListPositions(ctx context.Context, id uuid.UUID) ([]trip.Position, error) {
	query, args, err := r.builder.Select("id", "trip_id", "latitude", "longitude", "recorded_at", "created_at").
		From("trip_positions").
		Where(squirrel.Eq{"trip_id": id}).
		OrderBy("recorded_at ASC", "id ASC").ToSql()
	if err != nil {
		return nil, fmt.Errorf("build list positions: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	rows, err := r.getExecutor(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}
	positions, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (trip.Position, error) {
		var p trip.Position
		err := row.Scan(&p.ID, &p.TripID, &p.Coordinates.Latitude, &p.Coordinates.Longitude, &p.RecordedAt, &p.CreatedAt)
		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("read positions: %w", err)
	}
	return positions, nil
}

func (r *Repository) InsertPosition(ctx context.Context, p *trip.Position) error {
	query, args, err := r.builder.Insert("trip_positions").
		Columns("trip_id", "latitude", "longitude", "recorded_at").
		Values(p.TripID, p.Coordinates.Latitude, p.Coordinates.Longitude, p.RecordedAt).
		Suffix("RETURNING id, recorded_at, created_at").ToSql()
	if err != nil {
		return fmt.Errorf("build insert position: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	if err := r.getExecutor(ctx).QueryRow(ctx, query, args...).Scan(&p.ID, &p.RecordedAt, &p.CreatedAt); err != nil {
		return fmt.Errorf("insert position: %w", err)
	}
	return nil
}

func (r *Repository) AdvanceLastPositionAt(ctx context.Context, id uuid.UUID, at time.Time) error {
	query, args, err := r.builder.Update("trips").
		Set("last_position_at", squirrel.Expr("GREATEST(last_position_at, ?)", at)).
		Set("updated_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"id": id}).ToSql()
	if err != nil {
		return fmt.Errorf("build update last position time: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	tag, err := r.getExecutor(ctx).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update last position time: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return trip.ErrTripNotFound
	}
	return nil
}
