package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"server/config"
	"server/internal/db"
	"server/internal/db/models"
	"server/internal/poker"
	temporal "server/internal/workflow"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.temporal.io/sdk/client"
)

type TournamentDTO struct {
	ID                    uint
	Name                  string    `json:"name"`
	RegistrationStartDate time.Time `json:"registration_start_date"`
	RegistrationEndDate   time.Time `json:"registration_end_date"`
	StartDate             time.Time `json:"start_date"`
	EndDate               time.Time `json:"end_date"`
	EntryCost             float32   `json:"entry_cost"`
	Currency              string    `json:"currency"`
	Prize                 string    `json:"prize"`
	Configuration         string    `json:"configuration"`
	MinPlayers            int       `json:"min_players"`
	MaxPlayers            int       `json:"max_players"`
	TurnSeconds           int       `json:"turn_seconds"`
	Ongoing               bool      `json:"ongoing"`
	StartChips            int       `json:"start_chips"`
	BBValue               int       `json:"bb_value"`
	Start                 bool      `json:"start"`
}

type TournamentRegistrationDTO struct {
	TournamentID  uint   `json:"tournament_id"`
	WalletAddress string `json:"wallet"`
}

func CreateTournament(w http.ResponseWriter, r *http.Request, c client.Client, cfg *config.Config) {
	var req TournamentDTO
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Error al decodificar la solicitud", http.StatusBadRequest)
		return
	}

	tournament := convertToTournament(req)

	err = db.InsertTournament(tournament)
	if err != nil {
		http.Error(w, "Error al crear el torneo en la base de datos", http.StatusInternalServerError)
		return
	}

	tournamentID, err := db.GetTournamentByName(tournament.Name)
	if err != nil {
		http.Error(w, "Error al tomar el id", http.StatusInternalServerError)
		return
	}

	tournament.ID = tournamentID.ID

	tournamentController := convertToTournamentController(tournament)

	workflowID := fmt.Sprintf("tournament-workflow-%s", uuid.New().String())
	taskQueue := "poker-task-queue"

	we, err := c.ExecuteWorkflow(r.Context(), client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: taskQueue,
	}, temporal.TournamentControllerWorkflow, tournamentController, cfg)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error al iniciar el flujo de trabajo: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tournament": tournament,
		"workflowID": we.GetID(),
		"runID":      we.GetRunID(),
	})
}

