package poker

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"

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

type SidePot struct {
	Amount  int
	Players []*Player
	Winner  []*Player
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
	SidePots           []SidePot
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
			if table.Players[i].Chips <= smBet {
				smPlayer = player
				table.Players[i].LastAction = "SB"
				table.Players[i].IsSB = true
				table.Players[i].TotalBet += table.Players[i].Chips
				table.Players[i].HasAllIn = true
				if table.Players[i].Chips > table.BiggestBet {
					table.BiggestBet = smBet
				}
				table.Players[i].Chips = 0
			} else {
				smPlayer = player
				table.Players[i].Chips -= smBet
				table.Players[i].TotalBet += smBet
				table.Players[i].LastAction = "SB"
				table.Players[i].IsSB = true
				//table.TotalBet += smBet
			}
			table.ManageSidePots()
		} else if player.ID == table.CurrentBB {
			if table.Players[i].Chips <= bbBet {
				bbPlayer = player
				table.Players[i].LastAction = "BB"
				table.Players[i].IsBB = true
				table.Players[i].TotalBet += table.Players[i].Chips
				table.Players[i].HasAllIn = true
				if table.Players[i].Chips > table.BiggestBet {
					table.BiggestBet = bbBet
				}
				table.Players[i].Chips = 0
			} else {
				bbPlayer = player
				table.Players[i].Chips -= bbBet
				table.Players[i].TotalBet += bbBet
				table.Players[i].LastAction = "BB"
				table.Players[i].IsBB = true
				table.BiggestBet = bbBet
				//table.TotalBet += bbBet
			}
			table.ManageSidePots()
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

func (table *Table) FindPlayerByID(playerID string) int {
	for i := range table.Players {
		if table.Players[i].ID == playerID {
			return i
		}
	}
	return -1
}

func (table *Table) AssignChipsToWinners() {

	for i, sidePot := range table.SidePots {
		if len(sidePot.Winner) > 0 {
			if len(sidePot.Winner) == 1 {
				winner := sidePot.Winner[0]
				winnerIndex := table.FindPlayerByID(winner.ID)
				if winnerIndex != -1 {
					table.Players[winnerIndex].Chips += sidePot.Amount
				}
			} else {
				share := sidePot.Amount / len(sidePot.Winner)
				for _, winner := range sidePot.Winner {
					winnerIndex := table.FindPlayerByID(winner.ID)
					if winnerIndex != -1 {
						table.Players[winnerIndex].Chips += share
					}
				}
			}
			table.SidePots[i].Amount = 0
		}
	}

	if len(table.Winners) == 1 && table.TotalBet > 0 {
		winnerIndex := table.FindPlayerByID(table.Winners[0].ID)
		if winnerIndex != -1 {
			table.Players[winnerIndex].Chips += table.TotalBet
		}
		table.TotalBet = 0
	}

	table.SidePots = nil
}

func (table *Table) ManageSidePots() {
	table.SidePots = []SidePot{}

	playerBets := make(map[*Player]int)
	for i := range table.Players {
		player := &table.Players[i]
		playerBets[player] = player.TotalBet
	}

	for {
		minAllInBet := -1
		allInPlayerFound := false

		for player, bet := range playerBets {
			if player.HasAllIn && bet > 0 && (minAllInBet == -1 || bet < minAllInBet) {
				minAllInBet = bet
				allInPlayerFound = true
			}
		}

		if !allInPlayerFound {
			break
		}

		currentPot := SidePot{
			Amount:  0,
			Players: []*Player{},
		}

		for player, bet := range playerBets {
			if !player.HasFold && bet > 0 {
				if bet >= minAllInBet {
					currentPot.Amount += minAllInBet
					playerBets[player] -= minAllInBet
				} else {
					currentPot.Amount += bet
					playerBets[player] = 0
				}
				currentPot.Players = append(currentPot.Players, player)
			}
		}

		if len(currentPot.Players) > 0 {
			table.SidePots = append(table.SidePots, currentPot)
		}

		activeBets := 0
		for _, bet := range playerBets {
			activeBets += bet
		}

		if activeBets == 0 {
			break
		}
	}

	activePot := SidePot{
		Amount:  0,
		Players: []*Player{},
	}
	for player, bet := range playerBets {
		if bet > 0 {
			activePot.Amount += bet
			activePot.Players = append(activePot.Players, player)
		}
	}

	if activePot.Amount > 0 {
		table.SidePots = append(table.SidePots, activePot)
	}
}

func (table *Table) EvaluateHand() {
	suitMap := map[string]string{
		"Clubs":    "C",
		"Diamonds": "D",
		"Hearts":   "H",
		"Spades":   "S",
	}
	table.CurrentStage = "showDown"

	var totalBetWinners []*Player
	var bestTotalBetHandScore int = -1

	playersCopy := make([]Player, len(table.Players))
	copy(playersCopy, table.Players)

	for i := range playersCopy {
		player := &playersCopy[i]
		if player.HasFold || player.IsEliminated {
			continue
		}

		allCards := append(table.CommunityCards(), player.Cards...)
		riverboatCards := make([]eval.Card, len(allCards))

		for j, card := range allCards {
			suitAbbr := suitMap[card.Suit]
			cardStr := fmt.Sprintf("%v%v", card.Value, suitAbbr)
			riverboatCards[j] = eval.MustParseCardString(cardStr)
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

		handScore := eval.HandValue(bestFive[0], bestFive[1], bestFive[2], bestFive[3], bestFive[4])

		if handScore > bestTotalBetHandScore {
			totalBetWinners = []*Player{player}
			bestTotalBetHandScore = handScore
		} else if handScore == bestTotalBetHandScore {
			totalBetWinners = append(totalBetWinners, player)
		}

		player.HandScore = handScore
	}

	if len(totalBetWinners) > 0 {
		amountPerWinner := table.TotalBet / len(totalBetWinners)
		for _, player := range totalBetWinners {
			player.Winnings += amountPerWinner
		}
	} else {
		log.Println("Error: No hay ganadores en el totalBet.")
	}

	for i := range table.SidePots {
		sidePot := &table.SidePots[i]

		if len(sidePot.Players) == 1 {
			winner := sidePot.Players[0]
			table.SidePots[i].Winner = []*Player{winner}
			winner.Winnings += sidePot.Amount
			continue
		}

		var bestSidePotHandScore int = -1
		var sidePotWinner *Player

		for _, player := range sidePot.Players {
			if player.HasFold || player.IsEliminated {
				continue
			}

			allCards := append(table.CommunityCards(), player.Cards...)
			riverboatCards := make([]eval.Card, len(allCards))

			for j, card := range allCards {
				suitAbbr := suitMap[card.Suit]
				cardStr := fmt.Sprintf("%v%v", card.Value, suitAbbr)
				riverboatCards[j] = eval.MustParseCardString(cardStr)
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

			handScore := eval.HandValue(bestFive[0], bestFive[1], bestFive[2], bestFive[3], bestFive[4])

			if handScore > bestSidePotHandScore {
				bestSidePotHandScore = handScore
				sidePotWinner = player
			}
		}

		if sidePotWinner != nil {
			table.SidePots[i].Winner = []*Player{sidePotWinner}
			sidePotWinner.Winnings += sidePot.Amount
		} else {
			table.SidePots[i].Winner = nil
		}
	}

	table.Winners = make([]Player, len(totalBetWinners))
	for i, winner := range totalBetWinners {
		table.Winners[i] = *winner
	}

	if len(table.Winners) == 0 {
		log.Println("Error: No hay ganadores en la mesa.")
	}
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

func (table *Table) UpdateTotalBet() {
	total := table.TotalBet
	for _, sidePot := range table.SidePots {
		total += sidePot.Amount
	}

	table.Total = total
}

func (table *Table) UpdateTotalBetForFold() {
	total := table.TotalBet
	for _, sidePot := range table.SidePots {
		total += sidePot.Amount
		sidePot.Amount = 0
	}

	table.TotalBet = total
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
		activePlayers := []Player{}
		for _, player := range table.Players {
			if !player.IsEliminated {
				activePlayers = append(activePlayers, player)
			}
		}
		if len(activePlayers) >= 2 {
			table.CurrentSB = activePlayers[0].ID
			table.CurrentBB = activePlayers[1].ID
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

	newSBIndex := table.getNextActivePlayerIndex(sbIndex)
	newBBIndex := table.getNextActivePlayerIndex(newSBIndex)

	if newSBIndex != -1 {
		if newBBIndex == newSBIndex {
			newBBIndex = table.getNextActivePlayerIndex(newBBIndex)
		}
		table.CurrentSB = table.Players[newSBIndex].ID
		table.CurrentBB = table.Players[newBBIndex].ID
	}
}

func (table *Table) SetEliminatePlayersWithNoChips() {
	for i := range table.Players {
		if table.Players[i].Chips <= 0 {
			table.Players[i].IsEliminated = true
		}
	}
}

func (table *Table) RemovePlayersEliminatedWithNoChips() {
	var remainingPlayers []Player
	for _, player := range table.Players {
		if !player.IsEliminated {
			remainingPlayers = append(remainingPlayers, player)
		}
	}
	table.Players = remainingPlayers
}

func (table *Table) getNextActivePlayerIndex(startIndex int) int {
	for i := 1; i <= len(table.Players); i++ {
		currentIndex := (startIndex + i) % len(table.Players)
		if !table.Players[currentIndex].IsEliminated {
			return currentIndex
		}
	}
	return -1
}
