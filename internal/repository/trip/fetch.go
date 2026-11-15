package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/infra/postgres/transaction"
)

func (r *Repository) selectByID(id uuid.UUID) squirrel.SelectBuilder {
	return r.builder.Select(tripColumns...).
		From("trips").
		Where(squirrel.Eq{"id": id})
}

func (r *Repository) fetchOne(ctx context.Context, builder squirrel.SelectBuilder) (*trip.Trip, error) {
	query, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get trip query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	var row tripRow

	err = r.getExecutor(ctx).QueryRow(ctx, query, args...).Scan(row.scanTargets()...)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, trip.ErrTripNotFound
		}
		return nil, fmt.Errorf("get trip: %w", err)
	}

	return toDomain(row), nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*trip.Trip, error) {
	builder := r.selectByID(id)
	return r.fetchOne(ctx, builder)
}

func (r *Repository) GetByIDForUpdate(ctx context.Context, id uuid.UUID) (*trip.Trip, error) {
	_, ok := transaction.CheckTransaction(ctx)
	if !ok {
		return nil, errors.New("GetByIDForUpdate must be called inside a transaction")
	}

	builder := r.selectByID(id).Suffix("FOR UPDATE")
	return r.fetchOne(ctx, builder)
}
