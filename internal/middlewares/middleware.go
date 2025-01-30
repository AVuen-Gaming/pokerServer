package middlewares

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/gorilla/mux"
	"golang.org/x/time/rate"
)

var sessionTokens = struct {
	mu     sync.Mutex
	tokens map[string]SessionData
}{
	tokens: make(map[string]SessionData),
}

type SessionData struct {
	Token     string
	ExpiresAt time.Time
}

type RateLimiter struct {
	limiterMap map[string]*rate.Limiter
	mu         sync.Mutex
	rate       rate.Limit
	burst      int
}

func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	return &RateLimiter{
		limiterMap: make(map[string]*rate.Limiter),
		rate:       r,
		burst:      b,
	}
}

func (rl *RateLimiter) getLimiter(walletAddress string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if _, exists := rl.limiterMap[walletAddress]; !exists {
		rl.limiterMap[walletAddress] = rate.NewLimiter(rl.rate, rl.burst)
	}

	return rl.limiterMap[walletAddress]
}

func RateLimitMiddleware(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			walletAddress := r.Header.Get("Wallet-Address")
			if walletAddress == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			limiter := rl.getLimiter(walletAddress)
			if !limiter.Allow() {
				http.Error(w, "Too many requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func GenerateSessionToken(walletAddress, sign string) (string, error) {
	claims := jwt.MapClaims{
		"walletAddress": walletAddress,
		"exp":           time.Now().Add(15 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(sign))
	if err != nil {
		return "", err
	}

	StoreSessionToken(walletAddress, signedToken, time.Now().Add(15*time.Minute))
	return signedToken, nil
}

func StoreSessionToken(walletAddress, token string, expiresAt time.Time) {
	sessionTokens.mu.Lock()
	defer sessionTokens.mu.Unlock()

	sessionTokens.tokens[walletAddress] = SessionData{
		Token:     token,
		ExpiresAt: expiresAt,
	}
}

func ValidateSessionToken(token, walletAddress string) bool {
	sessionTokens.mu.Lock()
	defer sessionTokens.mu.Unlock()

	session, exists := sessionTokens.tokens[walletAddress]
	if exists && session.Token == token && session.ExpiresAt.After(time.Now()) {
		return true
	}
	return false
}

func GetTokenByWallet(walletAddress string) (string, error) {
	sessionTokens.mu.Lock()
	defer sessionTokens.mu.Unlock()

	session, exists := sessionTokens.tokens[walletAddress]
	if !exists || session.ExpiresAt.Before(time.Now()) {
		return "", errors.New("token no encontrado o expirado")
	}
	return session.Token, nil
}

func SessionTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("SessionToken")
		if err != nil || cookie.Value == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		wallet := r.Header.Get("Wallet-Address")
		if wallet == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if !ValidateWallet(wallet) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		token := cookie.Value
		if !ValidateSessionToken(token, wallet) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func WalletValidationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wallet := r.Header.Get("Wallet-Address")
		if wallet == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if !ValidateWallet(wallet) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		token, err := GetTokenByWallet(wallet)
		if err != nil || token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ValidateWallet(wallet string) bool {
	if len(wallet) != 42 || !strings.HasPrefix(wallet, "0x") {
		return false
	}
	for _, char := range wallet[2:] {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

func OriginValidationMiddleware(allowedOrigin string) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			referer := r.Header.Get("Origin")
			if referer == "" {
				referer = r.Header.Get("Referer")
			}
			if referer == "" || referer != allowedOrigin {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func JWTAuthMiddleware(staticToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
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
}
