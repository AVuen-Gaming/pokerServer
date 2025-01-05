package db

import (
	"errors"
	"server/internal/db/models"
)

func InsertTablePlayer(tableID, walletID, tournamentID uint) error {
	var existingRecord models.TablePlayer
	result := DB.Where("table_id = ? AND wallet_id = ? AND tournament_id = ?", tableID, walletID, tournamentID).First(&existingRecord)

	if result.RowsAffected > 0 {
		return nil
	}

	newRecord := models.TablePlayer{
		TableID:      tableID,
		WalletID:     walletID,
		TournamentID: tournamentID,
	}

	err := DB.Create(&newRecord).Error
	if err != nil {
		return err
	}

	return nil
}

func GetTablePlayersByWalletAndTournament(walletID uint, tournamentID uint) ([]models.TablePlayer, error) {
	var tablePlayers []models.TablePlayer
	result := DB.Where("wallet_id = ? AND tournament_id = ?", walletID, tournamentID).Find(&tablePlayers)
	if result.Error != nil {
		return nil, errors.New("error fetching table players for the given wallet and tournament")
	}
	return tablePlayers, nil
}
