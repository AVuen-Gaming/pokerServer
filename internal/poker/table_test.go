package poker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDealCards(t *testing.T) {
	table := &Table{Players: make([]Player, 7)}

	table.DealCards(nil)

	for i, player := range table.Players {
		assert.Lenf(t, player.Cards, 2, "player %d should have two cards", i)
	}
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
			{ID: "player1", Cards: []Card{{Suit: "Clubs", Value: "6"}, {Suit: "Spades", Value: "2"}}},
			{ID: "player2", Cards: []Card{{Suit: "Hearts", Value: "K"}, {Suit: "Diamonds", Value: "K"}}},
			{ID: "player3", Cards: []Card{{Suit: "Hearts", Value: "Q"}, {Suit: "Hearts", Value: "J"}}, HasFold: true},
			{ID: "player4", Cards: []Card{{Suit: "Spades", Value: "8"}, {Suit: "Clubs", Value: "7"}}, HasFold: true},
			{ID: "player5", Cards: []Card{{Suit: "Diamonds", Value: "3"}, {Suit: "Clubs", Value: "4"}}},
		},
	}

	table.EvaluateHand()

	if len(table.Winners) == 0 {
		t.Fatalf("expected at least one winner")
	}

	for _, winner := range table.Winners {
		playerIndex := table.FindPlayerByID(winner.ID)
		if playerIndex == -1 {
			t.Fatalf("winner %s not found in table", winner.ID)
		}
		if table.Players[playerIndex].HasFold {
			t.Fatalf("folded player %s should not be a winner", winner.ID)
		}
	}
}

func TestConvertCardToEvalCard(t *testing.T) {
	card := Card{Suit: "Hearts", Value: "A"}
	evalCard := convertCardToEvalCard(card)
	// Imprime para verificar el resultado
	t.Logf("Converted eval card: %v", evalCard)
}

func TestTableStageLifecycleFromPreFlopToRiver(t *testing.T) {
	table := &Table{
		ID:      "T-1",
		BBValue: 20,
		Players: []Player{
			{ID: "p1", Chips: 300},
			{ID: "p2", Chips: 300},
			{ID: "p3", Chips: 300},
		},
		CurrentSB: "p1",
		CurrentBB: "p2",
	}

	table.DealCards(nil)
	table.SetSMBB()
	table.CurrentStage = StagePreFlop
	table.applyPlayerAction(0, Player{LastAction: "call", LastBet: 0})
	table.applyPlayerAction(1, Player{LastAction: "raise", LastBet: 20})
	table.applyPlayerAction(2, Player{LastAction: "fold"})

	table.CurrentStage = StageFlop
	if len(table.FlopCards) != 3 {
		t.Fatalf("expected 3 flop cards, got %d", len(table.FlopCards))
	}

	table.CurrentStage = StageTurn
	if table.TurnCard == nil {
		t.Fatalf("turn card should be available")
	}

	table.CurrentStage = StageRiver
	if table.RiverCard == nil {
		t.Fatalf("river card should be available")
	}

	table.EvaluateHand()
	table.AssignChipsToWinners()
	for i := range table.Players {
		if table.Players[i].ID != table.Winners[0].ID {
			table.Players[i].Chips = 0
			table.Players[i].HasFold = true
		}
	}
	table.SetEliminatePlayersWithNoChips()
	table.RemovePlayersEliminatedWithNoChips()
	table.TableEnd()
	if !table.TableEnds {
		t.Fatalf("table should conclude after river showdown")
	}
}

func TestSetSMBBDeductsChipsAndSetsFlags(t *testing.T) {
	table := &Table{
		BBValue: 40,
		Players: []Player{
			{ID: "sb", Chips: 100},
			{ID: "bb", Chips: 200},
		},
		CurrentSB: "sb",
		CurrentBB: "bb",
	}

	table.SetSMBB()

	if table.Players[0].Chips != 80 {
		t.Fatalf("small blind should have paid 20, remaining chips %d", table.Players[0].Chips)
	}
	if !table.Players[0].IsSB {
		t.Fatalf("player should be flagged as SB")
	}
	if table.Players[1].Chips != 160 {
		t.Fatalf("big blind should have paid 40, remaining %d", table.Players[1].Chips)
	}
	if !table.Players[1].IsBB {
		t.Fatalf("player should be flagged as BB")
	}
}

