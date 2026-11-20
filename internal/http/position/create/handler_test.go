package create_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/http/position/create"
	"github.com/ArtemIushin123/template/internal/model"
)

func TestCreateTripPosition(t *testing.T) {
	const invalidRequest = "invalid_request"
	const body = `{"coordinates":{"latitude":0,"longitude":11.5},"recorded_at":"2026-08-24T12:00:00Z"}`
	id := uuid.New()
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	in := model.CreatePositionInput{TripID: id, Coordinates: trip.Coordinate{Longitude: 11.5}, RecordedAt: at}
	for _, tc := range []struct {
		name, body string
		err        error
		status     int
		code       string
	}{
		{"valid", body, nil, 201, ""},
		{"missing latitude", strings.Replace(body, `"latitude":0,`, "", 1), nil, 400, invalidRequest},
		{"null latitude", strings.Replace(body, `"latitude":0`, `"latitude":null`, 1), nil, 400, invalidRequest},
		{"unknown field", strings.Replace(body, "latitude", "Latitude", 1), nil, 400, invalidRequest},
		{"missing time", `{"coordinates":{"latitude":0,"longitude":0}}`, nil, 400, invalidRequest},
		{"malformed time", strings.Replace(body, "2026-08-24T12:00:00Z", "yesterday", 1), nil, 400, invalidRequest},
		{"invalid json", body + "{}", nil, 400, invalidRequest},
		{"range rejected", body, &trip.ValidationError{Field: "coordinates"}, 400, invalidRequest},
		{"missing trip", body, trip.ErrTripNotFound, 404, "trip_not_found"},
		{"completed", body, trip.ErrTripCompleted, 409, "trip_completed"},
		{"database error", body, errors.New("private database error"), 500, "internal_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uc := NewMockpositionCreator(gomock.NewController(t))
			if tc.body == body {
				uc.EXPECT().CreatePosition(gomock.Any(), in).Return(&trip.Position{ID: 42, TripID: id, Coordinates: in.Coordinates, RecordedAt: at}, tc.err)
			}
			w := httptest.NewRecorder()
			create.New(uc).CreateTripPosition(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body)), id)
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.status, w.Body)
			}
			if tc.status == http.StatusCreated {
				var got api.TripPosition
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.Id != 42 || got.TripId != id || !got.RecordedAt.Equal(at) || got.Coordinates != api.Coordinates(in.Coordinates) {
					t.Fatalf("position = %+v", got)
				}
				return
			}
			var p api.Problem
			if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			if w.Header().Get("Content-Type") != "application/problem+json" || p.Code != tc.code || strings.Contains(w.Body.String(), "private database error") {
				t.Fatalf("unexpected problem: %s", w.Body)
			}
		})
	}
}
