package controllers

import (
	"encoding/json"
	"net/http"
	"server/internal/db"
	"strconv"

	"github.com/gorilla/mux"
)

type TablePlayerResponse struct {
	ID           uint   `json:"id"`
	TableID      uint   `json:"table_id"`
	WalletID     uint   `json:"wallet_id"`
	TournamentID uint   `json:"tournament_id"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func GetTablePlayersByWalletAndTournament(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	walletIDStr, ok := vars["walletID"]
	if !ok {
		http.Error(w, "Wallet ID is required", http.StatusBadRequest)
		return
	}

	tournamentIDStr, ok := vars["tournamentID"]
	if !ok {
		http.Error(w, "Tournament ID is required", http.StatusBadRequest)
		return
	}

	walletID, err := strconv.ParseUint(walletIDStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid Wallet ID format", http.StatusBadRequest)
		return
	}

	tournamentID, err := strconv.ParseUint(tournamentIDStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid Tournament ID format", http.StatusBadRequest)
		return
	}

	tablePlayers, err := db.GetTablePlayersByWalletAndTournament(uint(walletID), uint(tournamentID))
	if err != nil {
		http.Error(w, "Error fetching table players", http.StatusInternalServerError)
		return
	}

	var response TablePlayerResponse
	response.ID = tablePlayers.ID
	response.TableID = tablePlayers.TableID
	response.WalletID = tablePlayers.WalletID
	response.WalletID = tablePlayers.TournamentID

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
