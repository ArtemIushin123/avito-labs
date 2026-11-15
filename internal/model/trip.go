package model

import (
	"github.com/google/uuid"

	"github.com/ArtemIushin123/template/internal/domain/trip"
)

type CreateTripInput struct {
	DriverID   uuid.UUID
	UserID     uuid.UUID
	Price      int64
	StartPoint trip.Coordinate
	EndPoint   trip.Coordinate
}
