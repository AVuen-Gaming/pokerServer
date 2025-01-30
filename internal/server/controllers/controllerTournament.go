package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"server/config"
	"server/internal/db"
	"server/internal/db/models"
	"server/internal/poker"
	temporal "server/internal/workflow"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.temporal.io/sdk/client"
)

type TournamentDTO struct {
	ID                    uint
	Name                  string    `json:"name"`
	RegistrationStartDate time.Time `json:"registration_start_date"`
	RegistrationEndDate   time.Time `json:"registration_end_date"`
	StartDate             time.Time `json:"start_date"`
	EndDate               time.Time `json:"end_date"`
	EntryCost             float32   `json:"entry_cost"`
	Currency              string    `json:"currency"`
	Prize                 string    `json:"prize"`
	Configuration         string    `json:"configuration"`
	MinPlayers            int       `json:"min_players"`
	MaxPlayers            int       `json:"max_players"`
	TurnSeconds           int       `json:"turn_seconds"`
	Ongoing               bool      `json:"ongoing"`
	StartChips            int       `json:"start_chips"`
	BBValue               int       `json:"bb_value"`
	Start                 bool      `json:"start"`
	IncrementBlind        int       `json:"increment_blind"`
}

type TournamentRegistrationDTO struct {
	TournamentID    uint   `json:"tournament_id"`
	WalletAddress   string `json:"wallet"`
	TransactionHash string `json:"transaction_hash"`
}

type RegistrationAvailabilityDTO struct {
	IsAvailable bool `json:"is_available"`
}

func CheckTournamentRegistrationAvailability(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	walletIDStr := vars["walletID"]
	tournamentIDStr := vars["tournamentID"]

	walletID, err := strconv.ParseUint(walletIDStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid wallet ID", http.StatusBadRequest)
		return
	}

	tournamentID, err := strconv.ParseUint(tournamentIDStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid tournament ID", http.StatusBadRequest)
		return
	}

	tournament, err := db.GetTournamentByID(uint(tournamentID))
	if err != nil {
		http.Error(w, "Error retrieving tournament", http.StatusInternalServerError)
		return
	}

	if tournament == nil {
		http.Error(w, "Tournament not found", http.StatusNotFound)
		return
	}

	registration, err := db.GetTournamentRegistrationByTournamentAndWallet(uint(tournamentID), uint(walletID))
	if err != nil {
		http.Error(w, "Error checking tournament registration", http.StatusInternalServerError)
		return
	}

	if registration != nil {
		response := RegistrationAvailabilityDTO{IsAvailable: false}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	if time.Now().After(tournament.RegistrationEndDate) {
		response := RegistrationAvailabilityDTO{IsAvailable: false}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	timeLeft := time.Until(tournament.RegistrationEndDate)
	if timeLeft.Seconds() <= 10 {
		response := RegistrationAvailabilityDTO{IsAvailable: false}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	response := RegistrationAvailabilityDTO{IsAvailable: true}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func CreateTournament(w http.ResponseWriter, r *http.Request, c client.Client, cfg *config.Config) {
	var req TournamentDTO
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Error al decodificar la solicitud", http.StatusBadRequest)
		return
	}

	tournament := convertToTournament(req)

	err = db.InsertTournament(tournament)
	if err != nil {
		http.Error(w, "Error al crear el torneo en la base de datos", http.StatusInternalServerError)
		return
	}

	tournamentID, err := db.GetTournamentByName(tournament.Name)
	if err != nil {
		http.Error(w, "Error al tomar el id", http.StatusInternalServerError)
		return
	}

	tournament.ID = tournamentID.ID

	tournamentController := convertToTournamentController(tournament)

	workflowID := fmt.Sprintf("tournament-workflow-%s", uuid.New().String())
	taskQueue := "poker-task-queue"

	we, err := c.ExecuteWorkflow(r.Context(), client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: taskQueue,
	}, temporal.TournamentControllerWorkflow, tournamentController, cfg)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error al iniciar el flujo de trabajo: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tournament": tournament,
		"workflowID": we.GetID(),
		"runID":      we.GetRunID(),
	})
}

