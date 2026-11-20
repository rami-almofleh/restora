package offsite

import (
	"errors"
	"log/slog"
	"net/http"

	"restora.local/restora/internal/server"
)

// RegisterRecords adds the export endpoints to mux:
//
//	GET /api/v1/exports?application=&location=&limit=&cursor=
//	GET /api/v1/exports/{id}
//	GET /api/v1/export-applications
func RegisterRecords(mux *http.ServeMux, svc *RecordService, log *slog.Logger) {
	mux.HandleFunc("GET /api/v1/exports", func(w http.ResponseWriter, r *http.Request) {
		limit, ok := server.ParseLimit(w, r)
		if !ok {
			return
		}
		q := r.URL.Query()
		page, err := svc.List(r.Context(), q.Get("application"), q.Get("location"), limit, q.Get("cursor"))
		if err != nil {
			writeRecordFailure(w, log, "list exports", err)
			return
		}
		server.WriteJSON(w, http.StatusOK, page)
	})

	mux.HandleFunc("GET /api/v1/exports/{id}", func(w http.ResponseWriter, r *http.Request) {
		view, err := svc.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeRecordFailure(w, log, "get export", err)
			return
		}
		server.WriteJSON(w, http.StatusOK, view)
	})

	mux.HandleFunc("GET /api/v1/export-applications", func(w http.ResponseWriter, r *http.Request) {
		apps, err := svc.Applications(r.Context())
		if err != nil {
			writeRecordFailure(w, log, "list exported applications", err)
			return
		}
		server.WriteJSON(w, http.StatusOK, map[string]any{"items": apps})
	})
}

func writeRecordFailure(w http.ResponseWriter, log *slog.Logger, action string, err error) {
	if errors.Is(err, ErrNotFound) {
		server.WriteError(w, http.StatusNotFound, "export_not_found",
			"This stored backup does not exist.", "It may have been removed. Go back to the list and refresh it.")
		return
	}
	log.Error(action, "error", err)
	server.WriteError(w, http.StatusBadGateway, "cluster_unreachable",
		"Restora could not read the stored backups.", "Check that the Restora service account has read access and that the cluster is reachable.")
}
