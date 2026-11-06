package finish

import (
	"encoding/json"
	"net/http"

	"github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/http/domainerror"
)

type Handler struct {
	usecase tripFinisher
}

func New(usecase tripFinisher) *Handler {
	return &Handler{usecase: usecase}
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	tr, err := h.usecase.Finish(r.Context(), tripId)
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(toAPI(tr))
}
