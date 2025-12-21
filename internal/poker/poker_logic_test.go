package poker

import (
	"fmt"
	"sort"
	"testing"
)

func TestTournamentCreationAndRegistrationSimulation(t *testing.T) {
	tournament := Tournament{
		MaxPlayers: 4,
		Players:    []Player{},
	}

	for i := 0; i < 7; i++ {
		tournament.Players = append(tournament.Players, Player{
			ID:    fmt.Sprintf("player-%d", i+1),
			Chips: 1000,
		})
	}

	tournament.CreateTablesForTournament()

	if len(tournament.Tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tournament.Tables))
	}

	totalPlayers := 0
	seen := map[string]bool{}
	for _, table := range tournament.Tables {
		if len(table.Players) > tournament.MaxPlayers {
			t.Fatalf("table %s exceeded max players: %d", table.ID, len(table.Players))
		}
		totalPlayers += len(table.Players)
		for _, player := range table.Players {
			seen[player.ID] = true
		}
	}

	if totalPlayers != len(tournament.Players) {
		t.Fatalf("expected %d registered players, found %d", len(tournament.Players), totalPlayers)
	}

	if len(seen) != len(tournament.Players) {
		t.Fatalf("expected all players to be assigned to a table")
	}
}

func TestPlanReshuffleMovesPlayersFromUnderfilledTable(t *testing.T) {
	tables := []Table{
		{
			ID:           "1",
			TournamentID: 1,
			MinPlayers:   2,
			MaxPlayers:   5,
			Players:      []Player{{ID: "A"}, {ID: "B"}, {ID: "C"}},
		},
		{
			ID:           "2",
			TournamentID: 1,
			MinPlayers:   2,
			MaxPlayers:   5,
			Players:      []Player{{ID: "D"}, {ID: "E"}, {ID: "F"}, {ID: "G"}},
		},
		{
			ID:           "3",
			TournamentID: 1,
			MinPlayers:   2,
			MaxPlayers:   9,
			Players:      []Player{{ID: "H"}, {ID: "I"}, {ID: "J"}, {ID: "K"}, {ID: "L"}, {ID: "M"}, {ID: "N"}},
		},
	}

	updated, transfers, removed := planReshuffleTables(tables, tables[0])
	if removed {
		t.Fatalf("table should not be removed when still above minimum")
	}
	if len(transfers) == 0 {
		t.Fatalf("expected at least one player transfer")
	}

	var tableOne, tableTwo *Table
	for i := range updated {
		if updated[i].ID == "1" {
			tableOne = &updated[i]
		}
		if updated[i].ID == "2" {
			tableTwo = &updated[i]
		}
	}

	if tableOne == nil || tableTwo == nil {
		t.Fatalf("tables not found after reshuffle")
	}

	if len(tableOne.Players) >= 3 {
		t.Fatalf("expected table 1 to have fewer players after reshuffle")
	}

	if len(tableTwo.Players) <= 2 {
		t.Fatalf("expected table 2 to receive players")
	}

	for _, transfer := range transfers {
		if !transfer.Player.SwitchingTable {
			t.Fatalf("transferred player should be flagged as switching")
		}
	}
}

func TestPlanReshuffleRemovesFinishedTable(t *testing.T) {
	baseTables := []Table{
		{
			ID:           "1",
			TournamentID: 42,
			MinPlayers:   2,
			MaxPlayers:   5,
			Players:      []Player{{ID: "A"}, {ID: "B"}},
		},
		{
			ID:           "2",
			TournamentID: 42,
			MinPlayers:   2,
			MaxPlayers:   5,
			Players:      []Player{{ID: "C"}, {ID: "D"}, {ID: "E"}, {ID: "F"}},
		},
	}

	current := baseTables[0]
	current.Players = nil // simulate table already emptied after moving players

	updated, transfers, removed := planReshuffleTables(baseTables, current)

	if !removed {
		t.Fatalf("expected table 1 to be removed after running out of players")
	}
	if len(updated) != 1 {
		t.Fatalf("expected one table to remain, got %d", len(updated))
	}
	if len(transfers) != 0 {
		t.Fatalf("no transfers should happen when table already empty")
	}
	if updated[0].ID != "2" {
		t.Fatalf("table 2 should remain active")
	}
}

