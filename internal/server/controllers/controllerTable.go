package controllers

import (
	"encoding/json"
	"net/http"
	"server/internal/db"
	"strconv"

	"github.com/gorilla/mux"
)

type TableResponse struct {
	ID           uint `json:"id"`
	TournamentID uint `json:"tournament_id"`
}

func GetTablesByTournamentID(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tournamentIDStr, ok := vars["tournamentID"]
	if !ok {
		http.Error(w, "Tournament ID is required", http.StatusBadRequest)
		return
	}

	tournamentID, err := strconv.ParseUint(tournamentIDStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid Tournament ID format", http.StatusBadRequest)
		return
	}

	tables, err := db.GetTablesByTournamentID(uint(tournamentID))
	if err != nil {
		http.Error(w, "Error fetching tables for the tournament", http.StatusInternalServerError)
		return
	}

	var response []TableResponse
	for _, table := range tables {
		response = append(response, TableResponse{
			ID:           table.ID,
			TournamentID: table.TournamentID,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
