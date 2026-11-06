package trip

import (
	"time"

	"github.com/google/uuid"
)

type Trip struct {
	ID         uuid.UUID
	DriverID   uuid.UUID
	UserID     uuid.UUID
	Price      int64
	Status     Status
	StartPoint Coordinate
	EndPoint   Coordinate
	StartedAt  time.Time
	FinishedAt *time.Time
}

func New(id, driverID, userID uuid.UUID, price int64, startPoint, endPoint Coordinate, startedAt time.Time) (*Trip, error) {
	if driverID == uuid.Nil {
		return nil, &ValidationError{Field: "driver_id", Message: "must not be empty"}
	}
	if userID == uuid.Nil {
		return nil, &ValidationError{Field: "user_id", Message: "must not be empty"}
	}
	if price < 0 {
		return nil, &ValidationError{Field: "price", Message: "must not be negative"}
	}
	if !startPoint.isValid() {
		return nil, &ValidationError{Field: "start_point", Message: "latitude must be in [-90, 90], longitude in [-180, 180]"}
	}
	if !endPoint.isValid() {
		return nil, &ValidationError{Field: "end_point", Message: "latitude must be in [-90, 90], longitude in [-180, 180]"}
	}

	return &Trip{
		ID:         id,
		DriverID:   driverID,
		UserID:     userID,
		Price:      price,
		Status:     StatusActive,
		StartPoint: startPoint,
		EndPoint:   endPoint,
		StartedAt:  startedAt,
		FinishedAt: nil,
	}, nil
}

func (t *Trip) Finish(at time.Time) error {
	if t.Status != StatusActive {
		return ErrTripCompleted
	}

	t.Status = StatusCompleted
	t.FinishedAt = &at
	return nil
}

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)

type Coordinate struct {
	Latitude  float64
	Longitude float64
}

func (c Coordinate) isValid() bool {
	return c.Latitude >= -90 && c.Latitude <= 90 &&
		c.Longitude >= -180 && c.Longitude <= 180
}
