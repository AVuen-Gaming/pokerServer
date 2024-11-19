package db

import (
	"server/internal/db/models"

	"gorm.io/gorm"
)

func UserExists(username string) (bool, error) {
	var user models.User
	result := DB.Where("username = ?", username).First(&user)
	if result.Error != nil {
		if result.Error.Error() == "record not found" {
			return false, nil
		}
		return false, result.Error
	}
	return true, nil
}

func CreateUserWithWallet(username, walletAddress string) (*models.User, error) {
	user := &models.User{
		Username: username,
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}

		wallet := &models.Wallet{
			UserID:        user.ID,
			WalletAddress: walletAddress,
		}

		if err := tx.Create(wallet).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return user, nil
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
