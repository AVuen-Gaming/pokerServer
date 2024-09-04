package poker

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"

	"github.com/alexclewontin/riverboat/eval"
	"github.com/nats-io/nats.go"
)

type Card struct {
	Suit  string `json:"suit"`
	Value string `json:"value"`
}

type PlayerCards struct {
	PlayerID string
	Cards    []Card
}

type Table struct {
	ID                 string
	CurrentBB          string
	CurrentSB          string
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
	Winners            []Player
	BiggestBet         int
	IsPreFlop          bool
	Round              int
	RoundFinish        bool
	PreviousStage      string
	BBValue            int
	AllFoldExceptOne   bool
	PlayerActedInRound int
	LastToRaiserIndex  int
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
	subject := fmt.Sprintf("pokerServer.tournament.%s", table.ID)

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
		if player.ID == table.CurrentSB {
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
	activePlayers := []Player{}

	for _, player := range table.Players {
		if !player.HasFold && !player.IsEliminated {
			activePlayers = append(activePlayers, player)
		}
	}

	if len(activePlayers) == 1 {
		table.Winners = activePlayers
		table.RoundFinish = true
		table.AllFoldExceptOne = true
		table.CurrentTurn = ""
	} else {
		table.AllFoldExceptOne = false
		table.RoundFinish = false
	}
}

func (table *Table) AllPlayersAllInExceptFolded() bool {
	for _, player := range table.Players {
		if player.HasFold || player.IsEliminated {
			continue
		}
		if !player.HasAllIn {
			return false
		}
	}
	return true
}

func (table *Table) AllPlayersAllInExceptOneAndFolded() bool {
	countNotAllIn := 0

	for _, player := range table.Players {
		if player.HasFold || player.IsEliminated {
			continue
		}
		if !player.HasAllIn {
			countNotAllIn++
		}
		if countNotAllIn > 1 {
			return false
		}
	}
	return countNotAllIn == 1
}

func (table *Table) ClearPlayerActions() {
	for i := range table.Players {
		table.Players[i].CallAmount = 0
		table.Players[i].TotalBet = 0
		table.Players[i].LastAction = ""
		table.Players[i].HasFold = false
		table.Players[i].HasAllIn = false
		table.Players[i].Cards = nil
	}
}

func (table *Table) ClearTableActions() {
	table.AllFoldExceptOne = false
	table.BiggestBet = 0
	table.PlayerActedInRound = 0
	table.Total = 0
	table.TotalBet = 0
	table.CurrentTurn = ""
	table.IsPreFlop = false
	table.LastToRaiserIndex = 0
	table.FlopCards = []Card{}
	table.TurnCard = nil
	table.RiverCard = nil
}

func (table *Table) CountActivePlayers() int {
	count := 0
	for _, player := range table.Players {
		if !player.HasFold && !player.HasAllIn && !player.IsEliminated {
			count++
		}
	}
	return count
}

func (table *Table) AllPlayersHaveCalled() bool {
	for _, player := range table.Players {
		// Ignorar jugadores que han hecho fold, están en all-in o están eliminados
		if player.HasFold || player.HasAllIn || player.IsEliminated {
			continue
		}
		// Si algún jugador aún tiene una cantidad pendiente para igualar, retornar false
		if player.CallAmount > 0 {
			return false
		}
	}
	return true
}

func (table *Table) CommunityCards() []Card {
	var communityCards []Card
	communityCards = append(communityCards, table.FlopCards...)
	if table.TurnCard != nil {
		communityCards = append(communityCards, *table.TurnCard)
	}
	if table.RiverCard != nil {
		communityCards = append(communityCards, *table.RiverCard)
	}
	return communityCards
}

func (table *Table) AssignChipsToWinner(winner *Player) {
	if winner == nil {
		return
	}

	for i := range table.Players {
		if table.Players[i].ID == winner.ID {
			table.Players[i].Chips += table.TotalBet
			table.TotalBet = 0
			return
		}
	}
}

func (table *Table) EvaluateHand() {
	suitMap := map[string]string{
		"Clubs":    "C",
		"Diamonds": "D",
		"Hearts":   "H",
		"Spades":   "S",
	}
	var winner Player
	bestHandScore := 99999
	table.CurrentStage = "showDown"

	for i, player := range table.Players {
		allCards := append(table.CommunityCards(), player.Cards...)
		riverboatCards := make([]eval.Card, len(allCards))
		if player.HasFold {
			table.Players[i].Cards = nil
			continue
		}
		for i, card := range allCards {
			// Convertir el nombre del palo a su abreviación
			suitAbbr, ok := suitMap[card.Suit]
			if !ok {
				panic(fmt.Sprintf("invalid suit: %v", card.Suit))
			}
			// Formatear el string de la carta
			cardStr := fmt.Sprintf("%v%v", card.Value, suitAbbr)
			riverboatCards[i] = eval.MustParseCardString(cardStr)
		}

		var bestFive []eval.Card
		switch len(riverboatCards) {
		case 5:
			bestFive = riverboatCards
		case 6:
			bestFive, _ = eval.BestFiveOfSix(riverboatCards[0], riverboatCards[1], riverboatCards[2], riverboatCards[3], riverboatCards[4], riverboatCards[5])
		case 7:
			bestFive, _ = eval.BestFiveOfSeven(riverboatCards[0], riverboatCards[1], riverboatCards[2], riverboatCards[3], riverboatCards[4], riverboatCards[5], riverboatCards[6])

		}

		sort.Slice(bestFive, func(i, j int) bool {
			rankI := int((bestFive[i] >> 8) & 0xF)
			rankJ := int((bestFive[j] >> 8) & 0xF)

			return rankI < rankJ
		})

		bestFiveOfSevenCards := bestFive[0:5]

		// Evaluate the hand
		handScore := eval.HandValue(bestFiveOfSevenCards[0], bestFiveOfSevenCards[1], bestFiveOfSevenCards[2], bestFiveOfSevenCards[3], bestFiveOfSevenCards[4])

		//handStringify := HandDescription(handScore)

		//winningHand := convertEvalCardsToCards(bestFiveOfSevenCards)

		for i := range table.Players {
			if table.Players[i].ID == player.ID {
				table.Players[i].HandScore = handScore
				if table.Players[i].HandScore < bestHandScore {
					table.Winners = nil //rework para los side pots en un futuro
					table.Winners = append(table.Winners, table.Players[i])
					bestHandScore = table.Players[i].HandScore
					winner = table.Players[i]
				}
			}
		}
	}
	fmt.Println("el ganador es", winner)
}

func HandDescription(handScore int) string {
	var handType string

	switch {
	case handScore > 6185:
		handType = "High Card"
	case handScore > 3325:
		handType = "One Pair"
	case handScore > 2467:
		handType = "Two Pairs"
	case handScore > 1609:
		handType = "Three of a Kind"
	case handScore > 1599:
		handType = "Straight"
	case handScore > 322:
		handType = "Flush"
	case handScore > 166:
		handType = "Full House"
	case handScore > 10:
		handType = "Four of a Kind"
	default:
		handType = "Straight Flush"
	}

	return handType
}

func convertCardToEvalCard(card Card) eval.Card {
	suits := map[string]int{"Clubs": 0, "Diamonds": 1, "Hearts": 2, "Spades": 3}
	ranks := map[string]int{"2": 0, "3": 1, "4": 2, "5": 3, "6": 4, "7": 5, "8": 6, "9": 7, "10": 8, "J": 9, "Q": 10, "K": 11, "A": 12}

	suit, suitExists := suits[card.Suit]
	rank, rankExists := ranks[card.Value]

	if !suitExists || !rankExists {
		panic(fmt.Sprintf("Carta inválida: %s de %s", card.Value, card.Suit))
	}

	// Crear la carta con el formato correcto para el evaluador
	evalCard := eval.Card((rank << 8) | suit)
	fmt.Printf("Carta convertida: %d (rango: %d, palo: %d)\n", evalCard, rank, suit)

	return evalCard
}

func (table *Table) AssignPlayerCardsFromSecTable(secTable *Table) {
	playerCardsMap := make(map[string][]Card)
	for _, player := range secTable.Players {
		if !player.HasFold {
			playerCardsMap[player.ID] = player.Cards
		}
	}

	for i := range table.Players {
		player := &table.Players[i]
		if cards, ok := playerCardsMap[player.ID]; ok {
			player.Cards = cards
		}
	}
}

func convertEvalCardToCard(evalCard eval.Card) Card {
	suits := map[string]string{"H": "h", "D": "d", "C": "c", "S": "s"}
	values := map[string]string{"2": "2", "3": "3", "4": "4", "5": "5", "6": "6", "7": "7", "8": "8", "9": "9", "T": "T", "J": "J", "Q": "Q", "K": "K", "A": "A"}

	cardStr := fmt.Sprintf("%v", evalCard)
	valueStr := cardStr[:len(cardStr)-1]
	suitStr := cardStr[len(cardStr)-1:]

	return Card{
		Suit:  suits[suitStr],
		Value: values[valueStr],
	}
}

func convertEvalCardsToCards(evalCards []eval.Card) []Card {
	cards := make([]Card, len(evalCards))

	for i, evalCard := range evalCards {
		cards[i] = convertEvalCardToCard(evalCard)
	}

	return cards
}

func (table *Table) SMBBTurn() {
	if table.CurrentSB == "" && table.CurrentBB == "" {
		if len(table.Players) >= 2 {
			table.CurrentSB = table.Players[0].ID
			table.CurrentBB = table.Players[1].ID
		}
		return
	}

	sbIndex := -1
	for i, player := range table.Players {
		if player.ID == table.CurrentSB {
			sbIndex = i
			break
		}
	}

	if sbIndex == -1 {
		sbIndex = 0
	}

	newSBIndex := (sbIndex + 1) % len(table.Players)
	newBBIndex := (newSBIndex + 1) % len(table.Players)

	table.CurrentSB = table.Players[newSBIndex].ID
	table.CurrentBB = table.Players[newBBIndex].ID
	return
}

func (table *Table) EliminateAndRemovePlayersWithNoChips() {
	var remainingPlayers []Player

	for _, player := range table.Players {
		if player.Chips <= 0 {
			player.IsEliminated = true
		} else {
			remainingPlayers = append(remainingPlayers, player)
		}
	}
	table.Players = remainingPlayers
}
