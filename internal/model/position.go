package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/ArtemIushin123/template/internal/domain/trip"
)

type StaleTrip struct {
	TripID   uuid.UUID
	DriverID uuid.UUID
}

type CreatePositionInput struct {
	TripID      uuid.UUID
	Coordinates trip.Coordinate
	RecordedAt  time.Time
}