func GetTournaments(w http.ResponseWriter, r *http.Request) {
	tournaments, err := db.GetAllTournaments()
	if err != nil {
		http.Error(w, "Error al obtener los torneos", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tournaments)
}

func RegisterUserToTournament(w http.ResponseWriter, r *http.Request, cfg *config.ServerConfig) {
	var req TournamentRegistrationDTO
	valid := false
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Failed to decode request", http.StatusBadRequest)
		return
	}

	if req.TransactionHash == "" {
		http.Error(w, "Invalid Hash", http.StatusInternalServerError)
		return
	}

	tournament, err := db.GetTournamentByID(req.TournamentID)
	if err != nil {
		http.Error(w, "Error retrieving tournament", http.StatusInternalServerError)
		return
	}

	if time.Now().After(tournament.RegistrationEndDate) {
		http.Error(w, "Registration period has ended", http.StatusForbidden)
		return
	}

	wallet, err := db.GetWalletByAddress(req.WalletAddress)
	if err != nil {
		http.Error(w, "Error retrieving wallet", http.StatusInternalServerError)
		return
	}

	if wallet == nil {
		http.Error(w, "Wallet not found", http.StatusNotFound)
		return
	}

	isRegistered, err := db.CheckUserRegistration(req.TournamentID, wallet.ID)
	if err != nil {
		http.Error(w, "Error checking user registration", http.StatusInternalServerError)
		return
	}

	if isRegistered {
		http.Error(w, "User already registered for the tournament", http.StatusConflict)
		return
	}

	if tournament.Currency == "usdt" {
		valid = isValidTransactionBsc(req.TransactionHash, req.WalletAddress, float64(tournament.EntryCost), cfg)
	} else if tournament.Currency == "sepolia" {
		valid = isValidTransactionSepolia(req.TransactionHash, req.WalletAddress, float64(tournament.EntryCost), cfg)
	}

	if !valid {
		http.Error(w, "invalid transaction", http.StatusConflict)
		return
	}

	err = db.RegisterUserToTournament(req.TournamentID, wallet.ID)
	if err != nil {
		http.Error(w, "Error registering user to tournament", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("User successfully registered for the tournament"))
}

func GetAvailableTournamentsByWallet(w http.ResponseWriter, r *http.Request) {
	walletAddress := mux.Vars(r)["wallet"]

	wallet, err := db.GetWalletByAddress(walletAddress)
	if err != nil {
		http.Error(w, "Error retrieving wallet", http.StatusInternalServerError)
		return
	}

	if wallet == nil {
		http.Error(w, "Wallet does not exist", http.StatusNotFound)
		return
	}

	tournaments, err := db.GetUnregisteredTournaments(wallet.ID)
	if err != nil {
		http.Error(w, "Error retrieving tournaments", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tournaments)
}

func GetOngoingTournamentsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	walletAddress := vars["wallet"]

	wallet, err := db.GetWalletByAddress(walletAddress)
	if err != nil {
		http.Error(w, "Error fetching wallet", http.StatusInternalServerError)
		return
	}

	if wallet == nil {
		http.Error(w, "Wallet not found", http.StatusNotFound)
		return
	}

	tournaments, err := db.GetOngoingTournaments(wallet.ID)
	if err != nil {
		http.Error(w, "Error fetching tournaments", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tournaments)
}

func GetRegisteredOngoingTournamentsController(w http.ResponseWriter, r *http.Request) {
	wallet := mux.Vars(r)["wallet"]

	walletData, err := db.GetWalletByAddress(wallet)
	if err != nil || walletData == nil {
		http.Error(w, "Wallet does not exist", http.StatusNotFound)
		return
	}

	tournaments, err := db.GetRegisteredOngoingTournaments(walletData.ID)
	if err != nil {
		http.Error(w, "Error fetching registered tournaments", http.StatusInternalServerError)
		return
	}

	response := []poker.Tournament{}
	for _, t := range tournaments {
		response = append(response, convertToTournamentController(t))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func GetEliminatedAndFinishedTournamentsController(w http.ResponseWriter, r *http.Request) {
	wallet := mux.Vars(r)["wallet"]

	walletData, err := db.GetWalletByAddress(wallet)
	if err != nil {
		http.Error(w, "Error fetching wallet", http.StatusInternalServerError)
		return
	}

	if walletData == nil {
		http.Error(w, "Wallet not found", http.StatusNotFound)
		return
	}

	tournaments, err := db.GetEliminatedAndFinishedTournaments(walletData.ID)
	if err != nil {
		http.Error(w, "Error fetching tournaments", http.StatusInternalServerError)
		return
	}

	response := []poker.Tournament{}
	for _, t := range tournaments {
		response = append(response, convertToTournamentController(t))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func convertToTournament(dto TournamentDTO) models.Tournament {
	var endDate *time.Time
	if !dto.EndDate.IsZero() {
		endDate = &dto.EndDate
	}

	return models.Tournament{
		ID:                    dto.ID,
		Name:                  dto.Name,
		RegistrationStartDate: dto.RegistrationStartDate,
		RegistrationEndDate:   dto.RegistrationEndDate,
		StartDate:             dto.StartDate,
		EndDate:               endDate,
		EntryCost:             dto.EntryCost,
		Currency:              dto.Currency,
		Prize:                 dto.Prize,
		Configuration:         dto.Configuration,
		MinPlayers:            dto.MinPlayers,
		MaxPlayers:            dto.MaxPlayers,
		TurnSeconds:           dto.TurnSeconds,
		Ongoing:               dto.Ongoing,
		StartChips:            dto.StartChips,
		BBValue:               dto.BBValue,
		Start:                 dto.Start,
		IncrementBlind:        dto.IncrementBlind,
	}
}

func convertToTournamentController(dto models.Tournament) poker.Tournament {
	var endDate time.Time
	if dto.EndDate != nil {
		endDate = *dto.EndDate
	}

	return poker.Tournament{
		ID:                    dto.ID,
		Name:                  dto.Name,
		RegistrationStartDate: dto.RegistrationStartDate,
		RegistrationEndDate:   dto.RegistrationEndDate,
		StartDate:             dto.StartDate,
		EndDate:               endDate,
		EntryCost:             dto.EntryCost,
		Currency:              dto.Currency,
		Prize:                 dto.Prize,
		Configuration:         dto.Configuration,
		MinPlayers:            dto.MinPlayers,
		MaxPlayers:            dto.MaxPlayers,
		TurnSeconds:           dto.TurnSeconds,
		Ongoing:               dto.Ongoing,
		StartChips:            dto.StartChips,
		BBValue:               dto.BBValue,
		Start:                 dto.Start,
		IncrementBlind:        dto.IncrementBlind,
	}
}

func isValidTransactionBsc(txHash string, senderWallet string, requiredAmount float64, config *config.ServerConfig) bool {
	url := fmt.Sprintf("https://api.bscscan.com/api?module=account&action=tokentx&txhash=%s&apikey=%s", txHash, config.BcsApiKey)

	resp, err := http.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var response struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Result  []struct {
			From        string `json:"from"`
			To          string `json:"to"`
			Value       string `json:"value"`
			Contract    string `json:"contractAddress"`
			BlockNumber string `json:"blockNumber"`
		} `json:"result"`
	}

	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return false
	}

	if len(response.Result) == 0 {
		return false
	}

	tx := response.Result[0]

	senderWallet = strings.ToLower(senderWallet)
	receiverWallet := strings.ToLower(config.Wallet)
	txSender := strings.ToLower(tx.From)
	txReceiver := strings.ToLower(tx.To)

	if txSender != senderWallet {
		return false
	}

	if txReceiver != receiverWallet {
		return false
	}

	valueInUSDT, err := strconv.ParseFloat(tx.Value, 64)
	if err != nil {
		return false
	}
	valueInUSDT /= 1e6 // UST tiene 6 decimales en BSC

	txDetailsURL := fmt.Sprintf("https://api.bscscan.com/api?module=proxy&action=eth_getTransactionByHash&txhash=%s&apikey=%s", txHash, config.BcsApiKey)
	txDetailsResp, err := http.Get(txDetailsURL)
	if err != nil {
		fmt.Println("Error llamando a BscScan API para obtener detalles de la transacción:", err)
		return false
	}
	defer txDetailsResp.Body.Close()

	if txDetailsResp.StatusCode != http.StatusOK {
		fmt.Println("BscScan API devolvió un código de estado no válido para detalles de la transacción:", txDetailsResp.Status)
		return false
	}

	var txDetails struct {
		Result struct {
			Gas      string `json:"gas"`
			GasPrice string `json:"gasPrice"`
		} `json:"result"`
	}

	err = json.NewDecoder(txDetailsResp.Body).Decode(&txDetails)
	if err != nil {
		return false
	}

	gasUsed, err := strconv.ParseInt(txDetails.Result.Gas, 0, 64)
	if err != nil {
		return false
	}

	gasPrice, err := strconv.ParseInt(txDetails.Result.GasPrice, 0, 64)
	if err != nil {
		return false
	}

	gasCostInBNB := float64(gasUsed*gasPrice) / 1e18

	totalSpent := valueInUSDT + gasCostInBNB
	if totalSpent < requiredAmount {
		return false
	}

	blockURL := fmt.Sprintf("https://api.bscscan.com/api?module=proxy&action=eth_getBlockByNumber&tag=%s&boolean=true&apikey=%s", tx.BlockNumber, config.BcsApiKey)
	blockResp, err := http.Get(blockURL)
	if err != nil {
		return false
	}
	defer blockResp.Body.Close()

	if blockResp.StatusCode != http.StatusOK {
		return false
	}

	var blockResponse struct {
		Result struct {
			Timestamp string `json:"timestamp"`
		} `json:"result"`
	}

	err = json.NewDecoder(blockResp.Body).Decode(&blockResponse)
	if err != nil || blockResponse.Result.Timestamp == "" {
		fmt.Println("Error al obtener el timestamp del bloque.")
		return false
	}

	blockTimestamp, err := strconv.ParseInt(blockResponse.Result.Timestamp, 0, 64)
	if err != nil {
		fmt.Println("Error al convertir el timestamp de la transacción.")
		return false
	}

	now := time.Now().Unix()
	allowedTime := now - 300 // cambiar margen

	if blockTimestamp < allowedTime {
		return false
	}

	return true
}

func isValidTransactionSepolia(txHash string, senderWallet string, requiredAmount float64, config *config.ServerConfig) bool {
	url := fmt.Sprintf("https://api-sepolia.etherscan.io/api?module=proxy&action=eth_getTransactionByHash&txhash=%s&apikey=%s", txHash, config.SepApiKey)

	resp, err := http.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var txResponse struct {
		Result struct {
			From        string `json:"from"`
			To          string `json:"to"`
			Value       string `json:"value"`
			Gas         string `json:"gas"`
			GasPrice    string `json:"gasPrice"`
			BlockNumber string `json:"blockNumber"`
		} `json:"result"`
	}

	err = json.NewDecoder(resp.Body).Decode(&txResponse)
	if err != nil {
		return false
	}

	if txResponse.Result.From == "" || txResponse.Result.To == "" || txResponse.Result.BlockNumber == "" {
		return false
	}

	senderWallet = strings.ToLower(senderWallet)
	receiverWallet := strings.ToLower(config.Wallet)
	txSender := strings.ToLower(txResponse.Result.From)
	txReceiver := strings.ToLower(txResponse.Result.To)

	if txSender != senderWallet {
		return false
	}

	if txReceiver != receiverWallet {
		return false
	}

	valueInWei, err := strconv.ParseInt(txResponse.Result.Value, 0, 64)
	if err != nil {
		return false
	}
	valueInEther := float64(valueInWei) / 1e18

	gasUsed, err := strconv.ParseInt(txResponse.Result.Gas, 0, 64)
	if err != nil {
		return false
	}

	gasPrice, err := strconv.ParseInt(txResponse.Result.GasPrice, 0, 64)
	if err != nil {
		return false
	}

	gasCostInEther := float64(gasUsed*gasPrice) / 1e18
	totalSpent := valueInEther + gasCostInEther

	if totalSpent < requiredAmount {
		return false
	}

	blockURL := fmt.Sprintf("https://api-sepolia.etherscan.io/api?module=proxy&action=eth_getBlockByNumber&tag=%s&boolean=true&apikey=%s", txResponse.Result.BlockNumber, config.SepApiKey)
	blockResp, err := http.Get(blockURL)
	if err != nil {
		return false
	}
	defer blockResp.Body.Close()

	if blockResp.StatusCode != http.StatusOK {
		return false
	}

	var blockResponse struct {
		Result struct {
			Timestamp string `json:"timestamp"`
		} `json:"result"`
	}

	err = json.NewDecoder(blockResp.Body).Decode(&blockResponse)
	if err != nil || blockResponse.Result.Timestamp == "" {
		return false
	}

	blockTimestamp, err := strconv.ParseInt(blockResponse.Result.Timestamp, 0, 64)
	if err != nil {
		return false
	}

	now := time.Now().Unix()
	allowedTime := now - 300 // cambiar margen

	if blockTimestamp < allowedTime {
		return false
	}

	return true
}
