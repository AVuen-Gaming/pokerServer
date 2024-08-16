package poker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDealCards(t *testing.T) {
	table := &Table{}
	table.DealCards()

	assert.Equal(t, 7, len(table.Players), "Should have cards for all players")
	assert.Len(t, table.FlopCards, 3, "There should be 3 cards in the flop")
	assert.NotNil(t, table.TurnCard, "There should be 1 card on the turn")
	assert.NotNil(t, table.RiverCard, "There should be 1 card on the river")
}