func TestPlanReshuffleTrimsShortHandedTableToMinimum(t *testing.T) {
	tables := []Table{
		buildReshuffleTable("short", 2, 9, 5),
		buildReshuffleTable("mid", 2, 9, 8),
		buildReshuffleTable("deep", 2, 9, 7),
	}

	updated, transfers, removed := planReshuffleTables(tables, tables[0])
	if removed {
		t.Fatalf("short-handed table should not be removed when still above minimum")
	}
	if len(transfers) == 0 {
		t.Fatalf("expected players to move away from the short-handed table")
	}

	for _, tbl := range updated {
		if tbl.ID == "short" {
			if len(tbl.Players) != tbl.MinPlayers {
				t.Fatalf("short-handed table should be trimmed to its minimum, got %d", len(tbl.Players))
			}
		} else if len(tbl.Players) <= 8 {
			t.Fatalf("receiving tables should gain seats, table %s has %d", tbl.ID, len(tbl.Players))
		}
	}

	for _, transfer := range transfers {
		if !transfer.Player.SwitchingTable {
			t.Fatalf("moved player must set SwitchingTable=true")
		}
	}
}

func TestPlanReshuffleMarksSinglePlayerTableAsStopped(t *testing.T) {
	tables := []Table{
		buildReshuffleTable("solo", 2, 9, 1),
		buildReshuffleTable("full", 2, 9, 9),
	}
	updated, transfers, removed := planReshuffleTables(tables, tables[0])
	if removed {
		t.Fatalf("single-player table should not be immediately deleted while still holding a player")
	}
	if len(transfers) != 0 {
		t.Fatalf("no transfers should happen while table is waiting with a single player")
	}
	if !updated[0].Stopped {
		t.Fatalf("single-player table must be flagged as stopped so MovePlayers can relocate it")
	}
}

func TestPlanReshuffleMaintainsThreeTablesWhenExtraTableEmpty(t *testing.T) {
	tables := []Table{
		buildReshuffleTable("1", 2, 9, 9),
		buildReshuffleTable("2", 2, 9, 9),
		buildReshuffleTable("3", 2, 9, 9),
		buildReshuffleTable("ghost", 2, 9, 0),
	}

	updated, transfers, removed := planReshuffleTables(tables, tables[3])
	if !removed {
		t.Fatalf("empty table should be removed so the tournament keeps only the active full-ring tables")
	}
	if len(transfers) != 0 {
		t.Fatalf("no transfers expected when the table had no players")
	}
	if len(updated) != 3 {
		t.Fatalf("expected exactly three tables after removing the empty one")
	}
	for _, tbl := range updated {
		if len(tbl.Players) != 9 {
			t.Fatalf("each remaining table should stay full-ring with 9 players, got %d", len(tbl.Players))
		}
	}
}

func buildReshuffleTable(id string, minPlayers, maxPlayers, count int) Table {
	players := make([]Player, count)
	for i := 0; i < count; i++ {
		players[i] = Player{ID: fmt.Sprintf("%s-P%d", id, i+1)}
	}
	return Table{ID: id, MinPlayers: minPlayers, MaxPlayers: maxPlayers, Players: players}
}

func maxInt(values []int) int {
	clone := append([]int(nil), values...)
	sort.Ints(clone)
	return clone[len(clone)-1]
}

func minInt(values []int) int {
	clone := append([]int(nil), values...)
	sort.Ints(clone)
	return clone[0]
}

func TestOnlyOneTableRemainsFlagsFinalTable(t *testing.T) {
	tables := []Table{
		{ID: "1"},
		{ID: "2"},
	}

	OnlyOneTableRemains(tables)
	if tables[0].LastTable {
		t.Fatalf("table should not be flagged as last when more than one table exists")
	}

	single := []Table{{ID: "Final"}}
	OnlyOneTableRemains(single)
	if !single[0].LastTable {
		t.Fatalf("single remaining table must be marked as last table")
	}
}

