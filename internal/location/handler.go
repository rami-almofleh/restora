package location

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"restora.local/restora/internal/server"
)

// maxBody bounds the request body; a location is small.
const maxBody = 64 << 10

// Register adds the location endpoints to mux:
//
//	GET    /api/v1/locations?limit=&cursor=
//	POST   /api/v1/locations
//	GET    /api/v1/locations/{id}
//	PUT    /api/v1/locations/{id}
//	DELETE /api/v1/locations/{id}
//	POST   /api/v1/locations/{id}/test
func Register(mux *http.ServeMux, svc *Service, log *slog.Logger) {
	mux.HandleFunc("GET /api/v1/locations", func(w http.ResponseWriter, r *http.Request) {
		limit, ok := server.ParseLimit(w, r)
		if !ok {
			return
		}
		page, err := svc.List(r.Context(), limit, r.URL.Query().Get("cursor"))
		if err != nil {
			writeFailure(w, log, "list locations", err)
			return
		}
		server.WriteJSON(w, http.StatusOK, page)
	})

	mux.HandleFunc("POST /api/v1/locations", func(w http.ResponseWriter, r *http.Request) {
		in, ok := readInput(w, r)
		if !ok {
			return
		}
		view, err := svc.Create(r.Context(), in)
		if err != nil {
			writeFailure(w, log, "create location", err)
			return
		}
		server.WriteJSON(w, http.StatusCreated, view)
	})

	mux.HandleFunc("GET /api/v1/locations/{id}", func(w http.ResponseWriter, r *http.Request) {
		view, err := svc.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeFailure(w, log, "get location", err)
			return
		}
		server.WriteJSON(w, http.StatusOK, view)
	})

	mux.HandleFunc("PUT /api/v1/locations/{id}", func(w http.ResponseWriter, r *http.Request) {
		in, ok := readInput(w, r)
		if !ok {
			return
		}
		view, err := svc.Update(r.Context(), r.PathValue("id"), in)
		if err != nil {
			writeFailure(w, log, "update location", err)
			return
		}
		server.WriteJSON(w, http.StatusOK, view)
	})

	mux.HandleFunc("DELETE /api/v1/locations/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := svc.Delete(r.Context(), r.PathValue("id")); err != nil {
			writeFailure(w, log, "delete location", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/v1/locations/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		view, err := svc.RequestTest(r.Context(), r.PathValue("id"))
		if err != nil {
			writeFailure(w, log, "request test", err)
			return
		}
		server.WriteJSON(w, http.StatusAccepted, view)
	})
}

func readInput(w http.ResponseWriter, r *http.Request) (Input, bool) {
	var in Input
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid_body",
			"The request body is not a valid location.", "Send JSON with name, type and the settings of the type.")
		return Input{}, false
	}
	return in, true
}

func writeFailure(w http.ResponseWriter, log *slog.Logger, action string, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		server.WriteError(w, http.StatusNotFound, "location_not_found",
			"This storage location does not exist.", "It may have been deleted. Go back to the list and refresh it.")
	case errors.Is(err, ErrNameTaken):
		server.WriteError(w, http.StatusConflict, "location_name_taken",
			"Another storage location already has this name.", "Choose a different name.")
	case errors.Is(err, ErrTargetTaken):
		server.WriteError(w, http.StatusConflict, "location_target_taken",
			"Another storage location already writes to this bucket folder or share.",
			"Use a different bucket or folder in the bucket. A second copy only helps when it lives in a separate place.")
	case errors.Is(err, ErrInUse):
		server.WriteError(w, http.StatusConflict, "location_in_use",
			"A policy still copies backups to this storage location.", "Remove the location from the policy or delete the policy first, then delete the location.")
	case errors.Is(err, ErrInvalid):
		server.WriteError(w, http.StatusBadRequest, "invalid_location", err.Error(),
			"Check the marked field and try again.")
	default:
		// The error may mention the secret's name, never its content.
		log.Error(action, "error", err)
		server.WriteError(w, http.StatusBadGateway, "cluster_unreachable",
			"Restora could not reach the cluster.", "Check that the Restora service account may manage storage locations and their secrets and that the cluster is reachable.")
	}
}
