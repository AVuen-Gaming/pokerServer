package controllers

import (
	"encoding/json"
	"net/http"
	"server/internal/db"
)

type UserDTO struct {
	Wallet string `json:"wallet"`
}

func CreateUser(w http.ResponseWriter, r *http.Request) {
	var req UserDTO

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Error al decodificar la solicitud", http.StatusBadRequest)
		return
	}

	wallet, err := db.GetWalletByAddress(req.Wallet)
	if err != nil {
		http.Error(w, "Error retrieving wallet", http.StatusInternalServerError)
	}

	if wallet == nil {
		err = db.CreateUserWithWallet(req.Wallet)
		if err != nil {
			http.Error(w, "Error al crear el usuario y su wallet", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(req.Wallet)
}
