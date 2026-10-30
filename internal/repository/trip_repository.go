package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/transaction"
	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

type TripRepository struct {
	pool    *pgxpool.Pool
	builder squirrel.StatementBuilderType
}

type Executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

var ErrDriverBusy = errors.New("driver already has an active trip")
var ErrTripNotFound = errors.New("trip not found")
var ErrTripCompleted = errors.New("trip already completed")

func NewTripRepository(pool *pgxpool.Pool) *TripRepository {
	return &TripRepository{
		pool:    pool,
		builder: squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar),
	}
}

func (tr *TripRepository) getExecutor(ctx context.Context) Executor {
	tx, ok := transaction.CheckTransaction(ctx)
	if !ok {
		return tr.pool
	}
	return tx
}

func (tr *TripRepository) GetByID(ctx context.Context, id api.TripId) (*api.Trip, error) {
	executor := tr.getExecutor(ctx)

	query, args, err := tr.builder.
		Select(
			"id", "user_id", "driver_id", "start_latitude", "start_longitude",
			"end_latitude", "end_longitude", "price", "status", "started_at", "finished_at",
		).
		From("trips").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get trip query: %w", err)
	}

	var (
		tripID                        openapi_types.UUID
		userID, driverID              openapi_types.UUID
		startLatitude, startLongitude float64
		endLatitude, endLongitude     float64
		price                         int64
		status                        api.TripStatus
		startedAt                     time.Time
		finishedAt                    *time.Time
	)

	err = executor.QueryRow(ctx, query, args...).Scan(
		&tripID, &userID, &driverID, &startLatitude, &startLongitude,
		&endLatitude, &endLongitude, &price, &status, &startedAt, &finishedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTripNotFound
		}
		return nil, fmt.Errorf("get trip: %w", err)
	}

	return &api.Trip{
		Id:       tripID,
		UserId:   userID,
		DriverId: driverID,
		StartPoint: api.Coordinates{
			Latitude:  startLatitude,
			Longitude: startLongitude,
		},
		EndPoint: api.Coordinates{
			Latitude:  endLatitude,
			Longitude: endLongitude,
		},
		Price:      price,
		Status:     status,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}, nil
}

func (tr *TripRepository) Create(ctx context.Context, trip api.Trip) error {
	executor := tr.getExecutor(ctx)

	query, args, err := tr.builder.
		Insert("trips").
		Columns(
			"id", "user_id", "driver_id", "start_latitude", "start_longitude",
			"end_latitude", "end_longitude", "price", "status", "started_at", "finished_at",
		).
		Values(
			trip.Id, trip.UserId, trip.DriverId, trip.StartPoint.Latitude, trip.StartPoint.Longitude,
			trip.EndPoint.Latitude, trip.EndPoint.Longitude, trip.Price, trip.Status, trip.StartedAt, trip.FinishedAt,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build create trip query: %w", err)
	}

	if _, err = executor.Exec(ctx, query, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDriverBusy
		}
		return fmt.Errorf("create trip: %w", err)
	}

	return nil
}

func (tr *TripRepository) AddStatusHistory(
	ctx context.Context,
	tripId api.TripId,
	fromStatus *string,
	toStatus string,
	reason *string,
) error {
	executor := tr.getExecutor(ctx)

	query, args, err := tr.builder.
		Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(tripId, fromStatus, toStatus, reason).
		ToSql()
	if err != nil {
		return fmt.Errorf("build add status history query: %w", err)
	}

	if _, err = executor.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("add status history: %w", err)
	}

	return nil
}

func (tr *TripRepository) Finish(ctx context.Context, id api.TripId) (*api.Trip, error) {
	executor := tr.getExecutor(ctx)
	now := time.Now().UTC()

	query, args, err := tr.builder.
		Update("trips").
		Set("status", api.Completed).
		Set("finished_at", now).
		Set("updated_at", now).
		Where(squirrel.And{
			squirrel.Eq{"id": id},
			squirrel.Eq{"status": api.Active},
		}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build finish trip query: %w", err)
	}

	tag, err := executor.Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("finish trip: %w", err)
	}

	if tag.RowsAffected() == 0 {
		if _, err := tr.GetByID(ctx, id); err != nil {
			return nil, err
		}
		return nil, ErrTripCompleted
	}

	trip, err := tr.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("read finished trip: %w", err)
	}

	return trip, nil
}
