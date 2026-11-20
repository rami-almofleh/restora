package server

import "net/http"

type healthBody struct {
	Status string `json:"status"`
}

// RegisterHealth adds the liveness and readiness endpoints used by Kubernetes probes.
func RegisterHealth(mux *http.ServeMux) {
	ok := func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, healthBody{Status: "ok"})
	}
	mux.HandleFunc("GET /healthz", ok)
	mux.HandleFunc("GET /readyz", ok)
}
