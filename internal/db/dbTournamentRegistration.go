package db

import (
	"errors"
	"server/internal/db/models"
)

func UpdateTournamentRegistrationEliminated(tournamentID int, walletID uint) error {
	var registration models.TournamentRegistration
	result := DB.Where("tournament_id = ? AND wallet_id = ?", tournamentID, walletID).First(&registration)
	if result.Error != nil {
		if result.RowsAffected == 0 {
			return errors.New("no tournament registration found with the specified tournamentID and walletID")
		}
		return errors.New("error fetching tournament registration")
	}

	registration.Eliminated = true

	if saveErr := DB.Save(&registration).Error; saveErr != nil {
		return errors.New("error updating eliminated status for tournament registration")
	}

	return nil
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

func GetTournamentRegistrationByTournamentAndWallet(tournamentID uint, walletID uint) (*models.TournamentRegistration, error) {
	var registration models.TournamentRegistration
	result := DB.Where("tournament_id = ? AND wallet_id = ?", tournamentID, walletID).First(&registration)
	if result.Error != nil {
		if result.RowsAffected == 0 {
			return nil, nil
		}
		return nil, result.Error
	}
	return &registration, nil
}
