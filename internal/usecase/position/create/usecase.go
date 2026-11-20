package create

import (
	"context"
	"fmt"
	"time"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/model"
)

type Usecase struct {
	repo   tripRepository
	tx     txManager
	events positionEvents
	queue  positionCanceller
	now    func() time.Time
}

func New(repo tripRepository, tx txManager, events positionEvents, queue positionCanceller, now func() time.Time) *Usecase {
	return &Usecase{repo: repo, tx: tx, events: events, queue: queue, now: now}
}

func (u *Usecase) CreatePosition(ctx context.Context, in model.CreatePositionInput) (*trip.Position, error) {
	p, err := trip.NewPosition(in.TripID, in.Coordinates, in.RecordedAt, u.now())
	if err != nil {
		return nil, fmt.Errorf("create position: %w", err)
	}
	err = u.tx.Do(ctx, func(ctx context.Context) error {
		tr, err := u.repo.GetByIDForUpdate(ctx, in.TripID)
		if err != nil {
			return err
		}
		if tr.Status != trip.StatusActive {
			return trip.ErrTripCompleted
		}
		if err := u.repo.InsertPosition(ctx, p); err != nil {
			return err
		}
		return u.repo.AdvanceLastPositionAt(ctx, p.TripID, p.RecordedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("save trip position: %w", err)
	}
	u.queue.Cancel(p.TripID)
	u.events.PositionSaved(ctx)
	return p, nil
}
