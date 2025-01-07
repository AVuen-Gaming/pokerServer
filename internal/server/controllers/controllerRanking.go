package controllers

import (
	"encoding/json"
	"net/http"
	"server/internal/db"
	"strconv"

	"github.com/gorilla/mux"
)

type RankingDTO struct {
	WalletID     uint `json:"wallet_id"`
	TournamentID uint `json:"tournament_id"`
	Position     int  `json:"position"`
}

func GetRankingsByTournament(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tournamentID, err := strconv.ParseUint(vars["tournamentID"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid tournament ID", http.StatusBadRequest)
		return
	}

	rankings, err := db.GetRankingsByTournamentID(uint(tournamentID))
	if err != nil {
		http.Error(w, "Error fetching rankings", http.StatusInternalServerError)
		return
	}

	var rankingsDTO []RankingDTO
	for _, r := range rankings {
		rankingsDTO = append(rankingsDTO, RankingDTO{
			WalletID:     r.WalletID,
			TournamentID: r.TournamentID,
			Position:     r.Position,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(rankingsDTO)
}

func GetRankingByTournamentAndWallet(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tournamentID, err := strconv.ParseUint(vars["tournamentID"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid tournament ID", http.StatusBadRequest)
		return
	}

	walletID, err := strconv.ParseUint(vars["walletID"], 10, 64)
	if err != nil {
		http.Error(w, "Invalid wallet ID", http.StatusBadRequest)
		return
	}

	ranking, err := db.GetRankingByTournamentAndWallet(uint(tournamentID), uint(walletID))
	if err != nil {
		http.Error(w, "Error fetching ranking", http.StatusInternalServerError)
		return
	}

	if ranking == nil {
		http.Error(w, "Ranking not found", http.StatusNotFound)
		return
	}

	rankingDTO := RankingDTO{
		WalletID:     ranking.WalletID,
		TournamentID: ranking.TournamentID,
		Position:     ranking.Position,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(rankingDTO)
}
