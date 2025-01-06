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
