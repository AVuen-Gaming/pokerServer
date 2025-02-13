package db

import (
	"errors"
	"server/internal/db/models"
)

func CreateTables(tournamentID uint, numTables int) error {
	var existingTables []models.Table
	err := DB.Where("tournament_id = ?", tournamentID).Find(&existingTables).Error
	if err != nil {
		return err
	}

	existingTableNumbers := make(map[int]bool)
	for _, table := range existingTables {
		existingTableNumbers[table.TableNumber] = true
	}

	var newTables []models.Table
	for i := 1; i <= numTables; i++ {
		if !existingTableNumbers[i] {
			newTables = append(newTables, models.Table{
				TournamentID: tournamentID,
				TableNumber:  i,
			})
		}
	}

	if len(newTables) > 0 {
		err = DB.Create(&newTables).Error
		if err != nil {
			return err
		}
	}

	return nil
}

func GetTablesByTournamentID(tournamentID uint) ([]models.Table, error) {
	var tables []models.Table
	result := DB.Where("tournament_id = ?", tournamentID).Find(&tables)
	if result.Error != nil {
		return nil, errors.New("error fetching tables for the tournament")
	}
	return tables, nil
}

func DeleteTableByTableAndTournament(tableID int, tournamentID int) error {
	result := DB.Where("id = ? AND tournament_id = ?", tableID, tournamentID).Delete(&models.Table{})
	if result.Error != nil {
		return errors.New("error deleting table for the specified table and tournament")
	}
	if result.RowsAffected == 0 {
		return errors.New("no rows affected, check table_id and tournament_id")
	}
	return nil
}
