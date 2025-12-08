package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"server/config"
	"time"

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
	startTournamentCreation()
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("No se pudo iniciar el servidor: %v", err)
	}
}

func createTournaments() {
	now := time.Now().UTC()
	registrationStartDate := now.Format(time.RFC3339)
	registrationEndDate := now.Add(30 * time.Minute).Format(time.RFC3339)
	startDate := now.Add(31 * time.Minute).Format(time.RFC3339)

	tournaments := []map[string]interface{}{
		{
			"name":                    fmt.Sprintf("Auto Tournament 5 USDT %d", now.Unix()),
			"registration_start_date": registrationStartDate,
			"registration_end_date":   registrationEndDate,
			"start_date":              startDate,
			"entry_cost":              5,
			"increment_blind":         1,
			"currency":                "usdt",
			"ongoing":                 false,
			"start":                   false,
			"configuration":           "Texas Hold'em",
			"min_players":             2,
			"max_players":             306,
			"turn_seconds":            1,
			"start_chips":             10,
			"bb_value":                20,
		},
		{
			"name":                    fmt.Sprintf("Auto Tournament 20 USDT %d", now.Unix()),
			"registration_start_date": registrationStartDate,
			"registration_end_date":   registrationEndDate,
			"start_date":              startDate,
			"entry_cost":              20,
			"increment_blind":         1,
			"currency":                "usdt",
			"ongoing":                 false,
			"start":                   false,
			"configuration":           "Texas Hold'em",
			"min_players":             2,
			"max_players":             306,
			"turn_seconds":            1,
			"start_chips":             10,
			"bb_value":                20,
		},
		{
			"name":                    fmt.Sprintf("Auto Tournament 50 USDT %d", now.Unix()),
			"registration_start_date": registrationStartDate,
			"registration_end_date":   registrationEndDate,
			"start_date":              startDate,
			"entry_cost":              50,
			"increment_blind":         1,
			"currency":                "usdt",
			"ongoing":                 false,
			"start":                   false,
			"configuration":           "Texas Hold'em",
			"min_players":             2,
			"max_players":             306,
			"turn_seconds":            1,
			"start_chips":             10,
			"bb_value":                20,
		},
		{
			"name":                    fmt.Sprintf("Auto Tournament 100 USDT %d", now.Unix()),
			"registration_start_date": registrationStartDate,
			"registration_end_date":   registrationEndDate,
			"start_date":              startDate,
			"entry_cost":              100,
			"increment_blind":         1,
			"currency":                "usdt",
			"ongoing":                 false,
			"start":                   false,
			"configuration":           "Texas Hold'em",
			"min_players":             2,
			"max_players":             306,
			"turn_seconds":            1,
			"start_chips":             10,
			"bb_value":                20,
		},
		{
			"name":                    fmt.Sprintf("Test Tournament sepolia %d", now.Unix()),
			"registration_start_date": registrationStartDate,
			"registration_end_date":   registrationEndDate,
			"start_date":              startDate,
			"entry_cost":              0.00001,
			"increment_blind":         1,
			"currency":                "sepolia",
			"ongoing":                 false,
			"start":                   false,
			"configuration":           "Texas Hold'em",
			"min_players":             2,
			"max_players":             306,
			"turn_seconds":            1,
			"start_chips":             10,
			"bb_value":                20,
		},
	}

	client := &http.Client{Timeout: 10 * time.Second}

	for _, bodyData := range tournaments {
		bodyBytes, err := json.Marshal(bodyData)
		if err != nil {
			log.Printf("Error marshalling body data: %v", err)
			continue
		}

		req, err := http.NewRequest("POST", "http://localhost:7000/tournaments", bytes.NewReader(bodyBytes)) //mover llamado a una futura funcion para evitar el uso del endpoint interno porque puede haber errores con la go func
		if err != nil {
			log.Printf("Error creating request: %v", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "localhost:3000")      //usar env
		req.Header.Set("Authorization", "Bearer popio") //usar env

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error making POST request: %v", err)
			continue
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			log.Printf("Unexpected response status: %d", resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func startTournamentCreation() {
	go func() {
		tucker := time.NewTicker(7 * time.Second)
		defer tucker.Stop()
		createTournaments()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			createTournaments()
		}
	}()
}
