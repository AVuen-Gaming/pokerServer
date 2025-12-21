package temporal

import (
	"testing"
	"time"

	"server/internal/poker"

	"github.com/stretchr/testify/assert"
)

func TestUpdateTableFromUpdatedTablePropagatesPlayersAndMetadata(t *testing.T) {
	original := poker.Table{
		ID:      "table-1",
		Round:   3,
		BBValue: 200,
		Players: []poker.Player{
			{ID: "alice", Chips: 800},
			{ID: "bob", Chips: 600},
		},
	}

	updated := poker.Table{
		ID:                       "table-1",
		Round:                    4,
		BBValue:                  400,
		TotalPlayersInTournament: 12,
		CurrentTurn:              "alice",
		CurrentStage:             poker.StageTurn,
		Players: []poker.Player{
			{ID: "alice", Chips: 1600, LastAction: "win"},
			{ID: "charlie", Chips: 1200},
		},
	}

	merged := updateTableFromUpdatedTable(original, updated)

	if assert.Len(t, merged.Players, 2, "expected eliminated players to be removed") {
		assert.Equal(t, "alice", merged.Players[0].ID)
		assert.Equal(t, 1600, merged.Players[0].Chips)
		assert.Equal(t, "charlie", merged.Players[1].ID)
	}
	assert.Equal(t, poker.StageTurn, merged.CurrentStage)
	assert.Equal(t, 4, merged.Round)
	assert.Equal(t, 400, merged.BBValue)
	assert.Equal(t, 12, merged.TotalPlayersInTournament)
}

func TestUpdateTableFromUpdatedTableKeepsEliminationFlags(t *testing.T) {
	original := poker.Table{
		ID: "table-55",
		Players: []poker.Player{
			{ID: "dave", Chips: 50},
			{ID: "erin", Chips: 50},
		},
	}

	updated := poker.Table{
		ID: "table-55",
		Players: []poker.Player{
			{ID: "dave", Chips: 100, IsEliminated: true, LastAction: "bust"},
		},
		Winners:            []poker.Player{{ID: "frank", Chips: 1400}},
		LastIncrementBlind: time.Now(),
	}

	merged := updateTableFromUpdatedTable(original, updated)

	if assert.Len(t, merged.Players, 1) {
		assert.True(t, merged.Players[0].IsEliminated, "player elimination flag should propagate")
		assert.Equal(t, "bust", merged.Players[0].LastAction)
	}
	assert.Len(t, merged.Winners, 1)
	assert.Equal(t, "frank", merged.Winners[0].ID)
	assert.WithinDuration(t, updated.LastIncrementBlind, merged.LastIncrementBlind, time.Millisecond)
}
