package middlewares

import (
	"net/http"
	"strings"
)

var staticToken = "popio"

// Middleware para validar un JWT hardcodeado
func JWTAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "No autorizado", http.StatusUnauthorized)
			return
		}

		token := strings.Split(authHeader, " ")[1]
		if token != staticToken {
			http.Error(w, "Token inválido", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
