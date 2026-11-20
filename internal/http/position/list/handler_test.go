package list_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/http/position/list"
)

func TestListTripPositions(t *testing.T) {
	id := uuid.New()
	at := time.Date(2026, 8, 24, 13, 0, 0, 0, time.FixedZone("UTC+1", 3600))
	point := trip.Position{ID: 42, TripID: id, RecordedAt: at, Coordinates: trip.Coordinate{Latitude: 1, Longitude: 2}}
	for _, tc := range []struct {
		name   string
		points []trip.Position
		err    error
		status int
		code   string
	}{
		{"empty route", nil, nil, 200, ""},
		{"recorded route", []trip.Position{point}, nil, 200, ""},
		{"unknown trip", nil, trip.ErrTripNotFound, 404, "trip_not_found"},
		{"read failure", nil, errors.New("read failed"), 500, "internal_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uc := NewMockpositionLister(gomock.NewController(t))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			uc.EXPECT().ListPositions(req.Context(), id).Return(tc.points, tc.err)
			w := httptest.NewRecorder()
			list.New(uc).ListTripPositions(w, req, id)
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.status, w.Body)
			}
			if tc.err != nil {
				var p api.Problem
				if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil || p.Code != tc.code || w.Header().Get("Content-Type") != "application/problem+json" {
					t.Fatalf("unexpected problem: %s", w.Body)
				}
				return
			}
			var got api.TripPositionsResponse
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Positions == nil || len(got.Positions) != len(tc.points) || w.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected route: %s", w.Body)
			}
			for i, p := range got.Positions {
				want := tc.points[i]
				if p.Id != want.ID || p.TripId != id || p.Coordinates != api.Coordinates(want.Coordinates) || !p.RecordedAt.Equal(want.RecordedAt) || p.RecordedAt.Location() != time.UTC {
					t.Errorf("unexpected response position: %+v", p)
				}
			}
		})
	}
}
