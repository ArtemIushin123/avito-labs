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
	"github.com/ArtemIushin123/template/internal/usecase/position/create"
)

func TestCreatePosition(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	id := uuid.New()
	errCommit := errors.New("commit failed")
	for _, tc := range []struct {
		name                        string
		status                      trip.Status
		readErr, commitErr, wantErr error
	}{
		{name: "active trip", status: trip.StatusActive},
		{name: "missing trip", readErr: trip.ErrTripNotFound, wantErr: trip.ErrTripNotFound},
		{name: "completed trip", status: trip.StatusCompleted, wantErr: trip.ErrTripCompleted},
		{name: "failed commit", status: trip.StatusActive, commitErr: errCommit, wantErr: errCommit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			ctrl := gomock.NewController(t)
			repo, tx, events := NewMocktripRepository(ctrl), NewMocktxManager(ctrl), NewMockpositionEvents(ctrl)
			queue := NewMockpositionCanceller(ctrl)
			committed := false
			tx.EXPECT().Do(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				txCtx, cancel := context.WithCancel(ctx)
				defer cancel()
				repo.EXPECT().GetByIDForUpdate(txCtx, id).Return(&trip.Trip{Status: tc.status}, tc.readErr)
				if tc.status == trip.StatusActive {
					gomock.InOrder(repo.EXPECT().InsertPosition(txCtx, gomock.Any()), repo.EXPECT().AdvanceLastPositionAt(txCtx, id, now))
				}
				if err := fn(txCtx); err != nil {
					return err
				}
				committed = tc.commitErr == nil
				return tc.commitErr
			})
			if tc.wantErr == nil {
				queue.EXPECT().Cancel(id).Do(func(uuid.UUID) {
					if !committed {
						t.Error("cancelled queued request before commit")
					}
				})
				events.EXPECT().PositionSaved(ctx).Do(func(context.Context) {
					if !committed {
						t.Error("reported saved before commit")
					}
				})
			}
			uc := create.New(repo, tx, events, queue, func() time.Time { return now })
			got, err := uc.CreatePosition(ctx, model.CreatePositionInput{TripID: id, RecordedAt: now})
			if !errors.Is(err, tc.wantErr) || (got != nil) != (tc.wantErr == nil) {
				t.Fatalf("CreatePosition returned %+v, %v; want error %v", got, err, tc.wantErr)
			}
		})
	}
}
