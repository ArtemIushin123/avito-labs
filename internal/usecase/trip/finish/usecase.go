package finish

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ArtemIushin123/template/internal/domain/trip"
)

type Usecase struct {
	repo   tripRepository
	tx     txManager
	events tripEvents
	now    func() time.Time
}

func New(repo tripRepository, tx txManager, events tripEvents, now func() time.Time) *Usecase {
	return &Usecase{repo: repo, tx: tx, events: events, now: now}
}

func (u *Usecase) Finish(ctx context.Context, id uuid.UUID) (*trip.Trip, error) {
	var tr *trip.Trip

	err := u.tx.Do(ctx, func(ctx context.Context) error {
		var err error
		tr, err = u.repo.GetByIDForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("get trip for update: %w", err)
		}

		err = tr.Finish(u.now())
		if err != nil {
			return fmt.Errorf("finish trip in domain: %w", err)
		}

		err = u.repo.Save(ctx, tr)
		if err != nil {
			return fmt.Errorf("save trip: %w", err)
		}

		reason := "trip finished"
		from := trip.StatusActive
		err = u.repo.AddStatusHistory(ctx, tr.ID, &from, tr.Status, &reason)
		if err != nil {
			return fmt.Errorf("add status history: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("finish trip: %w", err)
	}

	u.events.TripCompleted(ctx, tr)
	return tr, nil
}
