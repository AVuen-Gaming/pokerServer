package poker

import (
	"encoding/json"
	"fmt"
	"math/rand"

	"github.com/nats-io/nats.go"
)

type Card struct {
	Suit  string
	Value string
}

type PlayerCards struct {
	PlayerID string
	Cards    []Card
}

type Table struct {
	ID                 string
	CurrentBB          string
	CurrentSM          string
	CurrentTurn        string
	NextTurn           string
	LastAction         string
	TotalBetIndividual map[string]int
	Total              int
	TotalBet           int
	CurrentStage       string // "pre-flop", "flop", "turn", "river", "dealing"
	TurnTime           int
	EndTime            int
	Timestamp          int64
	FlopCards          []Card
	TurnCard           *Card
	RiverCard          *Card
	Players            []Player
	BiggestBet         int
	IsPreFlop          bool
	Round              int
	PreviousStage      string
	BBValue            int
	AllFold            bool
}

const (
	Hearts   = "Hearts"
	Diamonds = "Diamonds"
	Clubs    = "Clubs"
	Spades   = "Spades"
)

var Suits = []string{Hearts, Diamonds, Clubs, Spades}

const (
	Two   = "2"
	Three = "3"
	Four  = "4"
	Five  = "5"
	Six   = "6"
	Seven = "7"
	Eight = "8"
	Nine  = "9"
	Ten   = "10"
	Jack  = "J"
	Queen = "Q"
	King  = "K"
	Ace   = "A"
)

var Values = []string{Two, Three, Four, Five, Six, Seven, Eight, Nine, Ten, Jack, Queen, King, Ace}

func (table *Table) DealCards() {
	deck := createDeck()
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })

	// Deal 2 cards to each player
	for i := range table.Players {
		player := &table.Players[i]
		if len(deck) >= 2 {
			player.Cards = []Card{deck[0], deck[1]}
			deck = deck[2:]
		} else {
			player.Cards = []Card{}
		}
	}

	// Deal 3 cards to the flop
	if len(deck) >= 3 {
		table.FlopCards = deck[:3]
		deck = deck[3:]
	} else {
		table.FlopCards = []Card{} // Handle case if not enough cards
	}

	// Deal 1 card to the turn
	if len(deck) > 0 {
		table.TurnCard = &deck[0]
		deck = deck[1:]
	} else {
		table.TurnCard = nil
	}

	// Deal 1 card to the river
	if len(deck) > 0 {
		table.RiverCard = &deck[0]
	} else {
		table.RiverCard = nil
	}
}

func createDeck() []Card {
	var deck []Card
	for _, suit := range Suits {
		for _, value := range Values {
			deck = append(deck, Card{Suit: suit, Value: value})
		}
	}
	return deck
}

func SendPTableUpdateToNATS(js nats.JetStreamContext, table *Table) error {
	subject := fmt.Sprintf("poker.tournament.%s", table.ID)

	messageBytes, err := json.Marshal(table)
	if err != nil {
		return fmt.Errorf("failed to marshal player data for player %s: %w", table.ID, err)
	}

	if _, err := js.Publish(subject, messageBytes); err != nil {
		return fmt.Errorf("failed to publish message to JetStream for player %s: %w", table.ID, err)
	}

	return nil
}

func (table *Table) SetTablePlayerActions(indexValue int) {
	player := &table.Players[indexValue]

	player.AvailableActions = []string{}

	if player.CallAmount <= 0 {
		player.AvailableActions = append(player.AvailableActions, "check")
	}

	if player.CallAmount > 0 && player.Chips >= player.CallAmount {
		player.AvailableActions = append(player.AvailableActions, "call")
	}

	if player.Chips > player.CallAmount+table.BBValue {
		player.AvailableActions = append(player.AvailableActions, "raise")
	}

	if player.Chips > 0 {
		player.AvailableActions = append(player.AvailableActions, "allin")
	}

	player.AvailableActions = append(player.AvailableActions, "fold")

	table.Players[indexValue].AvailableActions = player.AvailableActions
}

func (table *Table) SetTablePlayersCallAmount() {
	for i := range table.Players {
		player := &table.Players[i]
		if !player.HasFold && !player.HasAllIn && !player.IsEliminated {
			player.CallAmount = table.BiggestBet - player.TotalBet
		}
	}
}

func (table *Table) SetSMBB() {
	var smPlayer, bbPlayer *Player
	bbBet := table.BBValue
	smBet := table.BBValue / 2
	for i := range table.Players {
		player := &table.Players[i]
		if player.ID == table.CurrentSM {
			smPlayer = player
			table.Players[i].Chips -= smBet
			table.Players[i].TotalBet += smBet
			table.Players[i].LastAction = "SB"
			table.Players[i].IsSB = true
			table.TotalBet += smBet
		} else if player.ID == table.CurrentBB {
			bbPlayer = player
			table.Players[i].Chips -= bbBet
			table.Players[i].TotalBet += bbBet
			table.Players[i].LastAction = "BB"
			table.Players[i].IsBB = true
			table.BiggestBet = bbBet
			table.TotalBet += bbBet
		}
	}

	if smPlayer == nil || bbPlayer == nil {
		return
	}

	table.SetTablePlayersCallAmount()
}

func (table *Table) AllPlayersExceptOneFold() {
	activePlayers := 0

	for _, player := range table.Players {
		if !player.HasFold && !player.IsEliminated {
			activePlayers++
		}
	}

	if activePlayers > 1 {
		table.AllFold = false
	} else {
		table.AllFold = true
	}
}
