package create

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/model"
)

var (
	coordinateKeys = map[string]bool{"latitude": true, "longitude": true}
	tripKeys       = map[string]bool{
		"driver_id": true, "user_id": true, "start_point": true,
		"end_point": true, "price": true,
	}
)

type coordinateRequest struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

type tripRequest struct {
	DriverID   *uuid.UUID         `json:"driver_id"`
	UserID     *uuid.UUID         `json:"user_id"`
	StartPoint *coordinateRequest `json:"start_point"`
	EndPoint   *coordinateRequest `json:"end_point"`
	Price      *int64             `json:"price"`
}

func checkKeys(raw []byte, allowed map[string]bool) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage

	if err := json.Unmarshal(raw, &m); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			return nil, errors.New("must be a JSON object")
		}
		return nil, errors.New("malformed JSON")
	}

	for k := range m {
		if !allowed[k] {
			return nil, fmt.Errorf("unknown field %q", k)
		}
	}
	return m, nil
}

func parseCreateTripRequest(body []byte) (*tripRequest, error) {
	top, err := checkKeys(body, tripKeys)
	if err != nil {
		return nil, err
	}

	if raw, ok := top["start_point"]; ok {
		if _, err := checkKeys(raw, coordinateKeys); err != nil {
			return nil, fmt.Errorf("start_point: %w", err)
		}
	}

	if raw, ok := top["end_point"]; ok {
		if _, err := checkKeys(raw, coordinateKeys); err != nil {
			return nil, fmt.Errorf("end_point: %w", err)
		}
	}

	var req tripRequest
	if err := json.Unmarshal(body, &req); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			return nil, fmt.Errorf("%s has invalid value", typeErr.Field)
		}
		return nil, errors.New("invalid field value")
	}

	if req.DriverID == nil {
		return nil, errors.New("driver_id is required")
	}
	if req.UserID == nil {
		return nil, errors.New("user_id is required")
	}
	if req.Price == nil {
		return nil, errors.New("price is required")
	}
	if req.StartPoint == nil {
		return nil, errors.New("start_point is required")
	}
	if req.StartPoint.Latitude == nil {
		return nil, errors.New("start_point.latitude is required")
	}
	if req.StartPoint.Longitude == nil {
		return nil, errors.New("start_point.longitude is required")
	}
	if req.EndPoint == nil {
		return nil, errors.New("end_point is required")
	}
	if req.EndPoint.Latitude == nil {
		return nil, errors.New("end_point.latitude is required")
	}
	if req.EndPoint.Longitude == nil {
		return nil, errors.New("end_point.longitude is required")
	}
	return &req, nil
}

func toInput(req *tripRequest) model.CreateTripInput {
	return model.CreateTripInput{
		DriverID:   *req.DriverID,
		UserID:     *req.UserID,
		Price:      *req.Price,
		StartPoint: toCoordinate(req.StartPoint),
		EndPoint:   toCoordinate(req.EndPoint),
	}
}

func toCoordinate(c *coordinateRequest) trip.Coordinate {
	return trip.Coordinate{Latitude: *c.Latitude, Longitude: *c.Longitude}
}

func toAPI(tr *trip.Trip) api.Trip {
	return api.Trip{
		Id:             tr.ID,
		UserId:         tr.UserID,
		DriverId:       tr.DriverID,
		StartPoint:     api.Coordinates(tr.StartPoint),
		EndPoint:       api.Coordinates(tr.EndPoint),
		Price:          tr.Price,
		Status:         api.TripStatus(tr.Status),
		StartedAt:      tr.StartedAt.UTC(),
		FinishedAt:     utcOrNil(tr.FinishedAt),
		LastPositionAt: nil,
	}
}

func utcOrNil(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}
