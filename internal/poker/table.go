package poker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"server/config"
	"server/internal/db"
	"strconv"
	"time"

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
	ID                       string
	CurrentBB                string
	CurrentSB                string
	CurrentTurn              string
	NextTurn                 string
	LastAction               string
	TotalBetIndividual       map[string]int
	Total                    int
	TotalBet                 int
	CurrentStage             string // "pre-flop", "flop", "turn", "river", "dealing"
	TurnTime                 int
	EndTime                  int
	Timestamp                int64
	FlopCards                []Card
	TurnCard                 *Card
	RiverCard                *Card
	Players                  []Player
	Winners                  []Player
	BiggestBet               int
	IsPreFlop                bool
	Round                    int
	RoundFinish              bool
	PreviousStage            string
	BBValue                  int
	AllFoldExceptOne         bool
	PlayerActedInRound       int
	SidePots                 []SidePot
	LastToRaiserIndex        int
	Stopped                  bool
	Frozen                   bool
	TableEnds                bool
	MinPlayers               int
	MaxPlayers               int
	AvgPlayers               int
	LastTable                bool
	EndTableTime             time.Time
	TournamentID             int
	IncrementBlind           int
	LastIncrementBlind       time.Time
	TotalPlayersInTournament int
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

const (
	StagePreFlop                  = "preFlop"
	StageInitRound                = "initRound"
	StageFinishTable              = "finishTable"
	StageFinishTournament         = "finishTorunament"
	StageSwitchingPlayer          = "switchingPlayer"
	StageFlop                     = "flop"
	StageTurn                     = "turn"
	StageRiver                    = "river"
	StageShowDown                 = "showDown"
	StageShowDownAllFoldExceptOne = "showDownAllFoldExceptOne"
)

var Values = []string{Two, Three, Four, Five, Six, Seven, Eight, Nine, Ten, Jack, Queen, King, Ace}

