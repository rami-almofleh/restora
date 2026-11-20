package restore

import (
	"errors"
	"log/slog"
	"net/http"

	"restora.local/restora/internal/offsite"
	"restora.local/restora/internal/server"
	"restora.local/restora/internal/snapshot"
)

// Register adds the restore endpoints to mux:
//
//	POST /api/v1/snapshots/{id}/restores
//	POST /api/v1/exports/{id}/restores
//	GET  /api/v1/applications/{name}/restores?limit=&cursor=
func Register(mux *http.ServeMux, svc *Service, log *slog.Logger) {
	mux.HandleFunc("POST /api/v1/snapshots/{id}/restores", func(w http.ResponseWriter, r *http.Request) {
		view, err := svc.Create(r.Context(), r.PathValue("id"))
		switch {
		case errors.Is(err, snapshot.ErrNotFound):
			server.WriteError(w, http.StatusNotFound, "snapshot_not_found",
				"This backup does not exist.", "It may have been deleted. Go back to the list and refresh it.")
		case errors.Is(err, ErrSnapshotNotReady):
			server.WriteError(w, http.StatusConflict, "snapshot_not_ready",
				"This backup is not complete, so it cannot be restored.", "Pick a backup that finished successfully.")
		case errors.Is(err, ErrInProgress):
			server.WriteError(w, http.StatusConflict, "restore_in_progress",
				"A restore of this application is already running.", "Wait until it has finished.")
		case errors.Is(err, ErrSnapshotRunning):
			server.WriteError(w, http.StatusConflict, "snapshot_in_progress",
				"A backup of this application is still running.", "Wait until it has finished, then restore.")
		case err != nil:
			log.Error("create restore", "snapshot", r.PathValue("id"), "error", err)
			server.WriteError(w, http.StatusBadGateway, "cluster_unreachable",
				"Restora could not start the restore.", "Check that the Restora service account may create restore requests and that the cluster is reachable.")
		default:
			server.WriteJSON(w, http.StatusAccepted, view)
		}
	})

	mux.HandleFunc("POST /api/v1/exports/{id}/restores", func(w http.ResponseWriter, r *http.Request) {
		view, err := svc.CreateFromExport(r.Context(), r.PathValue("id"))
		switch {
		case errors.Is(err, offsite.ErrNotFound):
			server.WriteError(w, http.StatusNotFound, "export_not_found",
				"This stored backup does not exist.", "It may have been removed. Go back to the list and refresh it.")
		case errors.Is(err, ErrInProgress):
			server.WriteError(w, http.StatusConflict, "restore_in_progress",
				"A restore of this application is already running.", "Wait until it has finished.")
		case errors.Is(err, ErrSnapshotRunning):
			server.WriteError(w, http.StatusConflict, "snapshot_in_progress",
				"A backup of this application is still running.", "Wait until it has finished, then restore.")
		case err != nil:
			log.Error("create restore from export", "export", r.PathValue("id"), "error", err)
			server.WriteError(w, http.StatusBadGateway, "cluster_unreachable",
				"Restora could not start the restore.", "Check that the Restora service account may create restore requests and that the cluster is reachable.")
		default:
			server.WriteJSON(w, http.StatusAccepted, view)
		}
	})

	mux.HandleFunc("GET /api/v1/applications/{name}/restores", func(w http.ResponseWriter, r *http.Request) {
		limit, ok := server.ParseLimit(w, r)
		if !ok {
			return
		}
		page, err := svc.List(r.Context(), r.PathValue("name"), limit, r.URL.Query().Get("cursor"))
		if err != nil {
			log.Error("list restores", "application", r.PathValue("name"), "error", err)
			server.WriteError(w, http.StatusBadGateway, "cluster_unreachable",
				"Restora could not read the restores.", "Check that the Restora service account has read access and that the cluster is reachable.")
			return
		}
		server.WriteJSON(w, http.StatusOK, page)
	})
}
