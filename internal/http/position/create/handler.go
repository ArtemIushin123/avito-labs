package create

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/http/domainerror"
	"github.com/ArtemIushin123/template/internal/http/position"
	"github.com/ArtemIushin123/template/internal/model"
)

type Handler struct{ uc positionCreator }

func New(uc positionCreator) *Handler { return &Handler{uc: uc} }
func (h *Handler) CreateTripPosition(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		domainerror.WriteInvalidRequest(w, r, "request body is too large or unreadable")
		return
	}
	data, err := parsePosition(body)
	if err != nil {
		domainerror.WriteInvalidRequest(w, r, err.Error())
		return
	}
	p, err := h.uc.CreatePosition(r.Context(), model.CreatePositionInput{TripID: tripID, Coordinates: trip.Coordinate(data.Coordinates), RecordedAt: data.RecordedAt})
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(position.ToAPI(*p))
}
