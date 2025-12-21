//go:build !js && !wasm

package nats

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"server/config"
	"server/internal/poker"

	nserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func runEmbeddedNATSServer(t *testing.T) *nserver.Server {
	t.Helper()
	opts := &nserver.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		Username:  "user",
		Password:  "pass",
	}
	srv, err := nserver.NewServer(opts)
	if err != nil {
		t.Fatalf("failed to create nats server: %v", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatalf("nats server not ready in time")
	}
	return srv
}

func buildTestConfig(port int) *config.Config {
	return &config.Config{
		NATS: config.NATSConfig{
			Host:     "127.0.0.1",
			Port:     port,
			Username: "user",
			Password: "pass",
			Stream: config.StreamConfig{
				Name:     "UNITY_ACTIONS",
				Subjects: []string{"unity.actions"},
			},
		},
	}
}

type unityServerSimulation struct {
	tournamentID string
	playerID     string
	js           nats.JetStreamContext
	t            *testing.T
	currentTable string
	actions      int
	reshuffled   bool
	started      bool
	opponents    int
}

func newUnityServerSimulation(t *testing.T, js nats.JetStreamContext, tournamentID, playerID string) *unityServerSimulation {
	return &unityServerSimulation{
		tournamentID: tournamentID,
		playerID:     playerID,
		js:           js,
		t:            t,
		currentTable: "Table-1",
		opponents:    3,
	}
}

func (u *unityServerSimulation) publishPlayerUpdate(update poker.Player) {
	update.ID = u.playerID
	if update.CurrentTable == "" {
		update.CurrentTable = u.currentTable
	}
	data, err := json.Marshal(update)
	if err != nil {
		u.t.Fatalf("failed to encode update: %v", err)
	}
	subject := fmt.Sprintf("pokerServer.%s.%s.%s", u.tournamentID, update.CurrentTable, u.playerID)
	if _, err := u.js.Publish(subject, data); err != nil {
		u.t.Fatalf("failed to publish update: %v", err)
	}
}

func (u *unityServerSimulation) announceTournamentStart() {
	u.started = true
	u.publishPlayerUpdate(poker.Player{LastAction: "tournament_start", IsTurn: true})
}

func (u *unityServerSimulation) announceOpponentEliminated() {
	if u.opponents > 1 {
		u.opponents--
	}
	u.publishPlayerUpdate(poker.Player{LastAction: "opponent_eliminated"})
}

func (u *unityServerSimulation) start() *nats.Subscription {
	subject := fmt.Sprintf("pokerClient.%s.>", u.tournamentID)
	sub, err := u.js.Subscribe(subject, u.handleAction,
		nats.Durable(fmt.Sprintf("unity-sim-%s", u.playerID)),
		nats.ManualAck(),
		nats.AckExplicit(),
		nats.BindStream("POKER_TOURNAMENT"))
	if err != nil {
		u.t.Fatalf("failed to subscribe unity simulation: %v", err)
	}
	return sub
}

func (u *unityServerSimulation) handleAction(msg *nats.Msg) {
	defer msg.Ack()
	u.actions++
	var action poker.Player
	if err := json.Unmarshal(msg.Data, &action); err != nil {
		u.t.Fatalf("failed to decode player action: %v", err)
	}
	update := poker.Player{
		ID:           u.playerID,
		CurrentTable: u.currentTable,
		LastAction:   action.LastAction,
		LastBet:      action.LastBet,
		Chips:        1000 - action.LastBet,
	}

	switch action.LastAction {
	case "await_start":
		update.LastAction = "waiting_start"
		update.IsTurn = false
	case "SB":
		update.IsSB = true
	case "BB":
		update.IsBB = true
	case "call":
		update.CallAmount = 0
	case "check", "pass":
		update.LastAction = "check"
	case "raise":
		update.TotalBet = action.LastBet
	case "reshuffle":
		if !u.reshuffled {
			u.reshuffled = true
			u.currentTable = fmt.Sprintf("Table-%d", u.actions+1)
			update.SwitchingTable = true
			update.CurrentTable = u.currentTable
		}
	case "fold":
		update.HasFold = true
	case "allin":
		update.HasAllIn = true
		update.Chips = 0
		update.IsEliminated = true
		update.LastAction = "eliminated"
	}

	u.publishPlayerUpdate(update)
}

func waitForPlayerUpdate(t *testing.T, sub *nats.Subscription, predicate func(poker.Player) bool, timeout time.Duration) poker.Player {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msgs, err := sub.Fetch(1, nats.MaxWait(500*time.Millisecond))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				continue
			}
			t.Fatalf("failed to fetch player update: %v", err)
		}
		var player poker.Player
		if err := json.Unmarshal(msgs[0].Data, &player); err != nil {
			t.Fatalf("failed to decode player update: %v", err)
		}
		msgs[0].Ack()
		if predicate(player) {
			return player
		}
	}
	t.Fatalf("timeout waiting for player update")
	return poker.Player{}
}

