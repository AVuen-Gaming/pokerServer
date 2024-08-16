package temporal

import (
	"context"
	"encoding/json"
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
	bbIndex := -1
	for i, player := range table.Players {
		if player.ID == table.CurrentBB {
			bbIndex = i
			break
		}
	}

	if table.CurrentStage == "preFlop" {
		table.SetSMBB()
	}

	if bbIndex == -1 {
		return nil, fmt.Errorf("No se encontró el jugador con Big Blind en la mesa")
	}

	// Encontrar el próximo jugador válido (que no haya foldeado ni ido all-in)
	startingPlayerIndex := -1
	for i := 1; i < len(table.Players); i++ {
		currentIndex := (bbIndex + i) % len(table.Players)
		player := table.Players[currentIndex]
		if !player.HasFold && !player.HasAllIn && !player.IsEliminated {
			startingPlayerIndex = currentIndex
			break
		}
	}

	if startingPlayerIndex == -1 {
		return nil, fmt.Errorf("No se encontró un jugador válido para iniciar la ronda")
	}

	for {
		raiseOccurred := false
		currentIndex := startingPlayerIndex
		for {
			player := &table.Players[currentIndex]
			if player.LastAction == "raise" && !player.IsBB {
				break
			}
			if player.HasFold || player.HasAllIn || player.IsEliminated {
				currentIndex = (currentIndex + 1) % len(table.Players)
				if currentIndex == startingPlayerIndex { //agregar que si el monto total de apuesta es igual a la BB
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

			subject := fmt.Sprintf("pokerSend.tournament.%s.%s", table.ID, player.ID)

			// Genera un identificador único para cada consumidor duradero
			consumerName := fmt.Sprintf("durable-consumer4-%s-%s", table.ID, player.ID)

			// Canal para recibir mensajes
			msgChan := make(chan *nats.Msg, 64)

			// Suscripción asíncrona
			sub, err := js.ChanSubscribe(subject, msgChan, nats.Durable(consumerName), nats.AckExplicit(), nats.DeliverAll())
			if err != nil {
				return nil, fmt.Errorf("Error suscribiéndose a JetStream subject %s: %v", subject, err)
			}

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
				msg.AckSync()
				msg.Ack()

				player.LastAction = action.LastAction

				switch action.LastAction {
				case "raise":
					raiseOccurred = true
					player.TotalBet += action.LastBet
					player.Chips = player.Chips - action.LastBet
					table.BiggestBet = player.TotalBet
					player.LastAction = "raise"
					player.LastBet = 0 //TODO: Delete?
					for i := range table.Players {
						if table.Players[i].ID != player.ID && !table.Players[i].HasFold && !table.Players[i].HasAllIn {
							table.Players[i].LastAction = ""
						}
					}
					player.CallAmount = player.CallAmount - action.LastBet
					table.SetTablePlayersCallAmount()
				case "fold":
					player.HasFold = true
					player.LastAction = "fold"
				case "call":
					player.TotalBet += action.LastBet
					player.LastAction = "call"
					player.Chips = player.Chips - action.LastBet
					player.CallAmount = player.CallAmount - action.LastBet
				case "allin":
					player.HasAllIn = true
					player.LastAction = "allin"
					player.CallAmount = player.CallAmount - action.LastBet
				case "check":
					player.LastAction = "check"
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
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			defer sub.Unsubscribe()

			table.Players[currentIndex] = *player

			currentIndex = (currentIndex + 1) % len(table.Players)
			if currentIndex == startingPlayerIndex {
				break
			}
			table.AllPlayersExceptOneFold()
			if table.AllFold {
				break
			}
		}

		if table.AllFold {
			break
		}

		if raiseOccurred && countActivePlayers(table.Players) > 1 {
			log.Printf("Un raise ocurrió, reiniciando la ronda de turnos.")
			continue
		}
		break

	}

	for i := range table.Players {
		table.Players[i].IsTurn = false
	}

	log.Printf("Los turnos de los jugadores se han completado para la mesa ID: %s", table.ID)
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
