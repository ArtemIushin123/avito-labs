package trip

import (
	"time"

	"github.com/google/uuid"
)

type Position struct {
	ID          int64
	TripID      uuid.UUID
	Coordinates Coordinate
	RecordedAt  time.Time
	CreatedAt   time.Time
}

func NewPosition(tripID uuid.UUID, coordinates Coordinate, recordedAt, now time.Time) (*Position, error) {
	if !coordinates.isValid() {
		return nil, &ValidationError{Field: "coordinates", Message: coordinateRangeMessage}
	}
	if recordedAt.After(now.Add(time.Minute)) {
		return nil, &ValidationError{Field: "recorded_at", Message: "must not be more than one minute in the future"}
	}
	return &Position{TripID: tripID, Coordinates: coordinates, RecordedAt: recordedAt.UTC()}, nil
}
