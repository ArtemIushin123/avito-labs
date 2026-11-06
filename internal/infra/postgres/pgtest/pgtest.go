//go:build integration

package pgtest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArtemIushin123/template/internal/domain/trip"
)

const QueryTimeout = 5 * time.Second

func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL is not set: run `tripgoctl environment start` and use `make test-integration`")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping database: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

func NewTrip(t *testing.T) *trip.Trip {
	t.Helper()

	startedAt := time.Now().UTC().Truncate(time.Microsecond)

	tr, err := trip.New(
		uuid.New(), uuid.New(), uuid.New(),
		1450,
		trip.Coordinate{Latitude: 59.9398, Longitude: 30.3146},
		trip.Coordinate{Latitude: 59.9290, Longitude: 30.3626},
		startedAt,
	)
	if err != nil {
		t.Fatalf("build test trip: %v", err)
	}
	return tr
}

func DeleteTrips(t *testing.T, pool *pgxpool.Pool, ids ...uuid.UUID) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if _, err := pool.Exec(ctx, "DELETE FROM trips WHERE id = ANY($1)", ids); err != nil {
			t.Errorf("cleanup trips: %v", err)
		}
	})
}