func GetTournaments(w http.ResponseWriter, r *http.Request) {
	tournaments, err := db.GetAllTournaments()
	if err != nil {
		http.Error(w, "Error al obtener los torneos", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tournaments)
}

func RegisterUserToTournament(w http.ResponseWriter, r *http.Request) {
	var req TournamentRegistrationDTO
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Failed to decode request", http.StatusBadRequest)
		return
	}

	tournament, err := db.GetTournamentByID(req.TournamentID)
	if err != nil {
		http.Error(w, "Error retrieving tournament", http.StatusInternalServerError)
		return
	}

	if time.Now().After(tournament.RegistrationEndDate) {
		http.Error(w, "Registration period has ended", http.StatusForbidden)
		return
	}

	wallet, err := db.GetWalletByAddress(req.WalletAddress)
	if err != nil {
		http.Error(w, "Error retrieving wallet", http.StatusInternalServerError)
		return
	}

	if wallet == nil {
		http.Error(w, "Wallet not found", http.StatusNotFound)
		return
	}

	isRegistered, err := db.CheckUserRegistration(req.TournamentID, wallet.ID)
	if err != nil {
		http.Error(w, "Error checking user registration", http.StatusInternalServerError)
		return
	}

	if isRegistered {
		http.Error(w, "User already registered for the tournament", http.StatusConflict)
		return
	}

	err = db.RegisterUserToTournament(req.TournamentID, wallet.ID)
	if err != nil {
		http.Error(w, "Error registering user to tournament", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("User successfully registered for the tournament"))
}

func GetAvailableTournamentsByWallet(w http.ResponseWriter, r *http.Request) {
	walletAddress := mux.Vars(r)["wallet"]

	wallet, err := db.GetWalletByAddress(walletAddress)
	if err != nil {
		http.Error(w, "Error retrieving wallet", http.StatusInternalServerError)
		return
	}

	if wallet == nil {
		http.Error(w, "Wallet does not exist", http.StatusNotFound)
		return
	}

	tournaments, err := db.GetUnregisteredTournaments(wallet.ID)
	if err != nil {
		http.Error(w, "Error retrieving tournaments", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tournaments)
}

func GetOngoingTournamentsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	walletAddress := vars["wallet"]

	wallet, err := db.GetWalletByAddress(walletAddress)
	if err != nil {
		http.Error(w, "Error fetching wallet", http.StatusInternalServerError)
		return
	}

	if wallet == nil {
		http.Error(w, "Wallet not found", http.StatusNotFound)
		return
	}

	tournaments, err := db.GetOngoingTournaments(wallet.ID)
	if err != nil {
		http.Error(w, "Error fetching tournaments", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tournaments)
}

func GetRegisteredOngoingTournamentsController(w http.ResponseWriter, r *http.Request) {
	wallet := mux.Vars(r)["wallet"]

	walletData, err := db.GetWalletByAddress(wallet)
	if err != nil || walletData == nil {
		http.Error(w, "Wallet does not exist", http.StatusNotFound)
		return
	}

	tournaments, err := db.GetRegisteredOngoingTournaments(walletData.ID)
	if err != nil {
		http.Error(w, "Error fetching registered tournaments", http.StatusInternalServerError)
		return
	}

	response := []poker.Tournament{}
	for _, t := range tournaments {
		response = append(response, convertToTournamentController(t))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func GetEliminatedAndFinishedTournamentsController(w http.ResponseWriter, r *http.Request) {
	wallet := mux.Vars(r)["wallet"]

	walletData, err := db.GetWalletByAddress(wallet)
	if err != nil {
		http.Error(w, "Error fetching wallet", http.StatusInternalServerError)
		return
	}

	if walletData == nil {
		http.Error(w, "Wallet not found", http.StatusNotFound)
		return
	}

	tournaments, err := db.GetEliminatedAndFinishedTournaments(walletData.ID)
	if err != nil {
		http.Error(w, "Error fetching tournaments", http.StatusInternalServerError)
		return
	}

	response := []poker.Tournament{}
	for _, t := range tournaments {
		response = append(response, convertToTournamentController(t))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func convertToTournament(dto TournamentDTO) models.Tournament {
	var endDate *time.Time
	if !dto.EndDate.IsZero() {
		endDate = &dto.EndDate
	}

	return models.Tournament{
		ID:                    dto.ID,
		Name:                  dto.Name,
		RegistrationStartDate: dto.RegistrationStartDate,
		RegistrationEndDate:   dto.RegistrationEndDate,
		StartDate:             dto.StartDate,
		EndDate:               endDate,
		EntryCost:             dto.EntryCost,
		Currency:              dto.Currency,
		Prize:                 dto.Prize,
		Configuration:         dto.Configuration,
		MinPlayers:            dto.MinPlayers,
		MaxPlayers:            dto.MaxPlayers,
		TurnSeconds:           dto.TurnSeconds,
		Ongoing:               dto.Ongoing,
		StartChips:            dto.StartChips,
		BBValue:               dto.BBValue,
		Start:                 dto.Start,
	}
}

func convertToTournamentController(dto models.Tournament) poker.Tournament {
	var endDate time.Time
	if dto.EndDate != nil {
		endDate = *dto.EndDate
	}

	return poker.Tournament{
		ID:                    dto.ID,
		Name:                  dto.Name,
		RegistrationStartDate: dto.RegistrationStartDate,
		RegistrationEndDate:   dto.RegistrationEndDate,
		StartDate:             dto.StartDate,
		EndDate:               endDate,
		EntryCost:             dto.EntryCost,
		Currency:              dto.Currency,
		Prize:                 dto.Prize,
		Configuration:         dto.Configuration,
		MinPlayers:            dto.MinPlayers,
		MaxPlayers:            dto.MaxPlayers,
		TurnSeconds:           dto.TurnSeconds,
		Ongoing:               dto.Ongoing,
		StartChips:            dto.StartChips,
		BBValue:               dto.BBValue,
		Start:                 dto.Start,
	}
}
