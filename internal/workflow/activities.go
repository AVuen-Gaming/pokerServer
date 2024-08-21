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

	// Encontrar el índice del jugador con Big Blind
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

	// Determinar el índice del primer jugador en actuar (preflop o postflop)
	startingPlayerIndex := -1
	if table.CurrentStage == "preFlop" {
		startingPlayerIndex = (bbIndex + 1) % len(table.Players) // Jugador a la izquierda del BB
	} else {
		// Postflop y rondas posteriores: Primer jugador activo a la izquierda del BB
		for i := 1; i <= len(table.Players); i++ {
			currentIndex := (bbIndex + i) % len(table.Players)
			player := table.Players[currentIndex]
			if !player.HasFold && !player.HasAllIn && !player.IsEliminated {
				startingPlayerIndex = currentIndex
				break
			}
		}
	}

	if startingPlayerIndex == -1 {
		return nil, fmt.Errorf("No se encontró un jugador válido para iniciar la ronda")
	}

	currentIndex := startingPlayerIndex
	playersActed := 0
	lastRaiserIndex := -1
	var raiseOccurred bool

	for {
		player := &table.Players[currentIndex]

		// Saltar jugadores que han foldeado, están all-in o eliminados
		if player.HasFold || player.HasAllIn || player.IsEliminated {
			currentIndex = (currentIndex + 1) % len(table.Players)
			continue
		}

		// Establecer el turno del jugador actual
		table.CurrentTurn = player.ID
		table.EndTime = int(time.Now().Unix()) + table.TurnTime
		table.SetTablePlayerActions(currentIndex)

		err := poker.SendPTableUpdateToNATS(js, table)
		if err != nil {
			return nil, fmt.Errorf("Error enviando actualización a JetStream para el jugador: %v", err)
		}

		log.Printf("El turno es para el jugador %s", table.CurrentTurn)

		subject := fmt.Sprintf("pokerSend.tournament.%s.%s", table.ID, player.ID)
		consumerName := fmt.Sprintf("durable-consumer4-%s-%s", table.ID, player.ID)
		msgChan := make(chan *nats.Msg, 64)

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

			player.LastAction = action.LastAction
			playersActed++

			switch action.LastAction {
			case "raise":
				// Marcar que ocurrió un raise
				raiseOccurred = true
				lastRaiserIndex = currentIndex
				player.TotalBet += action.LastBet
				table.Players[currentIndex].TotalBet += action.LastBet
				player.Chips -= action.LastBet
				table.Players[currentIndex].Chips -= action.LastBet
				table.BiggestBet = table.Players[currentIndex].TotalBet
				player.CallAmount -= action.LastBet
				table.Players[currentIndex].CallAmount -= action.LastBet
				table.TotalBet += action.LastBet
				table.SetTablePlayersCallAmount()

				playersActed = 1 // Solo el raiser ha actuado después de un raise
				for i := range table.Players {
					if table.Players[i].ID != player.ID && !table.Players[i].HasFold && !table.Players[i].HasAllIn {
						table.Players[i].LastAction = ""
					}
				}

			case "fold":
				player.HasFold = true
			case "call":
				player.TotalBet += action.LastBet
				player.Chips -= action.LastBet
				player.CallAmount -= action.LastBet
				table.Players[currentIndex].TotalBet += action.LastBet
				table.Players[currentIndex].Chips -= action.LastBet
				table.Players[currentIndex].CallAmount -= action.LastBet
				table.TotalBet += action.LastBet
			case "allin":
				player.HasAllIn = true
				player.CallAmount -= action.LastBet
				table.Players[currentIndex].HasAllIn = true
				table.Players[currentIndex].CallAmount -= action.LastBet
				table.TotalBet += action.LastBet
			case "check":
				// Solo registrar el check
			}
		case <-time.After(time.Duration(table.TurnTime) * time.Second):
			player.IsTurn = false
			log.Printf("El tiempo de turno para el jugador %s ha expirado", player.ID)
			player.LastAction = "fold"
			player.HasFold = true
			playersActed++
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		defer sub.Unsubscribe()

		// Verificar si todos los jugadores han actuado y han igualado la apuesta más alta
		allCalled := true
		for _, p := range table.Players {
			if !p.HasFold && !p.HasAllIn && p.CallAmount > 0 {
				allCalled = false
				break
			}
		}
		table.AllPlayersExceptOneFold()

		if table.AllFoldExceptOne || (allCalled && playersActed == len(table.Players)) {
			break
		}

		// Avanzar al siguiente jugador
		currentIndex = (currentIndex + 1) % len(table.Players)

		// Si todos los jugadores han actuado y no hay raise, terminar la ronda
		if currentIndex == startingPlayerIndex && !raiseOccurred {
			break
		}

		// Si hay un raise, el turno debe volver al último jugador que hizo raise
		if raiseOccurred && currentIndex == startingPlayerIndex {
			currentIndex = lastRaiserIndex
			raiseOccurred = false // Resetear el flag de raise para la siguiente ronda
		}
	}

	for i := range table.Players {
		table.Players[i].IsTurn = false
	}

	log.Printf("Los turnos de los jugadores se han completado para la mesa ID: %s", table.ID)
	return table, nil
}

func ShowDown(ctx context.Context, table *poker.Table, config *config.Config) (*poker.Table, error) {
	table.EvaluateHand()
	table.AssignChipsToWinner(&table.Winners[0])

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
