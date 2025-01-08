package db

import (
	"errors"
	"server/internal/db/models"
	"time"
)

func InsertTournament(tournament models.Tournament) error {
	tournament.EndDate = nil
	result := DB.Create(&tournament)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func GetTournamentByID(tournamentID uint) (*models.Tournament, error) {
	var tournament models.Tournament

	result := DB.First(&tournament, tournamentID)
	if result.Error != nil {
		return nil, result.Error
	}

	return &tournament, nil
}

func GetAllTournaments() ([]models.Tournament, error) {
	var tournaments []models.Tournament

	result := DB.Find(&tournaments)
	if result.Error != nil {
		return nil, result.Error
	}

	return tournaments, nil
}

func GetTournamentByName(tournamentName string) (*models.Tournament, error) {
	var tournament models.Tournament

	result := DB.Where("name = ?", tournamentName).First(&tournament)
	if result.Error != nil {
		return nil, result.Error
	}

	return &tournament, nil
}

func GetOngoingTournaments(walletID uint) ([]models.Tournament, error) {
	var tournaments []models.Tournament

	result := DB.Model(&models.Tournament{}).
		Where("end_date IS NULL AND ongoing = ?", true).
		Joins("LEFT JOIN tournament_registrations ON tournaments.id = tournament_registrations.tournament_id AND tournament_registrations.wallet_id = ?", walletID).
		Where("tournament_registrations.id IS NULL").
		Find(&tournaments)
	if result.Error != nil {
		return nil, result.Error
	}

	return tournaments, nil
}

func GetUnregisteredTournaments(walletID uint) ([]models.Tournament, error) {
	var tournaments []models.Tournament

	result := DB.Model(&models.Tournament{}).
		Where("end_date IS NULL AND start = false").
		Joins("LEFT JOIN tournament_registrations ON tournaments.id = tournament_registrations.tournament_id AND tournament_registrations.wallet_id = ?", walletID).
		Where("tournament_registrations.id IS NULL").
		Find(&tournaments)
	if result.Error != nil {
		return nil, result.Error
	}

	return tournaments, nil
}

func GetRegisteredOngoingTournaments(walletID uint) ([]models.Tournament, error) {
	var tournaments []models.Tournament

	result := DB.Model(&models.Tournament{}).
		Joins("JOIN tournament_registrations ON tournaments.id = tournament_registrations.tournament_id").
		Where("tournament_registrations.wallet_id = ? AND tournament_registrations.eliminated = false AND tournaments.end_date IS NULL", walletID).
		Find(&tournaments)

	if result.Error != nil {
		return nil, result.Error
	}

	return tournaments, nil
}

func GetEliminatedAndFinishedTournaments(walletID uint) ([]models.Tournament, error) {
	var tournaments []models.Tournament

	result := DB.Model(&models.Tournament{}).
		Joins("JOIN tournament_registrations ON tournaments.id = tournament_registrations.tournament_id").
		Where("tournament_registrations.wallet_id = ? AND tournament_registrations.eliminated = true AND tournaments.end_date IS NOT NULL", walletID).
		Find(&tournaments)
	if result.Error != nil {
		return nil, result.Error
	}

	return tournaments, nil
}

func SetTournamentEndDate(tournamentID uint) error {
	var tournament models.Tournament
	result := DB.First(&tournament, tournamentID)
	if result.Error != nil {
		return errors.New("tournament not found")
	}

	now := time.Now()
	tournament.EndDate = &now

	if saveErr := DB.Save(&tournament).Error; saveErr != nil {
		return errors.New("error updating tournament end date")
	}

	return nil
}

func UpdateTournamentOngoing(tournamentID uint, ongoing bool) error {
	var tournament models.Tournament
	result := DB.First(&tournament, tournamentID)
	if result.Error != nil {
		return errors.New("tournament not found")
	}

	tournament.Ongoing = ongoing

	if saveErr := DB.Save(&tournament).Error; saveErr != nil {
		return errors.New("error updating tournament ongoing status")
	}

	return nil
}

func UpdateTournamentLastIncrementBlind(tournamentID uint, lastIncrementBlind time.Time) error {
	var tournament models.Tournament
	result := DB.First(&tournament, tournamentID)
	if result.Error != nil {
		return errors.New("tournament not found")
	}

	tournament.LastIncrementBlind = lastIncrementBlind

	if saveErr := DB.Save(&tournament).Error; saveErr != nil {
		return errors.New("error updating tournament ongoing status")
	}

	return nil
}

func UpdateTournamentStart(tournamentID uint, start bool) error {
	var tournament models.Tournament
	result := DB.First(&tournament, tournamentID)
	if result.Error != nil {
		return errors.New("tournament not found")
	}

	tournament.Start = start

	if saveErr := DB.Save(&tournament).Error; saveErr != nil {
		return errors.New("error updating tournament start status")
	}

	return nil
}
