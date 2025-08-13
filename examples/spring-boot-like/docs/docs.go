package docs

import (
	_ "embed"
	"fmt"
	"net/http"

	"github.com/SeaRoll/zumi/server"
)

var (
	//go:embed openapi.yaml
	embedOpenAPI string
	//go:embed index.html
	embedIndexHTML string
)

func AddDocRoutes() {
	// gen:ignore
	server.AddHandler("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)

		_, err := w.Write([]byte(embedIndexHTML))
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to write index.html: %v", err), http.StatusInternalServerError)
			return
		}
	})

	// gen:ignore
	server.AddHandler("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		w.WriteHeader(http.StatusOK)

		_, err := w.Write([]byte(embedOpenAPI))
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to write openapi.yaml: %v", err), http.StatusInternalServerError)
			return
		}
	})
}
