package db

import (
	"fmt"
	"server/config"
	"server/internal/db/models"
	"testing"

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
