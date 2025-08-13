package springbootlike

import (
	"context"
	"net/http"

	"github.com/SeaRoll/zumi/server"
)

func AddHealthRoutes() {
	// Health check endpoint
	//
	// gen:tag=Health
	server.AddHandler("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		type HealthResponseDTO struct {
			Status string `json:"status"`
		}
		var req struct {
			Ctx context.Context `ctx:"context"`
		}

		err := server.ParseRequest(r, &req)
		if err != nil {
			server.WriteError(w, http.StatusBadRequest, "failed to parse request")
			return
		}

		server.WriteJSON(w, http.StatusOK, HealthResponseDTO{
			Status: "OK",
		})
	})
}
