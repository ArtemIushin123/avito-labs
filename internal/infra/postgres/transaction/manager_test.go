//go:build integration

package transaction_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/infra/postgres/pgtest"
	"github.com/ArtemIushin123/template/internal/infra/postgres/transaction"
	triprepo "github.com/ArtemIushin123/template/internal/repository/trip"
)

func TestManager_RollbackOnSecondInsertError(t *testing.T) {
	pool := pgtest.Pool(t)
	repo := triprepo.New(pool, pgtest.QueryTimeout)
	txm := transaction.New(pool, pgtest.QueryTimeout)
	tr := pgtest.NewTrip(t)
	pgtest.DeleteTrips(t, pool, tr.ID)

	reason := "test rollback"

	err := txm.Do(context.Background(), func(ctx context.Context) error {
		if err := repo.Create(ctx, tr); err != nil {
			return err
		}

		return repo.AddStatusHistory(ctx, tr.ID, nil, trip.Status("bogus"), &reason)
	})

	if err == nil {
		t.Fatal("Do() error = nil, want error from the second insert")
	}

	_, err = repo.GetByID(context.Background(), tr.ID)
	if !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("GetByID() after rollback error = %v, want %v: trip stayed in the database, rollback did not work",
			err, trip.ErrTripNotFound)
	}
}

func TestManager_TimeoutWaitingForConnection(t *testing.T) {
	cfg := pgtest.Pool(t).Config()
	cfg.MaxConns = 1
	cfg.MinConns = 0
	cfg.MinIdleConns = 0
	ctx, cancel := context.WithTimeout(t.Context(), pgtest.QueryTimeout)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("create single-connection pool: %v", err)
	}
	defer pool.Close()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire the only connection: %v", err)
	}
	defer conn.Release()

	txm := transaction.New(pool, 50*time.Millisecond)
	err = txm.Do(ctx, func(context.Context) error {
		t.Fatal("transaction started while the only connection was held")
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Errorf("Do error = %v, parent error = %v, want transaction deadline with live parent", err, ctx.Err())
	}
}
