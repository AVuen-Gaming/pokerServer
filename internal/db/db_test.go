package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"server/config"
	"server/internal/db/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInitDB(t *testing.T) {
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	InitDB(&cfg.Database)

	assert.NotNil(t, DB, "DB should not be nil")

	db, err := DB.DB()
	assert.NoError(t, err)
	err = db.Ping()
	assert.NoError(t, err, "Should be able to ping the database")
}

func TestMigrate(t *testing.T) {
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	InitDB(&cfg.Database)

	err = Migrate()
	assert.NoError(t, err, "Database migration should not return an error")
}

func TestInsertMultipleWallets(t *testing.T) {
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	InitDB(&cfg.Database)

	DB.Exec("DELETE FROM wallets")

	var wallets []models.Wallet

	for i := 0; i < 300; i++ {
		wallets = append(wallets, models.Wallet{
			Wallet: fmt.Sprintf("0x%040x", i),
		})
	}

	result := DB.Create(&wallets)
	assert.NoError(t, result.Error, "Error al insertar wallets en la base de datos")
	assert.Equal(t, int64(300), result.RowsAffected, "Deben haberse insertado exactamente 300 wallets")

	var count int64
	DB.Model(&models.Wallet{}).Count(&count)
	assert.Equal(t, int64(300), count, "La cantidad de wallets en la base de datos debe ser 300")
}

func TestRegisterMultipleWalletsToTournament(t *testing.T) {
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	InitDB(&cfg.Database)

	var wallets []models.Wallet
	DB.Limit(300).Find(&wallets)
	assert.Equal(t, 300, len(wallets), "Deben existir 300 wallets en la base de datos")

	for _, wallet := range wallets {
		err := RegisterUserToTournament(1, wallet.ID)
		assert.NoError(t, err, "Error al registrar wallet en el torneo")
	}

	var count int64
	DB.Model(&models.TournamentRegistration{}).Where("tournament_id = ?", 1).Count(&count)
	assert.Equal(t, int64(300), count, "Debe haber 300 registros en el torneo")
}

type TournamentResponse struct {
	ID uint `json:"id"`
	// Otros campos que sean necesarios…
}

func TestCreateTournamentAndRegisterMultipleUsers(t *testing.T) {
	cfg, err := config.LoadConfig()
	assert.NoError(t, err)

	InitDB(&cfg.Database)

	now := time.Now().UTC()
	registrationStartDate := now.Format(time.RFC3339)
	registrationEndDate := now.Add(1 * time.Minute).Format(time.RFC3339)
	startDate := now.Add(75 * time.Second).Format(time.RFC3339)
	tournamentName := fmt.Sprintf("Test Tournament %d", now.Unix())

	bodyData := map[string]interface{}{
		"name":                    tournamentName,
		"registration_start_date": registrationStartDate,
		"registration_end_date":   registrationEndDate,
		"start_date":              startDate,
		"entry_cost":              0.001,
		"increment_blind":         1,
		"currency":                "sepolia",
		"prize":                   "50000 USD",
		"ongoing":                 false,
		"start":                   false,
		"configuration":           "Texas Hold'em",
		"min_players":             2,
		"max_players":             306,
		"turn_seconds":            1,
		"start_chips":             10,
		"bb_value":                20,
	}
	bodyBytes, err := json.Marshal(bodyData)
	assert.NoError(t, err)

	req, err := http.NewRequest("POST", "http://localhost:7000/tournaments", bytes.NewReader(bodyBytes))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "localhost:3000")
	req.Header.Set("Authorization", "Bearer popio")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated, "El código de respuesta debe ser 200 o 201")

	tournamet, err := GetTournamentByName(tournamentName)
	assert.NoError(t, err)

	var tournamentResp TournamentResponse
	err = json.NewDecoder(resp.Body).Decode(&tournamentResp)
	assert.NoError(t, err)

	DB.Unscoped().Exec("DELETE FROM wallets")

	var wallets []models.Wallet
	for i := 0; i < 300; i++ {
		wallets = append(wallets, models.Wallet{
			Wallet: fmt.Sprintf("0x%040x", i),
		})
	}
	result := DB.Create(&wallets)
	assert.NoError(t, result.Error, "Error al insertar wallets en la base de datos")
	assert.Equal(t, int64(300), result.RowsAffected, "Deben haberse insertado exactamente 300 wallets")

	var count int64
	DB.Model(&models.Wallet{}).Count(&count)
	assert.Equal(t, int64(300), count, "La cantidad de wallets en la base de datos debe ser 300")

	for _, wallet := range wallets {
		err := RegisterUserToTournament(tournamet.ID, wallet.ID)
		assert.NoError(t, err, "Error al registrar wallet en el torneo")
	}
}