func TestSMBBTurnSkipsEliminatedPlayers(t *testing.T) {
	table := &Table{
		Players: []Player{
			{ID: "p1"},
			{ID: "p2", IsEliminated: true},
			{ID: "p3"},
			{ID: "p4"},
		},
		CurrentSB: "p1",
		CurrentBB: "p2",
	}

	table.SMBBTurn()

	if table.CurrentSB != "p3" {
		t.Fatalf("expected SB to move to next active player p3, got %s", table.CurrentSB)
	}
	if table.CurrentBB != "p4" {
		t.Fatalf("expected BB to move to p4, got %s", table.CurrentBB)
	}
}

func TestApplyPlayerActionAllInAndCallFlows(t *testing.T) {
	table := &Table{
		Players: []Player{
			{ID: "hero", Chips: 150},
			{ID: "villain", Chips: 150},
		},
		BBValue: 30,
	}

	table.applyPlayerAction(0, Player{LastAction: "call", LastBet: 30})
	if table.Players[0].Chips != 120 {
		t.Fatalf("call should deduct bet from chips")
	}

	table.applyPlayerAction(1, Player{LastAction: "allin"})
	if !table.Players[1].HasAllIn {
		t.Fatalf("player should be marked all-in")
	}
	if table.Players[1].Chips != 0 {
		t.Fatalf("all-in player should have zero chips")
	}
}

func TestManageSidePotsCreatesNestedPots(t *testing.T) {
	table := &Table{
		Players: []Player{
			{ID: "p1", TotalBet: 50, HasAllIn: true},
			{ID: "p2", TotalBet: 100},
			{ID: "p3", TotalBet: 100},
		},
	}

	table.ManageSidePots()

	assert.Len(t, table.SidePots, 2, "expected main side pot and residual pot")
	assert.Equal(t, 150, table.SidePots[0].Amount, "first pot should include capped contribution for all players")
	assert.Equal(t, 100, table.SidePots[1].Amount, "second pot holds remaining heads-up bets")

	for _, pot := range table.SidePots {
		assert.NotZero(t, len(pot.Players), "each side pot should track participating players")
	}
}

func TestAssignChipsToWinnersMovesStacks(t *testing.T) {
	table := &Table{
		Players: []Player{
			{ID: "winner", Chips: 0, TotalBet: 1000},
			{ID: "loser", Chips: 0, TotalBet: 1000},
		},
	}
	table.SidePots = []SidePot{
		{
			Amount:  2000,
			Players: []*Player{&table.Players[0], &table.Players[1]},
			Winner:  []*Player{&table.Players[0]},
		},
	}
	table.Winners = []Player{{ID: "winner"}}

	table.AssignChipsToWinners()
	table.SetEliminatePlayersWithNoChips()

	if got := table.Players[0].Chips; got != 2000 {
		t.Fatalf("winner stack should increase to 2000, got %d", got)
	}
	if got := table.Players[1].Chips; got != 0 {
		t.Fatalf("loser stack should remain at 0, got %d", got)
	}
	if !table.Players[1].IsEliminated {
		t.Fatalf("loser with zero chips must be marked eliminated")
	}
}

func TestCompareAndRemoveEliminatedPlayersRetainsUpdatedStacks(t *testing.T) {
	current := Table{
		ID:           "40",
		TournamentID: 1,
		Players: []Player{
			{ID: "p1", Chips: 3500},
			{ID: "p2", Chips: 500},
		},
	}
	original := Table{
		ID:           "40",
		TournamentID: 1,
		Players: []Player{
			{ID: "p1", Chips: 2500},
			{ID: "p2", Chips: 2500},
		},
	}

	updated := CompareAndRemoveEliminatedPlayers(current, original, nil)
	if len(updated.Players) != 2 {
		t.Fatalf("expected both players to remain, got %d", len(updated.Players))
	}
	if updated.Players[0].Chips != 3500 {
		t.Fatalf("player p1 chips should reflect updated state, got %d", updated.Players[0].Chips)
	}
	if updated.Players[1].Chips != 500 {
		t.Fatalf("player p2 chips should reflect updated state, got %d", updated.Players[1].Chips)
	}
}

func TestSetTablePlayersCallAmount(t *testing.T) {
	table := &Table{
		BiggestBet: 100,
		Players: []Player{
			{ID: "p1", TotalBet: 40},
			{ID: "p2", TotalBet: 100},
			{ID: "p3", TotalBet: 0, HasFold: true},
		},
	}

	table.SetTablePlayersCallAmount()

	assert.Equal(t, 60, table.Players[0].CallAmount, "player needing to call should match difference")
	assert.Equal(t, 0, table.Players[1].CallAmount, "player already at biggest bet should owe zero")
	assert.Equal(t, 0, table.Players[2].CallAmount, "folded players shouldn't be prompted to call")
}

