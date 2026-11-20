package list

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

func (u *Usecase) ListPositions(ctx context.Context, id uuid.UUID) ([]trip.Position, error) {
	if _, err := u.repo.GetByID(ctx, id); err != nil {
		return nil, fmt.Errorf("get trip for route: %w", err)
	}
	positions, err := u.repo.ListPositions(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list trip positions: %w", err)
	}
	return positions, nil
}
