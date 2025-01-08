package db

import (
	"fmt"
	"server/internal/db/models"
)

func InsertPrize(prize *models.Prize) error {
	if err := DB.Create(prize).Error; err != nil {
		return fmt.Errorf("error insertando el premio en la base de datos: %v", err)
	}
	return nil
}

func GetPrizeByTournamentID(tournamentID uint) (*models.Prize, error) {
	var prize models.Prize
	result := DB.Where("tournament_id = ?", tournamentID).First(&prize)
	if result.Error != nil {
		return nil, result.Error
	}
	return &prize, nil
}

func UpdatePrizeList(tournamentID uint, updatedPrizeList []byte) error {
	result := DB.Model(&models.Prize{}).
		Where("tournament_id = ?", tournamentID).
		Update("prize_list", updatedPrizeList)
	if result.Error != nil {
		return fmt.Errorf("error actualizando la lista de premios: %v", result.Error)
	}
	return nil
}
