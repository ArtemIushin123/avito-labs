//go:build integration

package trip_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/infra/postgres/pgtest"
	"github.com/ArtemIushin123/template/internal/infra/postgres/transaction"
	triprepo "github.com/ArtemIushin123/template/internal/repository/trip"
	"github.com/ArtemIushin123/template/internal/usecase/trip/finish"
)

func assertSameTrip(t *testing.T, got, want *trip.Trip) {
	t.Helper()

	if got.ID != want.ID || got.UserID != want.UserID || got.DriverID != want.DriverID {
		t.Errorf("ids = (%v, %v, %v), want (%v, %v, %v)",
			got.ID, got.UserID, got.DriverID, want.ID, want.UserID, want.DriverID)
	}
	if got.StartPoint != want.StartPoint || got.EndPoint != want.EndPoint {
		t.Errorf("points = %+v -> %+v, want %+v -> %+v",
			got.StartPoint, got.EndPoint, want.StartPoint, want.EndPoint)
	}
	if got.Price != want.Price || got.Status != want.Status {
		t.Errorf("price, status = %d, %q, want %d, %q", got.Price, got.Status, want.Price, want.Status)
	}
	if !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, want.StartedAt)
	}
	switch {
	case got.FinishedAt == nil && want.FinishedAt == nil:
	case got.FinishedAt == nil || want.FinishedAt == nil:
		t.Errorf("FinishedAt = %v, want %v", got.FinishedAt, want.FinishedAt)
	case !got.FinishedAt.Equal(*want.FinishedAt):
		t.Errorf("FinishedAt = %v, want %v", *got.FinishedAt, *want.FinishedAt)
	}
}

type historyRow struct {
	from   *string
	to     string
	reason *string
}

func readHistory(t *testing.T, pool *pgxpool.Pool, tripID uuid.UUID) []historyRow {
	t.Helper()

	rows, err := pool.Query(context.Background(),
		"SELECT from_status, to_status, reason FROM trip_status_history WHERE trip_id = $1 ORDER BY id",
		tripID)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	defer rows.Close()

	var out []historyRow
	for rows.Next() {
		var h historyRow
		if err := rows.Scan(&h.from, &h.to, &h.reason); err != nil {
			t.Fatalf("scan history: %v", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read history rows: %v", err)
	}
	return out
}

func finishTrip(t *testing.T, txm *transaction.Manager, repo *triprepo.Repository, id uuid.UUID, at time.Time) {
	t.Helper()

	err := txm.Do(context.Background(), func(ctx context.Context) error {
		tr, err := repo.GetByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if err := tr.Finish(at); err != nil {
			return err
		}
		return repo.Save(ctx, tr)
	})
	if err != nil {
		t.Fatalf("finish trip: %v", err)
	}
}

func TestRepository_CreateAndGetByID(t *testing.T) {
	pool := pgtest.Pool(t)
	repo := triprepo.New(pool, pgtest.QueryTimeout)

	want := pgtest.NewTrip(t)
	pgtest.DeleteTrips(t, pool, want.ID)

	if err := repo.Create(context.Background(), want); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.GetByID(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	assertSameTrip(t, got, want)
}

func TestRepository_GetByID_NotFound(t *testing.T) {
	pool := pgtest.Pool(t)
	repo := triprepo.New(pool, pgtest.QueryTimeout)

	_, err := repo.GetByID(context.Background(), uuid.New())

	if !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("GetByID() error = %v, want %v", err, trip.ErrTripNotFound)
	}
}

func TestRepository_Create_DriverBusy(t *testing.T) {
	pool := pgtest.Pool(t)
	repo := triprepo.New(pool, pgtest.QueryTimeout)

	first := pgtest.NewTrip(t)
	second := pgtest.NewTrip(t)
	second.DriverID = first.DriverID
	pgtest.DeleteTrips(t, pool, first.ID, second.ID)

	if err := repo.Create(context.Background(), first); err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}

	err := repo.Create(context.Background(), second)

	if !errors.Is(err, trip.ErrDriverBusy) {
		t.Fatalf("Create(second) error = %v, want %v", err, trip.ErrDriverBusy)
	}
}

func TestRepository_Create_DuplicateIDIsNotDriverBusy(t *testing.T) {
	pool := pgtest.Pool(t)
	repo := triprepo.New(pool, pgtest.QueryTimeout)

	first := pgtest.NewTrip(t)
	duplicate := pgtest.NewTrip(t)
	duplicate.ID = first.ID
	pgtest.DeleteTrips(t, pool, first.ID)

	if err := repo.Create(context.Background(), first); err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}

	err := repo.Create(context.Background(), duplicate)

	if err == nil {
		t.Fatal("Create(duplicate) error = nil, want primary key violation")
	}
	if errors.Is(err, trip.ErrDriverBusy) {
		t.Fatalf("Create(duplicate) error = %v, must not be reported as %v", err, trip.ErrDriverBusy)
	}
}

