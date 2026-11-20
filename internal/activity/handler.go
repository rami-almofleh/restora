package activity

import (
	"log/slog"
	"net/http"

	"restora.local/restora/internal/server"
)

// Register adds the activity endpoint to mux:
//
//	GET /api/v1/activities?type=&status=&q=&limit=&cursor=
//
// type is backup or restore, status is active, succeeded or failed.
func Register(mux *http.ServeMux, svc *Service, log *slog.Logger) {
	mux.HandleFunc("GET /api/v1/activities", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		filter := Filter{Type: Type(query.Get("type")), Status: Status(query.Get("status")), Query: query.Get("q")}

		switch filter.Type {
		case "", TypeBackup, TypeRestore:
		default:
			server.WriteError(w, http.StatusBadRequest, "invalid_filter",
				"The filter \"type\" must be \"backup\" or \"restore\".", "Remove the filter or use one of the two values.")
			return
		}
		switch filter.Status {
		case "", StatusActive, StatusSucceeded, StatusFailed:
		default:
			server.WriteError(w, http.StatusBadRequest, "invalid_filter",
				"The filter \"status\" must be \"active\", \"succeeded\" or \"failed\".", "Remove the filter or use one of the three values.")
			return
		}
		limit, ok := server.ParseLimit(w, r)
		if !ok {
			return
		}

		page, err := svc.List(r.Context(), filter, limit, query.Get("cursor"))
		if err != nil {
			log.Error("list activities", "error", err)
			server.WriteError(w, http.StatusBadGateway, "cluster_unreachable",
				"Restora could not read the activity.", "Check that the Restora service account has read access and that the cluster is reachable.")
			return
		}
		server.WriteJSON(w, http.StatusOK, page)
	})
}
