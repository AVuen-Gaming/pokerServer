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

func GetTablePlayersByWalletAndTournament(walletID uint, tournamentID uint) (models.TablePlayer, error) {
	var tablePlayers models.TablePlayer
	result := DB.Where("wallet_id = ? AND tournament_id = ?", walletID, tournamentID).Find(&tablePlayers)
	if result.Error != nil {
		return tablePlayers, errors.New("error fetching table players for the given wallet and tournament")
	}
	return tablePlayers, nil
}

func UpdateTablePlayerTableID(walletID uint, tournamentID int, newTableID int) error {
	result := DB.Model(&models.TablePlayer{}).
		Where("wallet_id = ? AND tournament_id = ?", walletID, tournamentID).
		Update("table_id", newTableID)

	if result.Error != nil {
		return errors.New("error updating table_id for the specified wallet and tournament")
	}

	if result.RowsAffected == 0 {
		return errors.New("no rows affected, check wallet_id and tournament_id")
	}

	return nil
}

func DeleteTablePlayerByTableAndTournament(tableID int, tournamentID int) error {
	result := DB.Where("table_id = ? AND tournament_id = ?", tableID, tournamentID).Delete(&models.TablePlayer{})

	if result.Error != nil {
		return errors.New("error deleting table player for the specified table and tournament")
	}

	if result.RowsAffected == 0 {
		return errors.New("no rows affected, check table_id and tournament_id")
	}

	return nil
}
