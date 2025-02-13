package db

import (
	"errors"
	"server/internal/db/models"
)

func InsertRanking(tournamentID int, walletID uint) (*models.Ranking, error) {
	var lastRanking models.Ranking
	result := DB.Where("tournament_id = ?", tournamentID).
		Last(&lastRanking)

	var newPosition int
	if result.RowsAffected == 0 {
		var count int64
		countResult := DB.Model(&models.TournamentRegistration{}).
			Where("tournament_id = ?", tournamentID).
			Count(&count)

		if countResult.Error != nil {
			return nil, errors.New("error counting tournament registrations")
		}

		newPosition = int(count)
	} else {
		newPosition = lastRanking.Position - 1
	}

	newRanking := &models.Ranking{
		TournamentID: uint(tournamentID),
		WalletID:     walletID,
		Position:     newPosition,
	}

	if insertErr := DB.Create(newRanking).Error; insertErr != nil {
		return nil, errors.New("error inserting new ranking")
	}

	return newRanking, nil
}

func GetRankingsByTournamentID(tournamentID uint) ([]models.Ranking, error) {
	var rankings []models.Ranking

	result := DB.Where("tournament_id = ?", tournamentID).
		Order("position DESC").
		Find(&rankings)

	if result.Error != nil {
		return nil, result.Error
	}

	return rankings, nil
}

func GetRankingByTournamentAndWallet(tournamentID uint, walletID uint) (*models.Ranking, error) {
	var ranking models.Ranking

	result := DB.Where("tournament_id = ? AND wallet_id = ?", tournamentID, walletID).
		Order("position DESC").
		First(&ranking)
	if result.Error != nil {
		if result.RowsAffected == 0 {
			return nil, nil
		}
		return nil, result.Error
	}

	return &ranking, nil
}

func RankingExists(tournamentID int, walletId uint) (bool, error) {
	var count int64
	err := DB.Model(&models.Ranking{}).Where("tournament_id = ? AND wallet_id = ?", tournamentID, walletId).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
