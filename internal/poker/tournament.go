package poker

import (
	"fmt"
	"time"
)

type Tournament struct {
	ID                    uint
	Name                  string
	RegistrationStartDate time.Time
	RegistrationEndDate   time.Time
	StartDate             time.Time
	EndDate               time.Time
	EntryCost             float32
	Currency              string
	Tables                []Table
	Players               []Player
	Prize                 string
	Configuration         string
	Ongoing               bool
	MinPlayers            int
	MaxPlayers            int
	TurnSeconds           int
	StartChips            int
	BBValue               int
	Start                 bool
}

func (tournament *Tournament) CreateTablesForTournament() {
	numPlayers := len(tournament.Players)
	numTables := numPlayers / tournament.MaxPlayers
	if numPlayers%tournament.MaxPlayers != 0 {
		numTables++
	}

	tables := make([]Table, numTables)

	playerIndex := 0

	for i := 0; i < numTables; i++ {
		table := Table{
			ID:      fmt.Sprintf("Table-%d", i+1),
			Players: []Player{},
		}

		for j := 0; j < tournament.MaxPlayers && playerIndex < numPlayers; j++ {
			table.Players = append(table.Players, tournament.Players[playerIndex])
			playerIndex++
		}

		tables[i] = table
	}

	tournament.Tables = tables
}
