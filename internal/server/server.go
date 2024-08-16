package server

import (
	"log"
	"net/http"
	"server/config"

	"github.com/gorilla/mux"
)

func StartServer(cfg *config.Config) {
	r := mux.NewRouter()

	port := ":" + cfg.Server.Port
	log.Printf("Server is listening on port%s", port)
	if err := http.ListenAndServe(port, r); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
