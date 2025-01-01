package db

import (
	"server/internal/db/models"

	"gorm.io/gorm"
)

func CreateUserWithWallet(wallet string) error {

	walletDTO := &models.Wallet{}
	walletDTO.Wallet = wallet
	result := DB.Create(&walletDTO)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func CheckUserRegistration(tournamentID, walletID uint) (bool, error) {
	var registration models.TournamentRegistration

	result := DB.Where("tournament_id = ? AND wallet_id = ?", tournamentID, walletID).First(&registration)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, result.Error
	}

	return true, nil
}
