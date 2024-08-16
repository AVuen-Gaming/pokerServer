package db

import (
	"fmt"
	"log"

	"server/config"
	"server/internal/db/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB(cfg *config.DatabaseConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode)
	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Error connecting to the database: %v", err)
		return nil, err
	}
	log.Println("Connected to the database")
	return DB, nil
}

func Migrate() error {
	err := DB.AutoMigrate(&models.User{}, &models.Wallet{}, &models.Tournament{}, &models.TournamentChip{}, &models.TournamentRegistration{}, &models.Table{}, &models.TablePlayer{}, &models.Ranking{}, &models.RecordLog{})
	if err != nil {
		log.Fatalf("Error migrating database: %v", err)
		return err
	}
	log.Println("Database migration completed")
	return nil
}

func GetDB() *gorm.DB {
	return DB
}
