package finish_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/usecase/trip/finish"
)

var (
	tripID    = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	startedAt = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	fixedNow  = time.Date(2026, 8, 24, 12, 18, 0, 0, time.UTC)
	earlier   = time.Date(2026, 8, 24, 12, 10, 0, 0, time.UTC)
)

type txKey struct{}

type inTx struct{}

func (inTx) Matches(x any) bool {
	ctx, ok := x.(context.Context)
	return ok && ctx.Value(txKey{}) != nil
}

func (inTx) String() string { return "context from txManager.Do" }

type outsideTx struct{}

func (outsideTx) Matches(x any) bool {
	ctx, ok := x.(context.Context)
	return ok && ctx.Value(txKey{}) == nil
}

func (outsideTx) String() string { return "context outside of txManager.Do" }

func expectTx(tx *MocktxManager) {
	tx.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(context.WithValue(ctx, txKey{}, "tx"))
		})
}

func activeTrip() *trip.Trip {
	return &trip.Trip{ID: tripID, Status: trip.StatusActive, StartedAt: startedAt}
}

func completedTrip() *trip.Trip {
	at := earlier
	return &trip.Trip{ID: tripID, Status: trip.StatusCompleted, StartedAt: startedAt, FinishedAt: &at}
}

func finishedTrip() *trip.Trip {
	at := fixedNow
	return &trip.Trip{ID: tripID, Status: trip.StatusCompleted, StartedAt: startedAt, FinishedAt: &at}
}

func TestUsecase_Finish(t *testing.T) {
	errDB := errors.New("db is down")

	tests := []struct {
		name    string
		setup   func(repo *MocktripRepository, tx *MocktxManager, events *MocktripEvents)
		wantErr error
		notErr  error
	}{
		{
			name: "active trip is finished",
			setup: func(repo *MocktripRepository, tx *MocktxManager, events *MocktripEvents) {
				expectTx(tx)
				active := trip.StatusActive
				gomock.InOrder(
					repo.EXPECT().GetByIDForUpdate(inTx{}, tripID).Return(activeTrip(), nil),
					repo.EXPECT().Save(inTx{}, finishedTrip()).Return(nil),
					repo.EXPECT().
						AddStatusHistory(inTx{}, tripID, &active, trip.StatusCompleted, gomock.Any()).
						Return(nil),
					events.EXPECT().TripCompleted(outsideTx{}, finishedTrip()),
				)
			},
		},
		{
			name: "completed trip is not saved again",
			setup: func(repo *MocktripRepository, tx *MocktxManager, _ *MocktripEvents) {
				expectTx(tx)
				repo.EXPECT().GetByIDForUpdate(inTx{}, tripID).Return(completedTrip(), nil)
			},
			wantErr: trip.ErrTripCompleted,
		},
		{
			name: "trip not found",
			setup: func(repo *MocktripRepository, tx *MocktxManager, _ *MocktripEvents) {
				expectTx(tx)
				repo.EXPECT().GetByIDForUpdate(inTx{}, tripID).Return(nil, trip.ErrTripNotFound)
			},
			wantErr: trip.ErrTripNotFound,
		},
		{
			name: "db error on read is not reported as not found",
			setup: func(repo *MocktripRepository, tx *MocktxManager, _ *MocktripEvents) {
				expectTx(tx)
				repo.EXPECT().GetByIDForUpdate(inTx{}, tripID).Return(nil, errDB)
			},
			wantErr: errDB,
			notErr:  trip.ErrTripNotFound,
		},
		{
			name: "save error, history is not written",
			setup: func(repo *MocktripRepository, tx *MocktxManager, _ *MocktripEvents) {
				expectTx(tx)
				repo.EXPECT().GetByIDForUpdate(inTx{}, tripID).Return(activeTrip(), nil)
				repo.EXPECT().Save(inTx{}, gomock.Any()).Return(errDB)
			},
			wantErr: errDB,
		},
		{
			name: "status history error",
			setup: func(repo *MocktripRepository, tx *MocktxManager, _ *MocktripEvents) {
				expectTx(tx)
				repo.EXPECT().GetByIDForUpdate(inTx{}, tripID).Return(activeTrip(), nil)
				repo.EXPECT().Save(inTx{}, gomock.Any()).Return(nil)
				repo.EXPECT().
					AddStatusHistory(inTx{}, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errDB)
			},
			wantErr: errDB,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			events := NewMocktripEvents(ctrl)
			tt.setup(repo, tx, events)

			uc := finish.New(repo, tx, events, func() time.Time { return fixedNow })

			got, err := uc.Finish(context.Background(), tripID)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Finish() error = %v, want %v", err, tt.wantErr)
			}
			if tt.notErr != nil && errors.Is(err, tt.notErr) {
				t.Fatalf("Finish() error = %v, must not be reported as %v", err, tt.notErr)
			}

			if tt.wantErr != nil {
				if got != nil {
					t.Errorf("Finish() trip = %+v, want nil on error", got)
				}
				return
			}

			if got == nil {
				t.Fatal("Finish() trip = nil, want finished trip")
			}
			if got.Status != trip.StatusCompleted {
				t.Errorf("Status = %q, want %q", got.Status, trip.StatusCompleted)
			}
			if got.FinishedAt == nil || !got.FinishedAt.Equal(fixedNow) {
				t.Errorf("FinishedAt = %v, want %v", got.FinishedAt, fixedNow)
			}
		})
	}
}
