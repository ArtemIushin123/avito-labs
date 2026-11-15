package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ArtemIushin123/template/internal/domain/trip"
)

const idxOneActiveTripPerDriver = "idx_one_active_trip_per_driver"

func (r *Repository) Create(ctx context.Context, tr *trip.Trip) error {
	executor := r.getExecutor(ctx)

	query, args, err := r.builder.
		Insert("trips").
		Columns(tripColumns...).
		Values(fromDomain(tr).values()...).
		ToSql()
	if err != nil {
		return fmt.Errorf("build create trip query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if _, err = executor.Exec(ctx, query, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == idxOneActiveTripPerDriver {
			return trip.ErrDriverBusy
		}
		return fmt.Errorf("create trip: %w", err)
	}

	return nil
}

func (r *Repository) AddStatusHistory(
	ctx context.Context,
	tripID uuid.UUID,
	fromStatus *trip.Status,
	toStatus trip.Status,
	reason *string,
) error {
	executor := r.getExecutor(ctx)

	query, args, err := r.builder.
		Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(tripID, statusToNullable(fromStatus), string(toStatus), reason).
		ToSql()
	if err != nil {
		return fmt.Errorf("build add status history query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if _, err = executor.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("add status history: %w", err)
	}

	return nil
}

func (r *Repository) Save(ctx context.Context, tr *trip.Trip) error {
	executor := r.getExecutor(ctx)

	query, args, err := r.builder.
		Update("trips").
		Set("status", string(tr.Status)).
		Set("finished_at", tr.FinishedAt).
		Set("updated_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"id": tr.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build save trip query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	tag, err := executor.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("save trip: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return trip.ErrTripNotFound
	}

	return nil
}

func statusToNullable(s *trip.Status) *string {
	if s == nil {
		return nil
	}
	v := string(*s)
	return &v
}
