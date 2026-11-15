package finish_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/http/trip/finish"
)

const secret = "password=s3cret host=10.0.0.5"

var (
	tripID = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	msk    = time.FixedZone("MSK", 3*60*60)
)

func completedTrip() *trip.Trip {
	finishedAt := time.Date(2026, 8, 24, 15, 18, 0, 0, msk)
	return &trip.Trip{
		ID:         tripID,
		UserID:     uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		DriverID:   uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		Price:      1450,
		Status:     trip.StatusCompleted,
		StartPoint: trip.Coordinate{Latitude: 59.9398, Longitude: 30.3146},
		EndPoint:   trip.Coordinate{Latitude: 59.9290, Longitude: 30.3626},
		StartedAt:  time.Date(2026, 8, 24, 15, 0, 0, 0, msk),
		FinishedAt: &finishedAt,
	}
}

func TestHandler_FinishTrip(t *testing.T) {
	tests := []struct {
		name       string
		ucTrip     *trip.Trip
		ucErr      error
		wantStatus int
		wantCode   string
	}{
		{name: "active trip finished", ucTrip: completedTrip(), wantStatus: http.StatusOK},
		{
			name:       "trip already completed",
			ucErr:      fmt.Errorf("finish trip: %w", trip.ErrTripCompleted),
			wantStatus: http.StatusConflict,
			wantCode:   "trip_completed",
		},
		{
			name:       "trip not found",
			ucErr:      fmt.Errorf("finish trip: %w", trip.ErrTripNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
		},
		{
			name:       "repository error",
			ucErr:      fmt.Errorf("finish trip: %w", errors.New(secret)),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMocktripFinisher(ctrl)
			uc.EXPECT().Finish(gomock.Any(), tripID).Return(tt.ucTrip, tt.ucErr)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+tripID.String()+"/finish", nil)

			finish.New(uc).FinishTrip(w, r, tripID)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "s3cret") || strings.Contains(w.Body.String(), "10.0.0.5") {
				t.Fatalf("response leaks internal error details: %s", w.Body.String())
			}

			if tt.wantCode != "" {
				assertProblem(t, w, tt.wantCode)
				return
			}
			assertTripResponse(t, w, tt.ucTrip)
		})
	}
}

func assertTripResponse(t *testing.T, w *httptest.ResponseRecorder, want *trip.Trip) {
	t.Helper()

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	raw := w.Body.String()

	var got api.Trip
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode trip: %v", err)
	}

	if got.Id != want.ID || got.UserId != want.UserID || got.DriverId != want.DriverID {
		t.Errorf("ids = (%v, %v, %v), want (%v, %v, %v)",
			got.Id, got.UserId, got.DriverId, want.ID, want.UserID, want.DriverID)
	}
	if got.StartPoint != api.Coordinates(want.StartPoint) || got.EndPoint != api.Coordinates(want.EndPoint) {
		t.Errorf("points = %+v -> %+v, want %+v -> %+v", got.StartPoint, got.EndPoint, want.StartPoint, want.EndPoint)
	}
	if got.Price != want.Price || string(got.Status) != string(want.Status) {
		t.Errorf("price, status = %d, %q; want %d, %q", got.Price, got.Status, want.Price, want.Status)
	}
	if !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("started_at = %v, want %v", got.StartedAt, want.StartedAt)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(*want.FinishedAt) {
		t.Errorf("finished_at = %v, want %v", got.FinishedAt, *want.FinishedAt)
	}
	if !strings.Contains(raw, `"finished_at":"`+want.FinishedAt.UTC().Format(time.RFC3339)+`"`) {
		t.Errorf("finished_at must be in UTC (…Z): %s", raw)
	}
	if got.LastPositionAt != nil {
		t.Errorf("last_position_at = %v, want absent", got.LastPositionAt)
	}
}

func assertProblem(t *testing.T, w *httptest.ResponseRecorder, wantCode string) {
	t.Helper()

	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", ct)
	}
	var p api.Problem
	if err := json.NewDecoder(w.Body).Decode(&p); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if p.Code != wantCode {
		t.Errorf("code = %q, want %q", p.Code, wantCode)
	}
}
