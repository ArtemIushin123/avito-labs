package get

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ArtemIushin123/template/internal/domain/trip"
)

type Usecase struct {
	repo tripRepository
}

func New(repo tripRepository) *Usecase {
	return &Usecase{repo: repo}
}

func (u *Usecase) GetByID(ctx context.Context, id uuid.UUID) (*trip.Trip, error) {
	tr, err := u.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get trip: %w", err)
	}
	return tr, nil
}
