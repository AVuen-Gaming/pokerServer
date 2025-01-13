package controllers

import (
	"encoding/json"
	"net/http"
	"server/internal/db"

	"github.com/gorilla/mux"
)

type WalletRequest struct {
	WalletAddress string `json:"wallet_address"`
}

type WalletResponse struct {
	WalletID uint `json:"wallet_id"`
}

func GetWalletIDByAddress(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	wallet := vars["wallet"]

	walletID, err := db.GetWalletIDByPlayerID(wallet)
	if err != nil {
		http.Error(w, "Wallet not found or error fetching wallet ID.", http.StatusNotFound)
		return
	}

	resp := WalletResponse{
		WalletID: walletID,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