func TestTableEndSetsEndFlagWhenHeadsUp(t *testing.T) {
	table := &Table{ID: "10", Players: []Player{{ID: "winner"}}}

	table.TableEnd()

	if !table.TableEnds {
		t.Fatalf("table with <=1 players should end")
	}
	if table.EndTableTime.IsZero() {
		t.Fatalf("table end should stamp end time")
	}
}

func TestApplyPlayerActionRaiseDeductsChips(t *testing.T) {
	table := &Table{
		Players: []Player{
			{ID: "player-1", Chips: 900},
		},
		BBValue: 20,
	}

	table.applyPlayerAction(0, Player{LastAction: "raise", LastBet: 50})

	if table.Players[0].Chips != 850 {
		t.Fatalf("expected chips to be 850, got %d", table.Players[0].Chips)
	}
	if table.Players[0].TotalBet != 50 {
		t.Fatalf("expected total bet to be 50, got %d", table.Players[0].TotalBet)
	}
}

func TestSetEliminatePlayersWithNoChips(t *testing.T) {
	table := &Table{
		Players: []Player{{ID: "player-1", Chips: 0}},
	}

	table.SetEliminatePlayersWithNoChips()

	if !table.Players[0].IsEliminated {
		t.Fatalf("expected player to be eliminated")
	}
}

func TestAssignChipsToWinners(t *testing.T) {
	table := &Table{
		Players:  []Player{{ID: "winner", Chips: 100}},
		Winners:  []Player{{ID: "winner"}},
		TotalBet: 200,
	}

	table.AssignChipsToWinners()

	if table.Players[0].Chips != 300 {
		t.Fatalf("expected winner chips to be 300, got %d", table.Players[0].Chips)
	}
	if table.TotalBet != 0 {
		t.Fatalf("expected total bet to be cleared")
	}
}

func TestTournamentLifecycleSimulationEndsWithSingleWinner(t *testing.T) {
	tournament := Tournament{
		MaxPlayers: 2,
		Players: []Player{
			{ID: "P1", Chips: 400},
			{ID: "P2", Chips: 400},
			{ID: "P3", Chips: 400},
			{ID: "P4", Chips: 400},
		},
	}

	tournament.CreateTablesForTournament()
	if len(tournament.Tables) != 2 {
		t.Fatalf("expected 2 tables got %d", len(tournament.Tables))
	}

	survivors := map[string]bool{}
	for i := range tournament.Tables {
		table := &tournament.Tables[i]
		table.BBValue = 20
		table.CurrentSB = table.Players[0].ID
		table.CurrentBB = table.Players[len(table.Players)-1].ID
		table.DealCards(nil)
		simulateHeadsUpHand(table)
		if !table.TableEnds {
			t.Fatalf("table %s should have finished after simulation", table.ID)
		}
		if len(table.Winners) != 1 {
			t.Fatalf("expected a single winner at table %s", table.ID)
		}
		survivors[table.Winners[0].ID] = true
	}

	if len(survivors) != len(tournament.Tables) {
		t.Fatalf("expected one survivor per table, got %d", len(survivors))
	}
}

func simulateHeadsUpHand(table *Table) {
	if len(table.Players) < 2 {
		return
	}
	table.SetSMBB()
	// Big blind calls, small blind raises to force action
	bbIndex := table.FindPlayerByID(table.CurrentBB)
	sbIndex := table.FindPlayerByID(table.CurrentSB)
	if bbIndex >= 0 {
		table.applyPlayerAction(bbIndex, Player{LastAction: "call", LastBet: 0})
	}
	if sbIndex >= 0 {
		table.applyPlayerAction(sbIndex, Player{LastAction: "raise", LastBet: table.BBValue})
	}
	// Remaining players fold to finish the hand
	for i := range table.Players {
		if table.Players[i].ID != table.CurrentSB {
			table.applyPlayerAction(i, Player{LastAction: "fold"})
			table.Players[i].Chips = 0
		}
	}
	table.AllPlayersExceptOneFold()
	table.AssignChipsToWinners()
	table.SetEliminatePlayersWithNoChips()
	table.RemovePlayersEliminatedWithNoChips()
	table.TableEnd()
}
