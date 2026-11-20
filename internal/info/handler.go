package info

import (
	"net/http"

	"restora.local/restora/internal/server"
)

// Register adds GET /api/v1/info to mux.
func Register(mux *http.ServeMux, svc *Service) {
	mux.HandleFunc("GET /api/v1/info", func(w http.ResponseWriter, _ *http.Request) {
		server.WriteJSON(w, http.StatusOK, svc.Get())
	})
}
