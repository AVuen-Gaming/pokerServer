package controllers

import (
	"encoding/json"
	"net/http"
	"server/internal/db"
)

type UserDTO struct {
	Username      string `json:"username"`
	WalletAddress string `json:"wallet_address"`
}

func CreateUser(w http.ResponseWriter, r *http.Request) {
	var req UserDTO

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Error al decodificar la solicitud", http.StatusBadRequest)
		return
	}

	exists, err := db.UserExists(req.Username)
	if err != nil {
		http.Error(w, "Error al verificar la existencia del usuario", http.StatusInternalServerError)
		return
	}

	if exists {
		http.Error(w, "El usuario ya existe", http.StatusConflict)
		return
	}

	user, err := db.CreateUserWithWallet(req.Username, req.WalletAddress)
	if err != nil {
		http.Error(w, "Error al crear el usuario y su wallet", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}
