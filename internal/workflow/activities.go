package temporal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"server/config"
	"server/internal/db"
	"server/internal/db/models"
	"server/internal/poker"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
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

func HandleTableActivitie(ctx context.Context, table *poker.Table, config *config.Config) (*poker.Table, error) {
	SecTable := poker.Table{}
	table.Round++
	table.CurrentStage = StageInitRound
	table.SMBBTurn()
	table.RemovePlayersEliminatedWithNoChips()
	if len(table.Players) < 2 {
		table.CurrentStage = StageFinishTable
	}
	js := GetJetStream()
	err := poker.SendPTableUpdateToNATS(js, table)
	if err != nil {
		log.Fatalf("Failed to Send Data To Table: %v", err)
	}

	time.Sleep(2 * time.Second)
	//DEAL CARDS
	SecTable.Players = table.Players
	SecTable.ID = table.ID
	SecTable.TournamentID = table.TournamentID
	SecTable.DealCards(js)

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
		err := poker.SendPTableUpdateToNATS(js, table)
		if err != nil {
			return nil, fmt.Errorf("Error enviando actualización a JetStream para el jugador: %v", err)
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
		err := poker.SendPTableUpdateToNATS(js, table)
		if err != nil {
			return nil, fmt.Errorf("Error enviando actualización a JetStream para el jugador: %v", err)
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
		err := poker.SendPTableUpdateToNATS(js, table)
		if err != nil {
			return nil, fmt.Errorf("Error enviando actualización a JetStream para el jugador: %v", err)
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
		err := poker.SendPTableUpdateToNATS(js, table)
		if err != nil {
			return nil, fmt.Errorf("Error enviando actualización a JetStream para el jugador: %v", err)
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

	err = poker.SendPTableUpdateToNATS(js, table)
	if err != nil {
		return nil, fmt.Errorf("Error enviando actualización a JetStream para el jugador: %v", err)
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

func DealPreFlop(ctx context.Context, table *poker.Table, config *config.Config) (*poker.Table, error) {
	table.SMBBTurn()
	table.RemovePlayersEliminatedWithNoChips()
	if len(table.Players) < 2 {
		table.CurrentStage = StageFinishTable
	}
	js := GetJetStream()
	err := poker.SendPTableUpdateToNATS(js, table)
	if err != nil {
		log.Fatalf("Failed to Send Data To Table: %v", err)
	}
	return table, nil
}

func HandleTurns(ctx context.Context, table *poker.Table) (*poker.Table, error) {
	js := GetJetStream()
	table.LastToRaiserIndex = -1
	bbIndex := -1

	for i, player := range table.Players {
		if player.ID == table.CurrentBB {
			bbIndex = i
			break
		}
	}

	if bbIndex == -1 {
		return nil, fmt.Errorf("No se encontró el jugador con Big Blind en la mesa")
	}

	startingPlayerIndex := -1

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
		return table, nil
	}

	if startingPlayerIndex == -1 {
		return nil, fmt.Errorf("No se encontró un jugador válido para iniciar la ronda")
	}

	currentIndex := startingPlayerIndex
	var raiseOccurred bool

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
		table.EndTime = int(time.Now().Unix()) + table.TurnTime
		table.SetTablePlayerActions(currentIndex)

		err := poker.SendPTableUpdateToNATS(js, table)
		if err != nil {
			return nil, fmt.Errorf("Error enviando actualización a JetStream para el jugador: %v", err)
		}

		tournamentID := strconv.Itoa(table.TournamentID)
		subject := fmt.Sprintf("pokerClient.%s.%s.%s", tournamentID, table.ID, player.ID)
		consumerName := fmt.Sprintf("durable-consumer4-%s-%s", table.ID, player.ID)
		msgChan := make(chan *nats.Msg, 64)

		err = js.DeleteConsumer("POKER_TOURNAMENT", consumerName)
		if err != nil && !errors.Is(err, nats.ErrConsumerNotFound) {
			return nil, fmt.Errorf("Error eliminando el consumidor %s: %v", consumerName, err)
		}

		sub, err := js.ChanSubscribe(subject, msgChan, nats.Durable(consumerName), nats.AckExplicit(), nats.DeliverAll())
		if err != nil {
			return nil, fmt.Errorf("Error suscribiéndose a JetStream subject %s: %v", subject, err)
		}
		defer func() {
			if err := sub.Unsubscribe(); err != nil {
				log.Printf("Error desuscribiendo del subject %s: %v", subject, err)
			}
		}()

		select {
		case msg := <-msgChan:
			player.IsTurn = false
			var action poker.Player
			if err := json.Unmarshal(msg.Data, &action); err != nil {
				log.Printf("Error al deserializar mensaje: %v", err)
				continue
			}

			if err := msg.Ack(); err != nil {
				log.Printf("Error al marcar el mensaje como leído: %v", err)
				break
			}

			player.LastAction = action.LastAction

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
				//table.TotalBet += action.LastBet
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
				//table.TotalBet += action.LastBet
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
			table.Players[currentIndex].IsTurn = false
			table.Players[currentIndex].LastAction = "fold"
			table.Players[currentIndex].HasFold = true
			if table.Players[currentIndex].CallAmount <= 0 {
				table.Players[currentIndex].LastAction = "check"
				table.Players[currentIndex].HasFold = false
			}
			table.PlayerActedInRound++
		case <-ctx.Done():
			return nil, ctx.Err()
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

	return table, nil
}

func CheckLastTable(ctx context.Context, tables []poker.Table, updatedTable poker.Table, config *config.Config) (bool, error) {
	js := GetJetStream()
	poker.OnlyOneTableRemains(tables)
	if tables[0].LastTable {
		tables[0].TableEnd()
		if tables[0].TableEnds {
			for _, player := range tables[0].Players {
				walletID, err := db.GetWalletIDByPlayerID(player.ID)
				if err != nil {
					return true, fmt.Errorf("error consiguiendo el walletID del ganador: %v", err)
				}
				position, err := db.InsertRanking(tables[0].TournamentID, walletID)
				if err != nil {
					return true, fmt.Errorf("error InsertRanking: %v", err)
				}
				err = poker.HandlePlayerPrize(uint(tables[0].TournamentID), position.Position, player.ID)
				if err != nil {
					return true, fmt.Errorf("error HandlePlayerPrize: %v", err)
				}

			}
			tables[0].CurrentStage = StageFinishTournament
			err := poker.SendPTableUpdateToNATS(js, &tables[0])
			if err != nil {
				return true, fmt.Errorf("Error enviando actualización a JetStream para el jugador: %v", err)
			}
			return true, nil
		}
	}

	return false, nil
}

func CreatePrizePool(ctx context.Context, tournament *poker.Tournament, config *config.Config) (*poker.Tournament, error) {
	tourId := tournament.ID
	dbTournament, err := db.GetTournamentByID(tourId)
	if err != nil {
		return tournament, fmt.Errorf("error obteniendo el torneo con ID %d: %v", tourId, err)
	}

	if dbTournament.EntryCost == 0 {
		return tournament, fmt.Errorf("el torneo con ID %d no tiene un entry cost válido", tourId)
	}

	registrations, err := db.GetTournamentRegistrationsByTournamentID(tourId)
	if err != nil {
		return tournament, fmt.Errorf("error obteniendo registros del torneo con ID %d: %v", tourId, err)
	}

	numParticipants := len(registrations)
	if numParticipants == 0 {
		return tournament, fmt.Errorf("no hay registros para el torneo con ID %d", tourId)
	}

	totalPot := float32(numParticipants) * dbTournament.EntryCost
	prizePool := totalPot * 0.96

	prizeList := poker.GeneratePrizeList(numParticipants, prizePool)

	tournament.PrizeList = prizeList

	prizeListJSON, err := json.Marshal(prizeList)
	if err != nil {
		return tournament, fmt.Errorf("error serializando la lista de premios a JSON: %v", err)
	}

	prize := models.Prize{
		TournamentID: dbTournament.ID,
		TotalPot:     prizePool,
		PrizeList:    prizeListJSON,
	}

	if err := db.InsertPrize(&prize); err != nil {
		return tournament, fmt.Errorf("error al guardar el premio: %v", err)
	}

	log.Printf("Se creó el premio para el torneo con ID %d, TotalPot: %.2f", tourId, prizePool)
	return tournament, nil
}

func CreateTablesInTournament(ctx context.Context, tournament *poker.Tournament, config *config.Config) (*poker.Tournament, error) {
	var tournamentRegistrations []models.TournamentRegistration

	if tournament.ID == 0 {
		tournamentName, err := db.GetTournamentByName(tournament.Name)
		if err != nil {
			return tournament, errors.New("no se pudo obtener torneo")
		}
		tournamentRegistrations, err = db.GetTournamentRegistrationsByTournamentID(tournamentName.ID)
		if err != nil {
			return tournament, err
		}
	} else {
		tournamentRegistrations, _ = db.GetTournamentRegistrationsByTournamentID(tournament.ID)
	}

	if len(tournamentRegistrations) == 0 {
		return tournament, errors.New("no existen registros para este torneo")
	}

	var players []poker.Player
	for _, registration := range tournamentRegistrations {
		player := poker.Player{
			ID:    registration.Wallet.Wallet,
			Chips: tournament.StartChips,
		}
		players = append(players, player)
	}

	maxPlayersPerTable := 9
	totalPlayers := len(players)
	numTables := (totalPlayers + maxPlayersPerTable - 1) / maxPlayersPerTable
	err := db.CreateTables(tournament.ID, numTables)
	if err != nil {
		return tournament, err
	}

	var tables []poker.Table
	for i := 0; i < numTables; i++ {
		table := poker.Table{
			ID:                       strconv.Itoa(i + 1),
			IncrementBlind:           tournament.IncrementBlind,
			TotalPlayersInTournament: totalPlayers,
			MaxPlayers:               maxPlayersPerTable,
			MinPlayers:               2,
		}
		tables = append(tables, table)
	}

	for i, player := range players {
		tableIndex := i % numTables
		tables[tableIndex].Players = append(tables[tableIndex].Players, player)
		tables[tableIndex].BBValue = tournament.BBValue
		tables[tableIndex].TurnTime = tournament.TurnSeconds
		tables[tableIndex].TournamentID = int(tournament.ID)
		walletID, err := db.GetWalletIDByPlayerID(player.ID)
		if err != nil {
			return tournament, errors.New("failed to retrieve wallet ID for player: " + player.ID)
		}
		err = db.InsertTablePlayer(uint(tableIndex+1), walletID, tournament.ID)
		if err != nil {
			return tournament, errors.New("failed to insert into TablePlayer for player: " + player.ID)
		}
	}

	tournament.Players = players
	tournament.Tables = tables

	if len(tournamentRegistrations) == 1 {
		walletAddres := tables[0].Players[0].ID
		tournamentID := tables[0].TournamentID
		walletId, _ := db.GetWalletIDByPlayerID(walletAddres)
		position, _ := db.InsertRanking(tournamentID, walletId)
		err := poker.HandlePlayerPrize(uint(tournamentID), position.Position, walletAddres)
		if err != nil {
			log.Printf("Error manejando premios para el jugador %s en posición %d: %v", walletAddres, position, err)
		}
		return tournament, errors.New("no se cumplen el minimo de players para el torneo")
	}

	return tournament, nil
}

type MessageResult struct {
	Msg   *nats.Msg
	Valid bool
}

func getMessage(sub *nats.Subscription, timeout int) <-chan MessageResult {
	msgChan := make(chan MessageResult)
	go func() {
		msg, err := sub.NextMsg(time.Duration(timeout) * time.Second)
		if err != nil {
			msgChan <- MessageResult{Msg: nil, Valid: false}
		} else {
			msgChan <- MessageResult{Msg: msg, Valid: true}
		}
		close(msgChan)
	}()
	return msgChan
}

func countActivePlayers(players []poker.Player) int {
	count := 0
	for _, player := range players {
		if !player.HasFold && !player.HasAllIn && !player.IsEliminated {
			count++
		}
	}
	return count
}

func tableExists(tables []poker.Table, tableID string) bool {
	for _, table := range tables {
		if table.ID == tableID {
			return true
		}
	}
	return false
}

func updateTableFromUpdatedTable(originalTable poker.Table, updatedTable poker.Table) poker.Table {
	playerMap := make(map[string]*poker.Player)
	for i := range originalTable.Players {
		for j := range updatedTable.Players {
			if updatedTable.Players[j].ID == originalTable.Players[j].ID {
				originalTable.Players[j].Chips = updatedTable.Players[j].Chips
				break
			}
		}
		playerMap[originalTable.Players[i].ID] = &originalTable.Players[i]
	}

	for _, updatedPlayer := range updatedTable.Players {
		if player, exists := playerMap[updatedPlayer.ID]; exists {
			player.Chips = updatedPlayer.Chips
			// posiblemente se necesite agregar otros campos
		} else {
			originalTable.Players = append(originalTable.Players, updatedPlayer)
		}
	}

	for i := len(originalTable.Players) - 1; i >= 0; i-- {
		if _, exists := playerMap[originalTable.Players[i].ID]; !exists {
			originalTable.Players = append(originalTable.Players[:i], originalTable.Players[i+1:]...)
		}
	}
	//CONSTRUCTOR
	//mover a constructor
	//ver posibles campos extras necesarios
	originalTable.Round = updatedTable.Round
	originalTable.CurrentBB = updatedTable.CurrentBB
	originalTable.CurrentSB = updatedTable.CurrentSB
	originalTable.BBValue = updatedTable.BBValue
	originalTable.LastIncrementBlind = updatedTable.LastIncrementBlind

	return originalTable
}

func DeleteWorkflowExecutionActivity(ctx context.Context, workflowID string, cfg *config.Config) error {
	conn, err := grpc.NewClient(cfg.Temporal.HostPort, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	temporalClient := workflowservice.NewWorkflowServiceClient(conn)

	describeReq := &workflowservice.DescribeWorkflowExecutionRequest{
		Namespace: "default",
		Execution: &common.WorkflowExecution{
			WorkflowId: workflowID,
		},
	}

	_, err = temporalClient.DescribeWorkflowExecution(ctx, describeReq)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			log.Printf("Workflow %s not found, skipping deletion", workflowID)
			return nil
		}
		return err
	}

	deleteReq := &workflowservice.DeleteWorkflowExecutionRequest{
		Namespace: "default",
		WorkflowExecution: &common.WorkflowExecution{
			WorkflowId: workflowID,
		},
	}

	_, err = temporalClient.DeleteWorkflowExecution(ctx, deleteReq)
	if err != nil {
		return err
	}

	return nil
}
