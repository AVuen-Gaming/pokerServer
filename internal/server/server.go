package internal

import (
	"fmt"
	"log"
	"net/http"
	"server/config"

	"github.com/gorilla/mux"
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
	log.Printf("Iniciando servidor en %s", addr)
	if err := http.ListenAndServe(addr, s.Router); err != nil {
		log.Fatalf("No se pudo iniciar el servidor: %v", err)
	}
}
