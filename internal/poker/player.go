package poker

import (
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

type Player struct {
	ID               string
	Chips            int
	Cards            []Card
	LastAction       string
	AvailableActions []string
	IsTurn           bool
	IsBB             bool
	IsSB             bool
	PreAction        []string
	LastBet          int
	TotalBet         int
	IsAFK            bool
	CallAmount       int
	HasFold          bool
	HasAllIn         bool
	IsEliminated     bool
	HandStrength     int
	BestHand         []Card
	HandDescription  string
	HandScore        int
	Winnings         int
}

func SendPlayerUpdateToNATS(js nats.JetStreamContext, tableID string, player Player) error {
	subject := fmt.Sprintf("pokerServer.tournament.%s.%s", tableID, player.ID)

	messageBytes, err := json.Marshal(player)
	if err != nil {
		return fmt.Errorf("failed to marshal player data for player %s: %w", player.ID, err)
	}

	if _, err := js.Publish(subject, messageBytes); err != nil {
		return fmt.Errorf("failed to publish message to JetStream for player %s: %w", player.ID, err)
	}

	return nil
}

func SendPlayerNotPointerUpdateToNATS(js nats.JetStreamContext, tableID string, player *Player) error {
	subject := fmt.Sprintf("poker.tournament.%s.%s", tableID, player.ID)

	messageBytes, err := json.Marshal(player)
	if err != nil {
		return fmt.Errorf("failed to marshal player data for player %s: %w", player.ID, err)
	}

	if _, err := js.Publish(subject, messageBytes); err != nil {
		return fmt.Errorf("failed to publish message to JetStream for player %s: %w", player.ID, err)
	}

	return nil
}
