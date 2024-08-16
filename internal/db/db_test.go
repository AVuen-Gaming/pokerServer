package db

import (
	"server/config"
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
