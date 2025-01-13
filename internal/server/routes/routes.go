package routes

import (
	"net/http"
	"server/config"
	"server/internal/middlewares"
	"server/internal/server/controllers"

	"github.com/gorilla/mux"
	"go.temporal.io/sdk/client"
)

func DefineRoutes(r *mux.Router, c client.Client, cfg *config.Config) {
	protectedRoutes := r.PathPrefix("/").Subrouter()
	publicRoutes := r.PathPrefix("/").Subrouter()
	adminRoutes := r.PathPrefix("/").Subrouter()
	adminRoutes.Use(middlewares.JWTAuthMiddleware)
	publicRoutes.Use(middlewares.OriginValidationMiddleware(cfg.Server.Allowedorigin))
	//protectedRoutes.Use(middlewares.JWTAuthMiddleware)
	protectedRoutes.Use(middlewares.OriginValidationMiddleware(cfg.Server.Allowedorigin))
	protectedRoutes.Use(middlewares.SessionTokenMiddleware)
	protectedRoutes.Use(middlewares.WalletValidationMiddleware)
	protectedRoutes.HandleFunc("/health", HealthCheckHandler).Methods("GET")
	adminRoutes.HandleFunc("/tournaments", func(w http.ResponseWriter, r *http.Request) {
		controllers.CreateTournament(w, r, c, cfg)
	}).Methods("POST")
	//tournaments
	protectedRoutes.HandleFunc("/tournaments", controllers.GetTournaments).Methods("GET")
	protectedRoutes.HandleFunc("/tournament/register", controllers.RegisterUserToTournament).Methods("POST")
	protectedRoutes.HandleFunc("/tournament/register/available/{tournamentID}/{walletID}", controllers.CheckTournamentRegistrationAvailability).Methods("GET")
	protectedRoutes.HandleFunc("/tournament/{wallet}", controllers.GetAvailableTournamentsByWallet).Methods("GET") //cambiar por tournaments
	protectedRoutes.HandleFunc("/tournaments/ongoing/{wallet}", controllers.GetOngoingTournamentsHandler).Methods("GET")
	protectedRoutes.HandleFunc("/tournaments/registered/{wallet}", controllers.GetRegisteredOngoingTournamentsController).Methods("GET")
	protectedRoutes.HandleFunc("/tournaments/eliminated/{wallet}", controllers.GetEliminatedAndFinishedTournamentsController).Methods("GET")
	//wallets
	protectedRoutes.HandleFunc("/users", controllers.CreateUser).Methods("POST")
	protectedRoutes.HandleFunc("/wallet/{wallet}", controllers.GetWalletIDByAddress).Methods("GET")
	//tables
	protectedRoutes.HandleFunc("/table/{tournamentID}", controllers.GetTablesByTournamentID).Methods("GET")
	//tablePlayer
	protectedRoutes.HandleFunc("/tablePlayer/{tournamentID}/{walletID}", controllers.GetTablePlayersByWalletAndTournament).Methods("GET")
	//tournamentRegistration
	protectedRoutes.HandleFunc("/tournamentRegistration/{tournamentID}/{walletID}", controllers.GetTournamentRegistrationByTournamentAndWallet).Methods("GET")
	protectedRoutes.HandleFunc("/tournamentAvailableToStart/{tournamentID}/{walletID}", controllers.TournamentIsAvailableToStart).Methods("GET")
	//ranking
	protectedRoutes.HandleFunc("/rankings/{tournamentID}", controllers.GetRankingsByTournament).Methods("GET")
	protectedRoutes.HandleFunc("/ranking/{tournamentID}/{walletID}", controllers.GetRankingByTournamentAndWallet).Methods("GET")
	//prizes
	protectedRoutes.HandleFunc("/prizes/{tournamentID}", controllers.GetPrizeByTournamentID).Methods("GET")
	//middleware publico
	publicRoutes.HandleFunc("/generate-token", controllers.GenerateTokenHandler).Methods("GET")
}

func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Servidor funcionando correctamente"))
}
