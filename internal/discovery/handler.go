package discovery

import (
	"errors"
	"log/slog"
	"net/http"

	"restora.local/restora/internal/server"
)

// Register adds the application endpoints to mux:
//
//	GET /api/v1/applications?q=&volumes=&limit=&cursor=
//	GET /api/v1/applications/{name}
func Register(mux *http.ServeMux, svc *Service, log *slog.Logger) {
	mux.HandleFunc("GET /api/v1/applications", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		volumes := query.Get("volumes")
		if volumes != "" && volumes != "with" && volumes != "without" {
			server.WriteError(w, http.StatusBadRequest, "invalid_filter",
				"The filter \"volumes\" must be \"with\" or \"without\".", "Remove the filter or use one of the two values.")
			return
		}
		limit, ok := server.ParseLimit(w, r)
		if !ok {
			return
		}

		page, err := svc.List(r.Context(), Filter{Query: query.Get("q"), Volumes: volumes}, limit, query.Get("cursor"))
		if err != nil {
			writeClusterError(w, log, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, page)
	})

	mux.HandleFunc("GET /api/v1/applications/{name}", func(w http.ResponseWriter, r *http.Request) {
		detail, err := svc.Get(r.Context(), r.PathValue("name"))
		switch {
		case errors.Is(err, ErrNotFound):
			server.WriteError(w, http.StatusNotFound, "application_not_found",
				"This application does not exist.", "It may have been deleted. Go back to the list and refresh it.")
		case err != nil:
			writeClusterError(w, log, err)
		default:
			server.WriteJSON(w, http.StatusOK, detail)
		}
	})
}

func writeClusterError(w http.ResponseWriter, log *slog.Logger, err error) {
	log.Error("read cluster", "error", err)
	server.WriteError(w, http.StatusBadGateway, "cluster_unreachable",
		"Restora could not read the cluster.", "Check that the Restora service account has read access and that the cluster is reachable.")
}