func (table *Table) DealCards(js nats.JetStreamContext) {
	deck := createDeck()
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })

	// Deal 2 cards to each player
	for i := range table.Players {
		player := table.Players[i]
		if len(deck) >= 2 {
			player.Cards = []Card{deck[0], deck[1]}
			deck = deck[2:]
			player.CurrentTable = table.ID
			if err := SendPlayerUpdateToNATS(js, table.ID, player, table.TournamentID); err != nil {
				continue
			}
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
	subject := fmt.Sprintf("pokerServer.%s.%s", strconv.Itoa(table.TournamentID), table.ID)

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
	//tournament, _ := db.GetTournamentByID(uint(table.TournamentID))
	if table.LastIncrementBlind.IsZero() {
		table.LastIncrementBlind = time.Now()
		//db.UpdateTournamentLastIncrementBlind(uint(table.TournamentID), table.LastIncrementBlind)
	} else {
		if time.Since(table.LastIncrementBlind) >= time.Duration(table.IncrementBlind)*time.Minute {
			table.BBValue *= 2
			table.LastIncrementBlind = time.Now()
			//db.UpdateTournamentLastIncrementBlind(uint(table.TournamentID), table.LastIncrementBlind)
		}
	}

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

func HandlePlayerPrize(tournamentID uint, position int, walletAddress string) error {
	prize, err := db.GetPrizeByTournamentID(tournamentID)
	if err != nil {
		return fmt.Errorf("error obteniendo premios para el torneo %d: %v", tournamentID, err)
	}

	var prizeList []struct {
		Position      int     `json:"position"`
		Prize         float64 `json:"prize"`
		Currency      string  `json:"currency"`
		WalletAddress string  `json:"wallet_address"`
	}

	if err := json.Unmarshal(prize.PrizeList, &prizeList); err != nil {
		return fmt.Errorf("error deserializando la lista de premios para el torneo %d: %v", tournamentID, err)
	}

	for idx, p := range prizeList {
		if p.Position == position {
			prizeList[idx].WalletAddress = walletAddress

			updatedPrizeList, err := json.Marshal(prizeList)
			if err != nil {
				return fmt.Errorf("error serializando la lista de premios actualizada: %v", err)
			}

			if err := db.UpdatePrizeList(tournamentID, updatedPrizeList); err != nil {
				return fmt.Errorf("error actualizando la lista de premios en la base de datos: %v", err)
			}

			return nil
		}
	}

	return nil
}

func (table *Table) RemovePlayersEliminatedWithNoChips() { //deprecado creo
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

func MovePlayers(tables []Table, currentTable Table, js nats.JetStreamContext) []Table {
	// Actualizar la current table usando CompareAndRemoveEliminatedPlayers.
	for i := 0; i < len(tables); i++ {
		if tables[i].ID == currentTable.ID {
			currentTable = CompareAndRemoveEliminatedPlayers(currentTable, tables[i], js)
			tables[i] = currentTable
			break
		}
	}

	const minPlayers = 2
	const maxPlayers = 9

	// Marcar mesas con menos de su mínimo como detenidas.
	for i := range tables {
		if len(tables[i].Players) < tables[i].MinPlayers {
			tables[i].Stopped = true
		}
	}

	// Calcular el promedio de jugadores.
	totalPlayers := 0
	for _, t := range tables {
		totalPlayers += len(t.Players)
	}
	avgPlayers := totalPlayers / len(tables)

	// Solo mover jugadores de la current table si ésta tiene menos que el promedio o su mínimo.
	if len(currentTable.Players) < avgPlayers || len(currentTable.Players) < currentTable.MinPlayers {
		for len(currentTable.Players) > 0 {
			// Evitar mover si eso dejaría a currentTable con menos de minPlayers.
			if len(currentTable.Players) <= minPlayers {
				break
			}
			player := currentTable.Players[0]
			currentTable.Players = currentTable.Players[1:]
			playerMoved := false
			originTableID := currentTable.ID
			for j := range tables {
				if currentTable.ID != tables[j].ID && len(tables[j].Players) < maxPlayers {
					// Solo mover si currentTable tendrá al menos minPlayers después de mover.
					if len(currentTable.Players) < minPlayers {
						break
					}
					tables[j].Players = append(tables[j].Players, player)
					player.SwitchingTable = true
					_ = SendPlayerUpdateToNATS(js, originTableID, player, tables[j].TournamentID)
					player.CurrentTable = tables[j].ID
					if walletId, err := db.GetWalletIDByPlayerID(player.ID); err == nil {
						if newTableID, err := strconv.Atoi(tables[j].ID); err == nil {
							db.UpdateTablePlayerTableID(walletId, tables[j].TournamentID, newTableID)
						}
					}
					playerMoved = true
					break
				}
			}
			if !playerMoved {
				// Si no se pudo mover el jugador, se reinserta y se sale del bucle.
				currentTable.Players = append([]Player{player}, currentTable.Players...)
				break
			}
		}
		// Si la current table queda vacía, marcarla como terminada y eliminarla.
		if len(currentTable.Players) == 0 {
			currentTable.TableEnds = true
			currentTable.CurrentStage = "deleteTable"
			if tableID, err := strconv.Atoi(currentTable.ID); err == nil {
				_ = db.DeleteTablePlayerByTableAndTournament(tableID, currentTable.TournamentID)
				_ = db.DeleteTableByTableAndTournament(tableID, currentTable.TournamentID)
			}
			SendPTableUpdateToNATS(js, &currentTable)
			tables = removeTable(tables, currentTable.ID)
		} else {
			// Actualizar la current table en el arreglo.
			for i := range tables {
				if tables[i].ID == currentTable.ID {
					tables[i] = currentTable
					break
				}
			}
		}
	}

	// Procesar mesas (que no sean la current) con exactamente 1 jugador:
	for i := range tables {
		if tables[i].ID == currentTable.ID {
			continue
		}
		if len(tables[i].Players) == 1 {
			// Comprobamos que la mesa tenga al menos 1 jugador antes de proceder.
			if len(tables[i].Players) == 0 {
				continue
			}
			bestCandidateIndex := -1
			bestCandidateCount := maxPlayers + 1
			for j := range tables {
				if tables[j].ID == tables[i].ID {
					continue
				}
				if len(tables[j].Players) < tables[j].MaxPlayers {
					count := len(tables[j].Players)
					if count >= tables[j].MinPlayers && count < bestCandidateCount {
						bestCandidateIndex = j
						bestCandidateCount = count
					}
				}
			}
			// Si no se encontró candidato óptimo, buscar cualquier mesa con espacio.
			if bestCandidateIndex == -1 {
				for j := range tables {
					if tables[j].ID == tables[i].ID {
						continue
					}
					if len(tables[j].Players) < tables[j].MaxPlayers {
						bestCandidateIndex = j
						break
					}
				}
			}
			if bestCandidateIndex != -1 && len(tables[i].Players) > 0 {
				player := tables[i].Players[0]
				// Removemos el jugador de la mesa que tiene 1 jugador.
				tables[i].Players = tables[i].Players[1:]
				// Asignamos al jugador a la mesa candidata.
				tables[bestCandidateIndex].Players = append(tables[bestCandidateIndex].Players, player)
				player.SwitchingTable = true
				oldTableID := tables[i].ID
				player.CurrentTable = tables[bestCandidateIndex].ID
				_ = SendPlayerUpdateToNATS(js, oldTableID, player, tables[bestCandidateIndex].TournamentID)
			}
			// Si la mesa aún tiene 1 jugador, se marca como Frozen.
			if len(tables[i].Players) == 1 {
				tables[i].Frozen = true
				_ = SendPTableUpdateToNATS(js, &tables[i])
			} else {
				tables[i].Frozen = false
			}
		}
	}

	return tables
}

func GetTableByID(tables []Table, targetTableID string) (*Table, bool) {
	for i := range tables {
		if tables[i].ID == targetTableID {
			return &tables[i], true
		}
	}
	return nil, false
}

func removeTable(tables []Table, tableID string) []Table {
	for i := 0; i < len(tables); i++ {
		if tables[i].ID == tableID {
			return append(tables[:i], tables[i+1:]...) // Retorna el arreglo sin la mesa eliminada
		}
	}
	return tables
}

func OnlyOneTableRemains(tables []Table) {
	if len(tables) > 0 {
		tables[0].LastTable = false
	}
	if len(tables) == 1 {
		tables[0].LastTable = true
	}
}

func (table *Table) TableEnd() {
	if len(table.Players) <= 1 {
		table.EndTableTime = time.Now()
		table.TableEnds = true
	} else {
		table.TableEnds = false
	}
}

func CompareAndRemoveEliminatedPlayers(currentTable, originalTable Table, js nats.JetStreamContext) Table {
	currentTable.SetEliminatePlayersWithNoChips()
	originalPlayerMap := make(map[string]Player)
	for _, player := range currentTable.Players {
		originalPlayerMap[player.ID] = player
	}

	var updatedPlayers []Player
	for _, player := range originalTable.Players {
		if originalPlayer, exists := originalPlayerMap[player.ID]; exists {
			if !originalPlayer.IsEliminated {
				updatedPlayers = append(updatedPlayers, player)
			}
			if originalPlayer.IsEliminated {
				walletId, _ := db.GetWalletIDByPlayerID(player.ID)
				position, _ := db.InsertRanking(originalTable.TournamentID, walletId)
				player.Position = position.Position
				err := HandlePlayerPrize(uint(originalTable.TournamentID), position.Position, player.ID)
				if err != nil {
					log.Printf("Error manejando premios para el jugador %s en posición %d: %v", player.ID, position, err)
				}
				tableId, _ := strconv.Atoi(currentTable.ID)
				db.DeleteTablePlayerByWalletAddress(tableId, originalTable.TournamentID, player.ID)
				db.UpdateTournamentRegistrationEliminated(originalTable.TournamentID, walletId)
				err = SendPlayerUpdateToNATS(js, originalTable.ID, player, originalTable.TournamentID)
				if err != nil {
					log.Printf("Error enviando actualización a NATS para el jugador %s: %v", player.ID, err)
				}
			}
		}
	}
	currentTable.Players = updatedPlayers
	return currentTable
}

func (table *Table) HandleTurn(ctx context.Context, js nats.JetStreamContext) error {
	// Inicializar
	table.LastToRaiserIndex = -1
	bbIndex := -1
	for i, player := range table.Players {
		if player.ID == table.CurrentBB {
			bbIndex = i
			break
		}
	}
	if bbIndex == -1 {
		return fmt.Errorf("No se encontró el jugador con Big Blind en la mesa")
	}

	var startingPlayerIndex int = -1
	if table.CurrentStage == StagePreFlop {
		table.SetSMBB()
		startingPlayerIndex = (bbIndex + 1) % len(table.Players)
	} else {
		for i := 1; i < len(table.Players); i++ {
			currentIndex := (bbIndex + i) % len(table.Players)
			if !table.Players[currentIndex].HasFold && !table.Players[currentIndex].HasAllIn && !table.Players[currentIndex].IsEliminated {
				startingPlayerIndex = currentIndex
				break
			}
		}
	}
	if table.AllPlayersAllInExceptOneAndFolded() || table.AllPlayersAllInExceptFolded() {
		return nil
	}
	if startingPlayerIndex == -1 {
		return fmt.Errorf("No se encontró un jugador válido para iniciar la ronda")
	}

	currentIndex := startingPlayerIndex
	var raiseOccurred bool
	// Usamos time.Now() aquí (podrías cambiarlo a workflow.Now(ctx) en un flujo de Temporal)
	now := time.Now()
	table.EndTime = int(now.Unix()) + table.TurnTime

	for {
		player := &table.Players[currentIndex]
		if player.HasFold || player.HasAllIn || player.IsEliminated {
			currentIndex = (currentIndex + 1) % len(table.Players)
			if currentIndex == startingPlayerIndex && !raiseOccurred && table.PlayerActedInRound >= table.CountActivePlayers() {
				break
			}
			continue
		}

		table.CurrentTurn = player.ID
		now = time.Now()
		table.EndTime = int(now.Unix()) + table.TurnTime
		table.SetTablePlayerActions(currentIndex)
		if player.IsAFK {
			table.EndTime = int(now.Unix()) + table.TurnTime/3
		}

		if err := SendPTableUpdateToNATS(js, table); err != nil {
			return fmt.Errorf("Error enviando actualización a JetStream: %v", err)
		}

		tournamentID := strconv.Itoa(table.TournamentID)
		subject := fmt.Sprintf("pokerClient.%s.%s.%s", tournamentID, table.ID, player.ID)
		consumerName := fmt.Sprintf("durable-consumer4-%s-%s", table.ID, player.ID)
		msgChan := make(chan *nats.Msg, 64)

		// Opción: Usar DeliverLast() para asegurarse de recibir el último mensaje y que se limpie la cola.
		// También se elimina el consumidor previo para evitar errores.
		if err := js.DeleteConsumer("POKER_TOURNAMENT", consumerName); err != nil && !errors.Is(err, nats.ErrConsumerNotFound) {
			return fmt.Errorf("Error eliminando el consumidor %s: %v", consumerName, err)
		}

		sub, err := js.ChanSubscribe(subject, msgChan,
			nats.Durable(consumerName),
			nats.AckExplicit(),
			nats.DeliverLast(),
		)
		if err != nil {
			return fmt.Errorf("Error suscribiéndose a JetStream subject %s: %v", subject, err)
		}
		// Se desuscribe al finalizar el select.
		defer func() {
			if err := sub.Unsubscribe(); err != nil {
				log.Printf("Error desuscribiendo del subject %s: %v", subject, err)
			}
		}()

		select {
		case msg := <-msgChan:
			player.IsTurn = false
			var action Player
			if err := json.Unmarshal(msg.Data, &action); err != nil {
				log.Printf("Error al deserializar mensaje: %v", err)
				continue
			}
			if err := msg.Ack(); err != nil {
				log.Printf("Error al marcar el mensaje como leído: %v", err)
				break
			}
			player.LastAction = action.LastAction
			player.IsAFK = false

			switch action.LastAction {
			case "raise":
				raiseOccurred = true
				table.LastToRaiserIndex = currentIndex
				startingPlayerIndex = currentIndex
				table.Players[currentIndex].TotalBet += action.LastBet
				table.Players[currentIndex].Chips -= action.LastBet
				table.BiggestBet = table.Players[currentIndex].TotalBet
				table.ManageSidePots()
				table.Players[currentIndex].CallAmount -= action.LastBet
				table.SetTablePlayersCallAmount()
				table.PlayerActedInRound = 1
				for i := range table.Players {
					if table.Players[i].ID != player.ID && !table.Players[i].HasFold && !table.Players[i].HasAllIn {
						table.Players[i].LastAction = ""
					}
				}
			case "fold":
				table.PlayerActedInRound++
				player.HasFold = true
			case "call":
				table.Players[currentIndex].TotalBet += action.LastBet
				table.Players[currentIndex].Chips -= action.LastBet
				table.Players[currentIndex].HasFold = false
				table.ManageSidePots()
				table.Players[currentIndex].CallAmount -= action.LastBet
				table.PlayerActedInRound++
			case "allin":
				table.Players[currentIndex].HasAllIn = true
				table.Players[currentIndex].TotalBet += table.Players[currentIndex].Chips
				biggestBet := table.Players[currentIndex].TotalBet
				table.Players[currentIndex].Chips = 0
				if table.Players[currentIndex].TotalBet > table.Players[currentIndex].CallAmount {
					table.BiggestBet = biggestBet
					raiseOccurred = true
					table.LastToRaiserIndex = currentIndex
					startingPlayerIndex = currentIndex
					table.PlayerActedInRound = 1
					for i := range table.Players {
						if table.Players[i].ID != player.ID && !table.Players[i].HasFold && !table.Players[i].HasAllIn {
							table.Players[i].LastAction = ""
						}
					}
					table.SetTablePlayersCallAmount()
				}
				table.ManageSidePots()
				table.Players[currentIndex].CallAmount -= table.Players[currentIndex].TotalBet
			case "check":
				table.PlayerActedInRound++
			}
		case <-time.After(time.Duration(table.TurnTime) * time.Second):
			player.IsTurn = false
			player.LastAction = "fold"
			player.HasFold = true
			player.IsAFK = true
			if player.CallAmount <= 0 {
				player.LastAction = "check"
				player.HasFold = false
			}
			table.PlayerActedInRound++
		case <-ctx.Done():
			return ctx.Err()
		}

		table.UpdateTotalBet()
		table.AllPlayersExceptOneFold()
		currentIndex = (currentIndex + 1) % len(table.Players)
		if table.AllFoldExceptOne {
			break
		}
		if currentIndex == table.LastToRaiserIndex && !raiseOccurred {
			break
		}
		if currentIndex == startingPlayerIndex && !raiseOccurred {
			break
		}
		if raiseOccurred {
			table.PlayerActedInRound = 1
			raiseOccurred = false
		}
		if table.PlayerActedInRound == len(table.Players) {
			break
		}
	}

	for i := range table.Players {
		table.Players[i].IsTurn = false
	}
	table.PlayerActedInRound = 0

	return nil
}

func CheckLastTableInTables(tables []Table, js nats.JetStreamContext) (bool, error) {
	OnlyOneTableRemains(tables)
	if tables[0].LastTable {
		tables[0].TableEnd()
		if tables[0].TableEnds {
			for _, player := range tables[0].Players {
				walletID, err := db.GetWalletIDByPlayerID(player.ID)
				if err != nil {
					return true, fmt.Errorf("error consiguiendo el walletID del ganador: %v", err)
				}
				tableID, err := strconv.Atoi(tables[0].ID)
				if err != nil {
					return true, fmt.Errorf("error consiguiendo el tableID del ganador: %v", err)
				}
				db.DeleteTablePlayerByWalletID(tableID, tables[0].TournamentID, walletID)
				db.DeleteTableByTableAndTournament(tableID, tables[0].TournamentID)
				position, err := db.InsertRanking(tables[0].TournamentID, walletID)
				if err != nil {
					return true, fmt.Errorf("error InsertRanking: %v", err)
				}
				err = HandlePlayerPrize(uint(tables[0].TournamentID), position.Position, player.ID)
				if err != nil {
					return true, fmt.Errorf("error HandlePlayerPrize: %v", err)
				}

			}
			tables[0].CurrentStage = StageFinishTournament
			err := SendPTableUpdateToNATS(js, &tables[0])
			if err != nil {
				return true, fmt.Errorf("Error enviando actualización a JetStream para el jugador: %v", err)
			}
			return true, nil
		}
	}

	return false, nil
}

func Reshuffle(tables []Table, updatedTable Table, js nats.JetStreamContext) []Table {

	tables = MovePlayers(tables, updatedTable, js)

	return tables
}

func HandleTable(ctx context.Context, table Table, config *config.Config, js nats.JetStreamContext) (Table, error) { //to delete
	SecTable := Table{}
	table.Round++
	table.CurrentStage = StageInitRound
	table.SMBBTurn()
	table.RemovePlayersEliminatedWithNoChips()
	if len(table.Players) < 2 {
		table.CurrentStage = StageFinishTable
	}
	err := SendPTableUpdateToNATS(js, &table)
	if err != nil {
		log.Fatalf("Failed to Send Data To Table: %v", err)
	}

	time.Sleep(2 * time.Second)
	//DEAL CARDS
	SecTable.DealCards(js)

	for _, player := range table.Players {
		player.CurrentTable = table.ID
		if err := SendPlayerUpdateToNATS(js, table.ID, player, table.TournamentID); err != nil {
			log.Printf("Error sending player update to NATS for player ID %s: %v", player.ID, err)
			continue
		}
	}
	time.Sleep(2 * time.Second)
	table.CurrentStage = StagePreFlop
	//HANDLETURNS

	table.HandleTurn(ctx, js)

	time.Sleep(2 * time.Second)

	//CHECKSHOWDOWN
	if table.AllFoldExceptOne {
		table.CurrentStage = StageShowDownAllFoldExceptOne
		table.UpdateTotalBetForFold()
		table.AssignChipsToWinners()
		err := SendPTableUpdateToNATS(js, &table)
		if err != nil {
			return table, fmt.Errorf("Error enviando actualización a JetStream para el jugador: %v", err)
		}
		table.ClearPlayerActions()
		table.ClearTableActions()
		table.SetEliminatePlayersWithNoChips()

		if len(table.Winners) > 0 {
			log.Printf("El jugador %s ha ganado la mano con %s", table.Winners[0].ID, table.Winners[0].HandDescription)
		} else {
			log.Printf("No se pudo determinar un ganador en EvaluateHand")
		}

		return table, nil
	}

	//FLOP
	table.FlopCards = SecTable.FlopCards
	table.CurrentStage = StageFlop
	time.Sleep(2 * time.Second)

	//HANDLETURN
	table.HandleTurn(ctx, js)
	time.Sleep(2 * time.Second)

	//CHECKSHOWDOWN
	if table.AllFoldExceptOne {
		table.CurrentStage = StageShowDownAllFoldExceptOne
		table.UpdateTotalBetForFold()
		table.AssignChipsToWinners()
		err := SendPTableUpdateToNATS(js, &table)
		if err != nil {
			return table, fmt.Errorf("Error enviando actualización a JetStream para el jugador: %v", err)
		}
		table.ClearPlayerActions()
		table.ClearTableActions()
		table.SetEliminatePlayersWithNoChips()

		if len(table.Winners) > 0 {
			log.Printf("El jugador %s ha ganado la mano con %s", table.Winners[0].ID, table.Winners[0].HandDescription)
		} else {
			log.Printf("No se pudo determinar un ganador en EvaluateHand")
		}

		return table, nil
	}

	//TURN
	table.TurnCard = SecTable.TurnCard
	table.CurrentStage = StageTurn
	time.Sleep(2 * time.Second)

	//HANDLETURN
	table.HandleTurn(ctx, js)
	time.Sleep(2 * time.Second)

	//CHECKSHOWDOWN
	if table.AllFoldExceptOne {
		table.CurrentStage = StageShowDownAllFoldExceptOne
		table.UpdateTotalBetForFold()
		table.AssignChipsToWinners()
		err := SendPTableUpdateToNATS(js, &table)
		if err != nil {
			return table, fmt.Errorf("Error enviando actualización a JetStream para el jugador: %v", err)
		}
		table.ClearPlayerActions()
		table.ClearTableActions()
		table.SetEliminatePlayersWithNoChips()

		if len(table.Winners) > 0 {
			log.Printf("El jugador %s ha ganado la mano con %s", table.Winners[0].ID, table.Winners[0].HandDescription)
		} else {
			log.Printf("No se pudo determinar un ganador en EvaluateHand")
		}

		return table, nil
	}

	//RIVER
	table.RiverCard = SecTable.RiverCard
	table.CurrentStage = StageRiver
	time.Sleep(2 * time.Second)

	//HANDLETURN
	table.HandleTurn(ctx, js)
	time.Sleep(2 * time.Second)

	//CHECKSHOWDOWN
	if table.AllFoldExceptOne {
		table.CurrentStage = StageShowDownAllFoldExceptOne
		table.UpdateTotalBetForFold()
		table.AssignChipsToWinners()
		err := SendPTableUpdateToNATS(js, &table)
		if err != nil {
			return table, fmt.Errorf("Error enviando actualización a JetStream para el jugador: %v", err)
		}
		table.ClearPlayerActions()
		table.ClearTableActions()
		table.SetEliminatePlayersWithNoChips()

		if len(table.Winners) > 0 {
			log.Printf("El jugador %s ha ganado la mano con %s", table.Winners[0].ID, table.Winners[0].HandDescription)
		} else {
			log.Printf("No se pudo determinar un ganador en EvaluateHand")
		}

		return table, nil
	}
	//SHOWDOWN
	table.AssignPlayerCardsFromSecTable(&SecTable)
	table.EvaluateHand()
	table.AssignChipsToWinners()

	table.CurrentStage = StageShowDown

	err = SendPTableUpdateToNATS(js, &table)
	if err != nil {
		return table, fmt.Errorf("Error enviando actualización a JetStream para el jugador: %v", err)
	}

	table.ClearPlayerActions()
	table.ClearTableActions()
	table.SetEliminatePlayersWithNoChips()

	if len(table.Winners) > 0 {
		log.Printf("El jugador %s ha ganado la mano con %s", table.Winners[0].ID, table.Winners[0].HandDescription)
	} else {
		log.Printf("No se pudo determinar un ganador en EvaluateHand")
	}

	return table, nil
}
