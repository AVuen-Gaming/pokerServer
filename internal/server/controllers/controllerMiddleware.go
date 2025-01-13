package controllers

import (
	"encoding/json"
	"net/http"
	"server/internal/middlewares"
	"time"
)

func GenerateTokenHandler(w http.ResponseWriter, r *http.Request) {
	walletAddress := r.Header.Get("Wallet-Address")
	if walletAddress == "" {
		http.Error(w, "Wallet address required", http.StatusBadRequest)
		return
	}

	token, err := middlewares.GenerateSessionToken(walletAddress)
	if err != nil {
		http.Error(w, "Error generating token", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "Authorization",
		Value:    token,
		Path:     "/",
		HttpOnly: true, //true en produccion y pasar por variable de entorno
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(15 * time.Minute),
	})

	response := true

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
