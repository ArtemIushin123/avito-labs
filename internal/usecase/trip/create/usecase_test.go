package create_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/model"
	"github.com/ArtemIushin123/template/internal/usecase/trip/create"
)

var (
	fixedID  = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	fixedNow = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
)

type txKey struct{}

type inTx struct{}

func (inTx) Matches(x any) bool {
	ctx, ok := x.(context.Context)
	return ok && ctx.Value(txKey{}) != nil
}

func (inTx) String() string {
	return "context from txManager.Do"
}

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

func validInput() model.CreateTripInput {
	return model.CreateTripInput{
		DriverID:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		UserID:     uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		Price:      1450,
		StartPoint: trip.Coordinate{Latitude: 59.9398, Longitude: 30.3146},
		EndPoint:   trip.Coordinate{Latitude: 59.9290, Longitude: 30.3626},
	}
}

func expectedTrip() *trip.Trip {
	in := validInput()
	return &trip.Trip{
		ID:         fixedID,
		DriverID:   in.DriverID,
		UserID:     in.UserID,
		Price:      in.Price,
		Status:     trip.StatusActive,
		StartPoint: in.StartPoint,
		EndPoint:   in.EndPoint,
		StartedAt:  fixedNow,
	}
}

func TestUsecase_Create(t *testing.T) {
	errDB := errors.New("db is down")

	tests := []struct {
		name           string
		modify         func(in *model.CreateTripInput)
		setup          func(repo *MocktripRepository, tx *MocktxManager, events *MocktripEvents)
		wantErr        error
		wantValidation bool
	}{
		{
			name: "success",
			setup: func(repo *MocktripRepository, tx *MocktxManager, events *MocktripEvents) {
				expectTx(tx)
				gomock.InOrder(
					repo.EXPECT().Create(inTx{}, expectedTrip()).Return(nil),
					repo.EXPECT().
						AddStatusHistory(inTx{}, fixedID, gomock.Nil(), trip.StatusActive, gomock.Any()).
						Return(nil),
					events.EXPECT().TripCreated(outsideTx{}, expectedTrip()),
				)
			},
		},
		{
			name:           "invalid input does not touch db",
			modify:         func(in *model.CreateTripInput) { in.DriverID = uuid.Nil },
			setup:          func(*MocktripRepository, *MocktxManager, *MocktripEvents) {},
			wantValidation: true,
		},
		{
			name: "driver busy, history is not written",
			setup: func(repo *MocktripRepository, tx *MocktxManager, _ *MocktripEvents) {
				expectTx(tx)
				repo.EXPECT().Create(inTx{}, gomock.Any()).Return(trip.ErrDriverBusy)
			},
			wantErr: trip.ErrDriverBusy,
		},
		{
			name: "status history error",
			setup: func(repo *MocktripRepository, tx *MocktxManager, _ *MocktripEvents) {
				expectTx(tx)
				repo.EXPECT().Create(inTx{}, gomock.Any()).Return(nil)
				repo.EXPECT().
					AddStatusHistory(inTx{}, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errDB)
			},
			wantErr: errDB,
		},
		{
			name: "transaction cannot start",
			setup: func(_ *MocktripRepository, tx *MocktxManager, _ *MocktripEvents) {
				tx.EXPECT().Do(gomock.Any(), gomock.Any()).Return(errDB)
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

			in := validInput()
			if tt.modify != nil {
				tt.modify(&in)
			}

			uc := create.New(repo, tx, events,
				func() time.Time { return fixedNow },
				func() uuid.UUID { return fixedID },
			)

			got, err := uc.Create(context.Background(), in)

			if tt.wantValidation {
				var vErr *trip.ValidationError
				if !errors.As(err, &vErr) {
					t.Fatalf("Create() error = %v, want *trip.ValidationError", err)
				}
				if got != nil {
					t.Errorf("Create() trip = %+v, want nil", got)
				}
				return
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Create() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if got != nil {
					t.Errorf("Create() trip = %+v, want nil on error", got)
				}
				return
			}

			if want := expectedTrip(); *got != *want {
				t.Errorf("Create() =\n  %+v\nwant\n  %+v", *got, *want)
			}
		})
	}
}
