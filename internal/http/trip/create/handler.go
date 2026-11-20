package create

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/http/domainerror"
)

const maxBodyBytes = 1 << 20

type Handler struct {
	usecase tripCreator
}

func New(usecase tripCreator) *Handler {
	return &Handler{usecase: usecase}
}

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, _ api.CreateTripParams) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		domainerror.WriteInvalidRequest(w, r, "request body is too large or unreadable")
		return
	}

	req, err := parseCreateTripRequest(body)
	if err != nil {
		domainerror.WriteInvalidRequest(w, r, err.Error())
		return
	}

	tr, err := h.usecase.Create(r.Context(), toInput(req))
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/api/v1/trips/"+tr.ID.String())
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toAPI(tr))
}
