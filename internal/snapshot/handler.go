package snapshot

import (
	"errors"
	"log/slog"
	"net/http"

	"restora.local/restora/internal/discovery"
	"restora.local/restora/internal/server"
)

// Register adds the snapshot endpoints to mux:
//
//	POST /api/v1/applications/{name}/snapshots
//	GET  /api/v1/applications/{name}/snapshots?limit=&cursor=
//	DELETE /api/v1/snapshots/{id}
//	POST /api/v1/snapshots/{id}/exports/{location}/retry
func Register(mux *http.ServeMux, svc *Service, log *slog.Logger) {
	mux.HandleFunc("POST /api/v1/applications/{name}/snapshots", func(w http.ResponseWriter, r *http.Request) {
		view, err := svc.Create(r.Context(), r.PathValue("name"))
		switch {
		case errors.Is(err, discovery.ErrNotFound):
			server.WriteError(w, http.StatusNotFound, "application_not_found",
				"This application does not exist.", "It may have been deleted. Go back to the list and refresh it.")
		case errors.Is(err, ErrInProgress):
			server.WriteError(w, http.StatusConflict, "snapshot_in_progress",
				"A backup of this application is already running.", "Wait until it has finished, then start the next one.")
		case errors.Is(err, ErrBlocked):
			server.WriteError(w, http.StatusConflict, "restore_in_progress",
				"A restore of this application is running.", "Wait until it has finished, then start the backup.")
		case err != nil:
			log.Error("create snapshot", "application", r.PathValue("name"), "error", err)
			server.WriteError(w, http.StatusBadGateway, "cluster_unreachable",
				"Restora could not start the backup.", "Check that the Restora service account may create snapshots and that the cluster is reachable.")
		default:
			server.WriteJSON(w, http.StatusAccepted, view)
		}
	})

	mux.HandleFunc("GET /api/v1/applications/{name}/snapshots", func(w http.ResponseWriter, r *http.Request) {
		limit, ok := server.ParseLimit(w, r)
		if !ok {
			return
		}
		page, err := svc.List(r.Context(), r.PathValue("name"), limit, r.URL.Query().Get("cursor"))
		if err != nil {
			log.Error("list snapshots", "application", r.PathValue("name"), "error", err)
			server.WriteError(w, http.StatusBadGateway, "cluster_unreachable",
				"Restora could not read the backups.", "Check that the Restora service account has read access and that the cluster is reachable.")
			return
		}
		server.WriteJSON(w, http.StatusOK, page)
	})

	mux.HandleFunc("DELETE /api/v1/snapshots/{id}", func(w http.ResponseWriter, r *http.Request) {
		err := svc.Delete(r.Context(), r.PathValue("id"))
		switch {
		case errors.Is(err, ErrNotFound):
			server.WriteError(w, http.StatusNotFound, "snapshot_not_found",
				"This backup does not exist.", "It may have been deleted already. Go back to the list and refresh it.")
		case errors.Is(err, ErrRunning):
			server.WriteError(w, http.StatusConflict, "snapshot_running",
				"This backup is still being created, so it cannot be deleted.", "Wait until it has finished, then delete it.")
		case errors.Is(err, ErrInUse):
			server.WriteError(w, http.StatusConflict, "snapshot_in_use",
				"A restore is using this backup right now.", "Wait until the restore has finished, then delete the backup.")
		case err != nil:
			log.Error("delete snapshot", "snapshot", r.PathValue("id"), "error", err)
			server.WriteError(w, http.StatusBadGateway, "cluster_unreachable",
				"Restora could not delete the backup.", "Check that the Restora service account may delete snapshots and that the cluster is reachable.")
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})

	mux.HandleFunc("POST /api/v1/snapshots/{id}/exports/{location}/retry", func(w http.ResponseWriter, r *http.Request) {
		view, err := svc.RetryExport(r.Context(), r.PathValue("id"), r.PathValue("location"))
		switch {
		case errors.Is(err, ErrNotFound):
			server.WriteError(w, http.StatusNotFound, "snapshot_not_found",
				"This backup does not exist.", "It may have been deleted already. Go back to the list and refresh it.")
		case errors.Is(err, ErrNotRetryable):
			server.WriteError(w, http.StatusConflict, "export_not_retryable",
				"This copy did not fail, so there is nothing to repeat.", "Refresh the list to see the current state.")
		case err != nil:
			log.Error("retry export", "snapshot", r.PathValue("id"), "location", r.PathValue("location"), "error", err)
			server.WriteError(w, http.StatusBadGateway, "cluster_unreachable",
				"Restora could not start the copy again.", "Check that the Restora service account may update snapshots and that the cluster is reachable.")
		default:
			server.WriteJSON(w, http.StatusAccepted, view)
		}
	})
}
