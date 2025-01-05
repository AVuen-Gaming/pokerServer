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
