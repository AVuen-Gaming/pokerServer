package controllers

import (
	"encoding/json"
	"net/http"
	"server/internal/db"
)

type WalletRequest struct {
	WalletAddress string `json:"wallet_address"`
}

type WalletResponse struct {
	WalletID uint `json:"wallet_id"`
}

func GetWalletIDByAddress(w http.ResponseWriter, r *http.Request) {
	var req WalletRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil || req.WalletAddress == "" {
		http.Error(w, "Invalid request. Provide a valid wallet_address.", http.StatusBadRequest)
		return
	}

	walletID, err := db.GetWalletIDByPlayerID(req.WalletAddress)
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
