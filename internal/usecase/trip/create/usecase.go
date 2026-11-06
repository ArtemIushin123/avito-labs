package create

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/model"
)

type Usecase struct {
	repo   tripRepository
	tx     txManager
	events tripEvents
	now    func() time.Time
	newID  func() uuid.UUID
}

func New(repo tripRepository, tx txManager, events tripEvents, now func() time.Time, newID func() uuid.UUID) *Usecase {
	return &Usecase{repo: repo, tx: tx, events: events, now: now, newID: newID}
}

func (u *Usecase) Create(ctx context.Context, in model.CreateTripInput) (*trip.Trip, error) {
	tr, err := trip.New(u.newID(), in.DriverID, in.UserID, in.Price, in.StartPoint, in.EndPoint, u.now())
	if err != nil {
		return nil, fmt.Errorf("create trip: %w", err)
	}

	err = u.tx.Do(ctx, func(ctx context.Context) error {
		err := u.repo.Create(ctx, tr)
		if err != nil {
			return fmt.Errorf("save trip: %w", err)
		}

		reason := "trip created"
		err = u.repo.AddStatusHistory(ctx, tr.ID, nil, tr.Status, &reason)
		if err != nil {
			return fmt.Errorf("add status history: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("create trip: %w", err)
	}

	u.events.TripCreated(ctx, tr)
	return tr, nil
}
