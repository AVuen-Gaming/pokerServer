package temporal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"server/config"
	"server/internal/poker"
	"time"

	"github.com/nats-io/nats.go"
)

func DealCardsActivity(ctx context.Context, table *poker.Table, config *config.Config) (*poker.Table, error) {
	log.Printf("Starting DealCardsActivity with table ID: %s", table.ID)
	table.DealCards()

	js := GetJetStream()
	for _, player := range table.Players {
		if err := poker.SendPlayerUpdateToNATS(js, table.ID, player); err != nil {
			log.Printf("Error sending player update to NATS for player ID %s: %v", player.ID, err)
			continue
		}
	}
	time.Sleep(2 * time.Second)

	log.Printf("Completed DealCardsActivity for table ID: %s", table.ID)
	return table, nil
}

func DealPreFlop(ctx context.Context, table *poker.Table, config *config.Config) (*poker.Table, error) {
	table.CurrentStage = "initRound"
	table.SMBBTurn()
	table.RemovePlayersEliminatedWithNoChips()
	if len(table.Players) < 2 {
		table.CurrentStage = "finishTable"
	}
	js := GetJetStream()
	err := poker.SendPTableUpdateToNATS(js, table)
	if err != nil {
		log.Fatalf("Failed to Send Data To Table: %v", err)
	}

	time.Sleep(2 * time.Second)

	return table, nil
}

func DealFlop(ctx context.Context, table *poker.Table, config *config.Config) (*poker.Table, error) {
	table.CurrentStage = "flop"
	time.Sleep(2 * time.Second)

	return table, nil
}

func DealTurn(ctx context.Context, table *poker.Table, config *config.Config) (*poker.Table, error) {
	table.CurrentStage = "turn"
	time.Sleep(2 * time.Second)

	return table, nil
}

func DealRiver(ctx context.Context, table *poker.Table, config *config.Config) (*poker.Table, error) {
	table.CurrentStage = "river"
	time.Sleep(2 * time.Second)

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

	if table.CurrentStage == "preFlop" {
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

		log.Printf("El turno es para el jugador %s", table.CurrentTurn)

		subject := fmt.Sprintf("pokerClient.tournament.%s.%s", table.ID, player.ID)
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
				table.Players[currentIndex].CallAmount -= action.LastBet
				table.TotalBet += action.LastBet
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
				table.Players[currentIndex].CallAmount -= action.LastBet
				table.TotalBet += action.LastBet
				table.Players[currentIndex].HasFold = false
				table.PlayerActedInRound++
			case "allin":
				table.Players[currentIndex].HasAllIn = true
				table.Players[currentIndex].TotalBet += table.Players[currentIndex].Chips
				table.TotalBet += table.Players[currentIndex].Chips
				table.PlayerActedInRound++
				if table.Players[currentIndex].Chips > table.Players[currentIndex].CallAmount {
					table.BiggestBet = table.Players[currentIndex].TotalBet
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
				table.Players[currentIndex].CallAmount -= table.Players[currentIndex].Chips
				table.Players[currentIndex].Chips = 0
			case "check":
				table.PlayerActedInRound++
			}
		case <-time.After(time.Duration(table.TurnTime) * time.Second):
			player.IsTurn = false
			log.Printf("El tiempo de turno para el jugador %s ha expirado", player.ID)
			player.LastAction = "fold"
			player.HasFold = true
			if player.CallAmount <= 0 {
				player.LastAction = "check"
				player.HasFold = false
			}
			table.PlayerActedInRound++
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		table.AllPlayersExceptOneFold()

		currentIndex = (currentIndex + 1) % len(table.Players)

		if table.AllPlayersAllInExceptFolded() {
			break
		}

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

		if table.AllFoldExceptOne || (table.PlayerActedInRound == len(table.Players)) {
			break
		}
	}

	// Limpiar el estado de IsTurn de todos los jugadores
	for i := range table.Players {
		table.Players[i].IsTurn = false
	}
	table.PlayerActedInRound = 0

	log.Printf("Los turnos de los jugadores se han completado para la mesa ID: %s", table.ID)
	return table, nil
}

func ShowDown(ctx context.Context, table *poker.Table, config *config.Config) (*poker.Table, error) {
	js := GetJetStream()
	table.EvaluateHand()
	table.AssignChipsToWinner(&table.Winners[0])

	table.CurrentStage = "ShowDown"

	err := poker.SendPTableUpdateToNATS(js, table)
	if err != nil {
		return nil, fmt.Errorf("Error enviando actualización a JetStream para el jugador: %v", err)
	}

	table.ClearPlayerActions()
	table.ClearTableActions()
	table.SetEliminatePlayersWithNoChips()

	log.Printf("El jugador %s ha ganado la mano con %s", table.Winners[0].ID, table.Winners[0].HandDescription)

	return table, nil
}

func ShowDownAllFoldExecptOne(ctx context.Context, table *poker.Table, config *config.Config) (*poker.Table, error) {
	js := GetJetStream()
	table.CurrentStage = "ShowDownAllFoldExceptOne"
	table.AssignChipsToWinner(&table.Winners[0])
	err := poker.SendPTableUpdateToNATS(js, table)
	if err != nil {
		return nil, fmt.Errorf("Error enviando actualización a JetStream para el jugador: %v", err)
	}
	table.ClearPlayerActions()
	table.ClearTableActions()
	table.SetEliminatePlayersWithNoChips()

	log.Printf("El jugador %s ha ganado la mano con %s", table.Winners[0].ID, table.Winners[0].HandDescription)

	return table, nil
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
