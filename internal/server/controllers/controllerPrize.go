package controllers

import (
	"encoding/json"
	"net/http"
	"server/internal/db"
	"strconv"

	"github.com/gorilla/mux"
)

type PrizeDTO struct {
	ID           uint    `json:"id"`
	TournamentID uint    `json:"tournament_id"`
	TotalPot     float32 `json:"total_pot"`
	PrizeList    string  `json:"prize_list"` // En JSON como string
}

func GetPrizeByTournamentID(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tournamentID, err := strconv.ParseUint(vars["tournamentID"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid tournament ID", http.StatusBadRequest)
		return
	}

	prize, err := db.GetPrizeByTournamentID(uint(tournamentID))
	if err != nil {
		http.Error(w, "Error fetching prize", http.StatusInternalServerError)
		return
	}

	if prize == nil {
		http.Error(w, "Prize not found", http.StatusNotFound)
		return
	}
	prizeDTO := PrizeDTO{
		ID:           prize.ID,
		TournamentID: prize.TournamentID,
		TotalPot:     prize.TotalPot,
		PrizeList:    string(prize.PrizeList),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(prizeDTO)
}