func TestRepository_Create_DriverFreeAfterFinish(t *testing.T) {
	pool := pgtest.Pool(t)
	repo := triprepo.New(pool, pgtest.QueryTimeout)
	txm := transaction.New(pool)

	first := pgtest.NewTrip(t)
	second := pgtest.NewTrip(t)
	second.DriverID = first.DriverID
	pgtest.DeleteTrips(t, pool, first.ID, second.ID)

	if err := repo.Create(context.Background(), first); err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	finishTrip(t, txm, repo, first.ID, first.StartedAt.Add(18*time.Minute))

	if err := repo.Create(context.Background(), second); err != nil {
		t.Fatalf("Create(second) after finish error = %v, want nil", err)
	}
}

func TestRepository_Save(t *testing.T) {
	pool := pgtest.Pool(t)
	repo := triprepo.New(pool, pgtest.QueryTimeout)
	txm := transaction.New(pool)

	want := pgtest.NewTrip(t)
	pgtest.DeleteTrips(t, pool, want.ID)

	if err := repo.Create(context.Background(), want); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	finishedAt := want.StartedAt.Add(18 * time.Minute)
	finishTrip(t, txm, repo, want.ID, finishedAt)

	got, err := repo.GetByID(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	want.Status = trip.StatusCompleted
	want.FinishedAt = &finishedAt
	assertSameTrip(t, got, want)
}

func TestRepository_Save_NotFound(t *testing.T) {
	pool := pgtest.Pool(t)
	repo := triprepo.New(pool, pgtest.QueryTimeout)

	tr := pgtest.NewTrip(t)

	err := repo.Save(context.Background(), tr)

	if !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("Save() error = %v, want %v", err, trip.ErrTripNotFound)
	}
}

func TestRepository_AddStatusHistory(t *testing.T) {
	pool := pgtest.Pool(t)
	repo := triprepo.New(pool, pgtest.QueryTimeout)

	tr := pgtest.NewTrip(t)
	pgtest.DeleteTrips(t, pool, tr.ID)

	if err := repo.Create(context.Background(), tr); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	created, finished := "trip created", "trip finished"
	active := trip.StatusActive

	if err := repo.AddStatusHistory(context.Background(), tr.ID, nil, trip.StatusActive, &created); err != nil {
		t.Fatalf("AddStatusHistory(created) error = %v", err)
	}
	if err := repo.AddStatusHistory(context.Background(), tr.ID, &active, trip.StatusCompleted, &finished); err != nil {
		t.Fatalf("AddStatusHistory(finished) error = %v", err)
	}

	got := readHistory(t, pool, tr.ID)

	if len(got) != 2 {
		t.Fatalf("history has %d rows, want 2", len(got))
	}
	if got[0].from != nil || got[0].to != "active" || got[0].reason == nil || *got[0].reason != created {
		t.Errorf("row 1 = from %v, to %q, reason %v; want NULL -> active, %q", got[0].from, got[0].to, got[0].reason, created)
	}
	if got[1].from == nil || *got[1].from != "active" || got[1].to != "completed" {
		t.Errorf("row 2 = from %v, to %q; want active -> completed", got[1].from, got[1].to)
	}
}

func TestRepository_GetByIDForUpdate_RequiresTransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	repo := triprepo.New(pool, pgtest.QueryTimeout)

	tr := pgtest.NewTrip(t)
	pgtest.DeleteTrips(t, pool, tr.ID)
	if err := repo.Create(context.Background(), tr); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.GetByIDForUpdate(context.Background(), tr.ID)

	if err == nil {
		t.Fatalf("GetByIDForUpdate() without transaction returned %+v, want error", got)
	}
	if errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("GetByIDForUpdate() error = %v, must not be reported as %v", err, trip.ErrTripNotFound)
	}
}

type noEvents struct{}

func (noEvents) TripCompleted(context.Context, *trip.Trip) {}

func TestFinish_Concurrent(t *testing.T) {
	pool := pgtest.Pool(t)
	repo := triprepo.New(pool, pgtest.QueryTimeout)
	txm := transaction.New(pool)
	uc := finish.New(repo, txm, noEvents{}, func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) })

	tr := pgtest.NewTrip(t)
	pgtest.DeleteTrips(t, pool, tr.ID)
	if err := repo.Create(context.Background(), tr); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	const callers = 2
	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		errs  = make([]error, callers)
	)
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = uc.Finish(context.Background(), tr.ID)
		}()
	}
	close(start)
	wg.Wait()

	var ok, completed int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, trip.ErrTripCompleted):
			completed++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 || completed != 1 {
		t.Fatalf("got %d successes and %d ErrTripCompleted, want 1 and 1", ok, completed)
	}

	var finishedRows int
	for _, h := range readHistory(t, pool, tr.ID) {
		if h.to == "completed" {
			finishedRows++
		}
	}
	if finishedRows != 1 {
		t.Errorf("history has %d 'completed' rows, want 1", finishedRows)
	}
}
