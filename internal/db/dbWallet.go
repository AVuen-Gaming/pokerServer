package db

import (
	"errors"
	"server/internal/db/models"
)

func GetWalletIDByPlayerID(walletAddress string) (uint, error) {
	var wallet models.Wallet
	result := DB.Where("wallet = ?", walletAddress).First(&wallet)
	if result.Error != nil {
		return 0, errors.New("wallet not found")
	}
	return wallet.ID, nil
}

func GetWalletByAddress(walletAddress string) (*models.Wallet, error) {
	var wallet models.Wallet
	result := DB.Where("wallet = ?", walletAddress).First(&wallet)
	if result.Error != nil {
		return nil, result.Error
	}
	return &wallet, nil
}

func WalletExistsByAddress(walletAddress string) (bool, error) {
	var wallet models.Wallet
	result := DB.Select("id").Where("wallet = ?", walletAddress).First(&wallet)
	if result.Error != nil {
		if result.RowsAffected == 0 {
			return false, nil
		}
		return false, result.Error
	}
	return true, nil
}