func TestConnectConfiguresDefaultStream(t *testing.T) {
	srv := runEmbeddedNATSServer(t)
	defer srv.Shutdown()

	port := srv.Addr().(*net.TCPAddr).Port
	cfg := buildTestConfig(port)

	nc, js, err := Connect(cfg)
	if err != nil {
		t.Fatalf("connect should succeed: %v", err)
	}
	defer nc.Close()

	info, err := js.StreamInfo("POKER_TOURNAMENT")
	if err != nil {
		t.Fatalf("expected default poker stream to exist: %v", err)
	}
	if info.Config.MaxAge != time.Hour {
		t.Fatalf("expected MaxAge 1h, got %v", info.Config.MaxAge)
	}

	if err := ConfigureStream(js, &cfg.NATS.Stream); err != nil {
		t.Fatalf("failed to create unity stream: %v", err)
	}
	customInfo, err := js.StreamInfo(cfg.NATS.Stream.Name)
	if err != nil {
		t.Fatalf("unity stream missing: %v", err)
	}
	if len(customInfo.Config.Subjects) != 1 || customInfo.Config.Subjects[0] != "unity.actions" {
		t.Fatalf("unexpected subjects: %+v", customInfo.Config.Subjects)
	}
}

func TestUnityActionFlowPublishesAndConsumes(t *testing.T) {
	srv := runEmbeddedNATSServer(t)
	defer srv.Shutdown()

	port := srv.Addr().(*net.TCPAddr).Port
	cfg := buildTestConfig(port)

	nc, js, err := Connect(cfg)
	if err != nil {
		t.Fatalf("connect should succeed: %v", err)
	}
	defer nc.Close()

	subject := "pokerClient.1.table.wallet"
	sub, err := js.PullSubscribe(subject, "durable-consumer4-table-wallet", nats.BindStream("POKER_TOURNAMENT"), nats.MaxAckPending(1))
	if err != nil {
		t.Fatalf("failed to create pull subscription: %v", err)
	}

	payload := []byte(`{"LastAction":"raise","LastBet":200}`)
	if _, err := js.Publish(subject, payload); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	msgs, err := sub.Fetch(1, nats.MaxWait(2*time.Second))
	if err != nil {
		t.Fatalf("failed to fetch unity action: %v", err)
	}
	if string(msgs[0].Data) != string(payload) {
		t.Fatalf("unexpected payload: %s", msgs[0].Data)
	}
	if err := msgs[0].Ack(); err != nil {
		t.Fatalf("failed to ACK message: %v", err)
	}
}

func TestUnityPlayerLifecycleOverJetStream(t *testing.T) {
	srv := runEmbeddedNATSServer(t)
	defer srv.Shutdown()

	port := srv.Addr().(*net.TCPAddr).Port
	cfg := buildTestConfig(port)

	nc, js, err := Connect(cfg)
	if err != nil {
		t.Fatalf("connect should succeed: %v", err)
	}
	defer nc.Close()

	playerID := "walletA"
	tournamentID := "1"
	serverSim := newUnityServerSimulation(t, js, tournamentID, playerID)
	serverSub := serverSim.start()
	defer serverSub.Drain()

	updates, err := js.PullSubscribe(fmt.Sprintf("pokerServer.%s.>", tournamentID), "unity-player-updates",
		nats.BindStream("POKER_TOURNAMENT"), nats.MaxAckPending(20))
	if err != nil {
		t.Fatalf("failed to subscribe to player updates: %v", err)
	}

	playerSubject := fmt.Sprintf("pokerClient.%s.%s.%s", tournamentID, "Table-1", playerID)
	sendAction := func(action string, bet int, expectSwitch bool, predicate func(poker.Player) bool) {
		payload := poker.Player{ID: playerID, LastAction: action, LastBet: bet}
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to marshal action: %v", err)
		}
		if _, err := js.Publish(playerSubject, data); err != nil {
			t.Fatalf("failed to publish unity action: %v", err)
		}

		update := waitForPlayerUpdate(t, updates, predicate, 3*time.Second)
		if expectSwitch {
			playerSubject = fmt.Sprintf("pokerClient.%s.%s.%s", tournamentID, update.CurrentTable, playerID)
		}
	}

	sendAction("await_start", 0, false, func(p poker.Player) bool { return p.LastAction == "waiting_start" })
	serverSim.announceTournamentStart()
	waitForPlayerUpdate(t, updates, func(p poker.Player) bool { return p.LastAction == "tournament_start" }, 3*time.Second)

	sendAction("SB", 10, false, func(p poker.Player) bool { return p.IsSB })
	sendAction("BB", 20, false, func(p poker.Player) bool { return p.IsBB })
	sendAction("call", 20, false, func(p poker.Player) bool { return p.LastAction == "call" && p.CallAmount == 0 })
	sendAction("pass", 0, false, func(p poker.Player) bool { return p.LastAction == "check" })

	serverSim.announceOpponentEliminated()
	waitForPlayerUpdate(t, updates, func(p poker.Player) bool { return p.LastAction == "opponent_eliminated" }, 3*time.Second)

	sendAction("call", 0, false, func(p poker.Player) bool { return p.LastAction == "call" })
	sendAction("reshuffle", 0, true, func(p poker.Player) bool { return p.SwitchingTable && p.CurrentTable != "Table-1" })
	sendAction("fold", 0, false, func(p poker.Player) bool { return p.HasFold })
	sendAction("allin", 0, false, func(p poker.Player) bool { return p.IsEliminated })
}