func TestAllPlayersHaveCalledIgnoresFoldedAndAllIn(t *testing.T) {
	table := &Table{
		Players: []Player{
			{ID: "p1", CallAmount: 0},
			{ID: "p2", CallAmount: 0, HasAllIn: true},
			{ID: "p3", CallAmount: 10, HasFold: true},
		},
	}

	if !table.AllPlayersHaveCalled() {
		t.Fatalf("folded/all-in players should be excluded when checking if everyone has called")
	}
}

func TestSetTablePlayerActionsRespectsChipState(t *testing.T) {
	table := &Table{
		BBValue: 40,
		Players: []Player{{ID: "p1", Chips: 200, CallAmount: 40}},
	}

	table.SetTablePlayerActions(0)
	actions := table.Players[0].AvailableActions

	assert.Contains(t, actions, "call")
	assert.Contains(t, actions, "raise")
	assert.Contains(t, actions, "fold")
	assert.Contains(t, actions, "allin")
}

func TestAllPlayersExceptOneFoldDetectsWinner(t *testing.T) {
	table := &Table{
		Players: []Player{
			{ID: "active"},
			{ID: "folded", HasFold: true},
			{ID: "out", IsEliminated: true},
		},
	}

	table.AllPlayersExceptOneFold()

	if !table.AllFoldExceptOne {
		t.Fatalf("table should detect when only one active player remains")
	}
	if len(table.Winners) != 1 || table.Winners[0].ID != "active" {
		t.Fatalf("active player should be marked as winner when others fold")
	}
}

func TestClearPlayerActionsResetsRoundState(t *testing.T) {
	table := &Table{
		Players: []Player{
			{ID: "p1", CallAmount: 50, TotalBet: 100, LastAction: "raise", HasFold: true, HasAllIn: true, Cards: []Card{{Suit: Hearts, Value: Ace}}},
		},
	}

	table.ClearPlayerActions()

	player := table.Players[0]
	assert.Zero(t, player.CallAmount)
	assert.Zero(t, player.TotalBet)
	assert.Empty(t, player.LastAction)
	assert.False(t, player.HasFold)
	assert.False(t, player.HasAllIn)
	assert.Nil(t, player.Cards)
}

func TestNormalizeIncomingActionConvertsShortRaiseToAllIn(t *testing.T) {
	table := &Table{
		BBValue:    100,
		BiggestBet: 200,
		Players: []Player{
			{ID: "p1", Chips: 80, TotalBet: 20},
		},
	}
	player := &table.Players[0]
	action := Player{ID: "p1", LastAction: "raise", LastBet: 200}
	normalized := table.normalizeIncomingAction(player, action)

	if normalized.LastAction != "allin" {
		t.Fatalf("expected action to convert to allin, got %s", normalized.LastAction)
	}
	if normalized.LastBet != 80 {
		t.Fatalf("all-in bet should match available chips (80), got %d", normalized.LastBet)
	}
	if player.Chips != 80 {
		t.Fatalf("normalization should not mutate player chips, got %d", player.Chips)
	}
}

func TestRaiseAttemptWithOnlyCallAvailableEndsAsAllIn(t *testing.T) {
	table := &Table{
		BiggestBet: 2500,
		Players: []Player{
			{ID: "p1", Chips: 2450, TotalBet: 50, CallAmount: 2450},
		},
	}
	player := &table.Players[0]
	action := Player{ID: "p1", LastAction: "raise", LastBet: 2409}
	normalized := table.normalizeIncomingAction(player, action)
	if normalized.LastAction != "call" {
		t.Fatalf("expected normalization to downgrade action to call, got %s", normalized.LastAction)
	}
	table.applyPlayerAction(0, normalized)

	if !player.HasAllIn {
		t.Fatalf("player matching biggest bet with last chips should be flagged all-in")
	}
	if player.Chips != 0 {
		t.Fatalf("player should have zero chips after all-in call, got %d", player.Chips)
	}
	if player.LastAction != "allin" {
		t.Fatalf("player action should be captured as allin, got %s", player.LastAction)
	}
	if !table.AllPlayersHaveCalled() {
		t.Fatalf("table should consider all players settled after forced all-in call")
	}
}
