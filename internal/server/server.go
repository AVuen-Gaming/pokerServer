package internal

import (
	"fmt"
	"log"
	"net/http"
	"server/config"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

type Server struct {
	Config *config.ServerConfig
	Router *mux.Router
}

func NewServer(cfg *config.ServerConfig) *Server {
	router := mux.NewRouter()
	server := &Server{
		Config: cfg,
		Router: router,
	}
	return server
}

func (s *Server) Start() {
	addr := fmt.Sprintf(":%s", s.Config.Port)
	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins:   []string{s.Config.Allowedorigin},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "Wallet-Address"},
		AllowCredentials: true,
	})
	handler := corsMiddleware.Handler(s.Router)
	log.Printf("Iniciando servidor en %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("No se pudo iniciar el servidor: %v", err)
	}
}
