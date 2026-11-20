package list

import (
	"encoding/json"
	"net/http"

	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/http/domainerror"
	"github.com/ArtemIushin123/template/internal/http/position"
)

type Handler struct {
	usecase positionLister
}

func New(usecase positionLister) *Handler {
	return &Handler{usecase: usecase}
}

func (h *Handler) ListTripPositions(w http.ResponseWriter, r *http.Request, id api.TripId) {
	points, err := h.usecase.ListPositions(r.Context(), id)
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}

	positions := make([]api.TripPosition, len(points))
	for i, point := range points {
		positions[i] = position.ToAPI(point)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.TripPositionsResponse{Positions: positions})
}
