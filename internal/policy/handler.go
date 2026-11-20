package policy

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"restora.local/restora/internal/server"
)

// maxBody bounds the request body; a policy is small.
const maxBody = 64 << 10

// Register adds the policy endpoints to mux:
//
//	GET    /api/v1/policies?limit=&cursor=
//	POST   /api/v1/policies
//	GET    /api/v1/policies/{id}
//	PUT    /api/v1/policies/{id}
//	DELETE /api/v1/policies/{id}
//	POST   /api/v1/policies/{id}/pause
//	POST   /api/v1/policies/{id}/resume
//	POST   /api/v1/policies/{id}/run
//	POST   /api/v1/policies/preview
//	POST   /api/v1/policies/{id}/retention-preview
func Register(mux *http.ServeMux, svc *Service, log *slog.Logger) {
	mux.HandleFunc("GET /api/v1/policies", func(w http.ResponseWriter, r *http.Request) {
		limit, ok := server.ParseLimit(w, r)
		if !ok {
			return
		}
		page, err := svc.List(r.Context(), limit, r.URL.Query().Get("cursor"))
		if err != nil {
			writeFailure(w, log, "list policies", err)
			return
		}
		server.WriteJSON(w, http.StatusOK, page)
	})

	mux.HandleFunc("POST /api/v1/policies", func(w http.ResponseWriter, r *http.Request) {
		in, ok := readInput(w, r)
		if !ok {
			return
		}
		view, err := svc.Create(r.Context(), in)
		if err != nil {
			writeFailure(w, log, "create policy", err)
			return
		}
		server.WriteJSON(w, http.StatusCreated, view)
	})

	mux.HandleFunc("POST /api/v1/policies/preview", func(w http.ResponseWriter, r *http.Request) {
		in, ok := readInput(w, r)
		if !ok {
			return
		}
		preview, err := svc.Preview(r.Context(), in)
		if err != nil {
			writeFailure(w, log, "preview policy", err)
			return
		}
		server.WriteJSON(w, http.StatusOK, preview)
	})

	mux.HandleFunc("POST /api/v1/policies/{id}/retention-preview", func(w http.ResponseWriter, r *http.Request) {
		in, ok := readInput(w, r)
		if !ok {
			return
		}
		plan, err := svc.RetentionPreview(r.Context(), r.PathValue("id"), in)
		if err != nil {
			writeFailure(w, log, "preview retention", err)
			return
		}
		server.WriteJSON(w, http.StatusOK, plan)
	})

	mux.HandleFunc("POST /api/v1/policies/{id}/run", func(w http.ResponseWriter, r *http.Request) {
		result, err := svc.RunNow(r.Context(), r.PathValue("id"))
		if err != nil {
			writeFailure(w, log, "run policy", err)
			return
		}
		server.WriteJSON(w, http.StatusAccepted, LastRun{At: result.At.UTC(), Snapshots: result.Snapshots, Skipped: result.Skipped})
	})

	mux.HandleFunc("GET /api/v1/policies/{id}", func(w http.ResponseWriter, r *http.Request) {
		view, err := svc.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeFailure(w, log, "get policy", err)
			return
		}
		server.WriteJSON(w, http.StatusOK, view)
	})

	mux.HandleFunc("PUT /api/v1/policies/{id}", func(w http.ResponseWriter, r *http.Request) {
		in, ok := readInput(w, r)
		if !ok {
			return
		}
		view, err := svc.Update(r.Context(), r.PathValue("id"), in)
		if err != nil {
			writeFailure(w, log, "update policy", err)
			return
		}
		server.WriteJSON(w, http.StatusOK, view)
	})

	mux.HandleFunc("DELETE /api/v1/policies/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := svc.Delete(r.Context(), r.PathValue("id")); err != nil {
			writeFailure(w, log, "delete policy", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	for path, paused := range map[string]bool{"pause": true, "resume": false} {
		mux.HandleFunc("POST /api/v1/policies/{id}/"+path, func(w http.ResponseWriter, r *http.Request) {
			view, err := svc.SetPaused(r.Context(), r.PathValue("id"), paused)
			if err != nil {
				writeFailure(w, log, path+" policy", err)
				return
			}
			server.WriteJSON(w, http.StatusOK, view)
		})
	}
}

func readInput(w http.ResponseWriter, r *http.Request) (Input, bool) {
	var in Input
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		server.WriteError(w, http.StatusBadRequest, "invalid_body",
			"The request body is not a valid policy.", "Send JSON with name, selector and schedule.")
		return Input{}, false
	}
	return in, true
}

func writeFailure(w http.ResponseWriter, log *slog.Logger, action string, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		server.WriteError(w, http.StatusNotFound, "policy_not_found",
			"This policy does not exist.", "It may have been deleted. Go back to the list and refresh it.")
	case errors.Is(err, ErrNameTaken):
		server.WriteError(w, http.StatusConflict, "policy_name_taken",
			"Another policy already has this name.", "Choose a different name.")
	case errors.Is(err, ErrInvalidName):
		server.WriteError(w, http.StatusBadRequest, "invalid_name", err.Error(), "Give the policy a short name.")
	case errors.Is(err, ErrInvalidSchedule):
		server.WriteError(w, http.StatusBadRequest, ErrorInvalidSchedule, err.Error(),
			"Pick a preset or write five cron fields, for example */15 * * * * for every 15 minutes.")
	case errors.Is(err, ErrInvalidExport):
		server.WriteError(w, http.StatusBadRequest, "invalid_export", err.Error(),
			"Choose storage locations from the list, or none to keep the backups in the cluster.")
	case errors.Is(err, ErrInvalidRetention):
		server.WriteError(w, http.StatusBadRequest, ErrorInvalidRetention, err.Error(),
			"Use whole numbers from 0 to 1000. Leave all at 0 to keep every backup.")
	case errors.Is(err, ErrInvalidSelector):
		server.WriteError(w, http.StatusBadRequest, ErrorInvalidSelector, err.Error(),
			"Choose applications, give labels, or select all applications.")
	default:
		log.Error(action, "error", err)
		server.WriteError(w, http.StatusBadGateway, "cluster_unreachable",
			"Restora could not reach the cluster.", "Check that the Restora service account may manage policies and that the cluster is reachable.")
	}
}
