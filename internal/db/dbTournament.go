package db

import (
	"server/internal/db/models"
)

func InsertTournament(tournament models.Tournament) error {
	result := DB.Create(&tournament)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func GetWalletByAddress(walletAddress string) (*models.Wallet, error) {
	var wallet models.Wallet
	result := DB.Where("wallet_address = ?", walletAddress).First(&wallet)
	if result.Error != nil {
		return nil, result.Error
	}
	return &wallet, nil
}

func RegisterUserToTournament(tournamentID, walletID uint) error {
	registration := &models.TournamentRegistration{
		TournamentID: tournamentID,
		WalletID:     walletID,
	}

	result := DB.Create(registration)
	if result.Error != nil {
		return result.Error
	}

	return nil
}

func GetTournamentRegistrationsByTournamentID(tournamentID uint) ([]models.TournamentRegistration, error) {
	var registrations []models.TournamentRegistration

	result := DB.Where("tournament_id = ?", tournamentID).
		Preload("Wallet").
		Find(&registrations)

	if result.Error != nil {
		return nil, result.Error
	}

	return registrations, nil
}

func GetTournamentByID(tournamentID uint) (*models.Tournament, error) {
	var tournament models.Tournament

	result := DB.First(&tournament, tournamentID)
	if result.Error != nil {
		return nil, result.Error
	}

	return &tournament, nil
}

func GetAllTournaments() ([]models.Tournament, error) {
	var tournaments []models.Tournament

	result := DB.Find(&tournaments)
	if result.Error != nil {
		return nil, result.Error
	}

	return tournaments, nil
}

func GetTournamentByName(tournamentName string) (*models.Tournament, error) {
	var tournament models.Tournament

	result := DB.Where("name = ?", tournamentName).First(&tournament)
	if result.Error != nil {
		return nil, result.Error
	}

	return &tournament, nil
}
