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

func TestCompareHands(t *testing.T) {
	table := &Table{
		FlopCards: []Card{
			{Suit: "Clubs", Value: "6"},
			{Suit: "Hearts", Value: "6"},
			{Suit: "Diamonds", Value: "6"},
		},
		TurnCard:  &Card{Suit: "Spades", Value: "7"},
		RiverCard: &Card{Suit: "Hearts", Value: "2"},
		Players: []Player{
			// Player 1 with a four-of-a-kind (poker)
			{ID: "player1", Cards: []Card{{Suit: "Clubs", Value: "6"}, {Suit: "Spades", Value: "2"}}},
			// Player 2 with a full house
			{ID: "player2", Cards: []Card{{Suit: "Hearts", Value: "K"}, {Suit: "Diamonds", Value: "K"}}},
			// Player 3 with a flush
			{ID: "player3", Cards: []Card{{Suit: "Hearts", Value: "Q"}, {Suit: "Hearts", Value: "J"}}, HasFold: true},
			// Player 4 with a straight
			{ID: "player4", Cards: []Card{{Suit: "Spades", Value: "8"}, {Suit: "Clubs", Value: "7"}}, HasFold: true},
			// Player 5 with a pair
			{ID: "player5", Cards: []Card{{Suit: "Diamonds", Value: "3"}, {Suit: "Clubs", Value: "4"}}},
		},
	}

	// Evaluar las manos de todos los jugadores
	table.EvaluateHand()

	if table.Winners == nil {
		t.Errorf("No winner determined")
		return
	}

	// Verifica que el ganador sea el esperado
	expectedWinner := "player1"
	if table.Winners[0].ID != expectedWinner {
		t.Errorf("Expected winner: %s, but got: %s", expectedWinner, table.Winners[0].ID)
	}
}

func TestConvertCardToEvalCard(t *testing.T) {
	card := Card{Suit: "Hearts", Value: "A"}
	evalCard := convertCardToEvalCard(card)
	// Imprime para verificar el resultado
	t.Logf("Converted eval card: %v", evalCard)
}
