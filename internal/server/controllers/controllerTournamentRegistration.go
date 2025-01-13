package controllers

import (
	"encoding/json"
	"net/http"
	"server/internal/db"
	"strconv"

	"github.com/gorilla/mux"
)

type TournamentRegistrationIDsDTO struct {
	ID           uint `json:"id"`
	TournamentID uint `json:"tournament_id"`
	WalletID     uint `json:"wallet_id"`
	Eliminated   bool `json:"eliminated"`
}

type TournamentAvailableDTO struct {
	IsAvailable bool `json:"is_available"`
}

func TournamentIsAvailableToStart(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tournamentIDStr := vars["tournamentID"]
	walletIDStr := vars["walletID"]
	var isAvailable bool

	tournamentID, err := strconv.ParseUint(tournamentIDStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid tournament ID", http.StatusBadRequest)
		return
	}

	tournament, err := db.GetTournamentByID(uint(tournamentID))
	if err != nil {
		http.Error(w, "Error fetching tournament details", http.StatusInternalServerError)
		return
	}

	if tournament == nil {
		http.Error(w, "Tournament not found", http.StatusNotFound)
		return
	}

	registrations, err := db.GetTournamentRegistrationsByTournamentID(uint(tournamentID))
	if err != nil {
		http.Error(w, "Error fetching tournament registrations", http.StatusInternalServerError)
		return
	}

	walletID, err := strconv.ParseUint(walletIDStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid wallet ID", http.StatusBadRequest)
		return
	}

	registration, err := db.GetTournamentRegistrationByTournamentAndWallet(uint(tournamentID), uint(walletID))
	if err != nil {
		http.Error(w, "Error fetching tournament registration", http.StatusInternalServerError)
		return
	}

	if registration == nil {
		http.Error(w, "Tournament registration not found", http.StatusNotFound)
		return
	}

	if registration.Eliminated || len(registrations) < int(tournament.MinPlayers) {
		isAvailable = false
		return
	}

	if tournament.EndDate != nil {
		isAvailable = false
		return
	}
	isAvailable = true
	response := TournamentAvailableDTO{
		IsAvailable: isAvailable,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func GetTournamentRegistrationByTournamentAndWallet(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tournamentIDStr := vars["tournamentID"]
	walletIDStr := vars["walletID"]

	tournamentID, err := strconv.ParseUint(tournamentIDStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid tournament ID", http.StatusBadRequest)
		return
	}

	walletID, err := strconv.ParseUint(walletIDStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid wallet ID", http.StatusBadRequest)
		return
	}

	registration, err := db.GetTournamentRegistrationByTournamentAndWallet(uint(tournamentID), uint(walletID))
	if err != nil {
		http.Error(w, "Error fetching tournament registration", http.StatusInternalServerError)
		return
	}

	if registration == nil {
		http.Error(w, "Tournament registration not found", http.StatusNotFound)
		return
	}

	response := TournamentRegistrationIDsDTO{ // todo change for constructor
		ID:           registration.ID,
		TournamentID: registration.TournamentID,
		WalletID:     registration.WalletID,
		Eliminated:   registration.Eliminated,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
