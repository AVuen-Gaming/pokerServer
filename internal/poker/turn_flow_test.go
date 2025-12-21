package poker

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	nserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func startTurnFlowJetStream(t *testing.T) (*nserver.Server, nats.JetStreamContext, *nats.Conn) {
	t.Helper()
	opts := &nserver.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	srv, err := nserver.NewServer(opts)
	if err != nil {
		t.Fatalf("failed to create embedded nats server: %v", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatalf("jetstream server not ready in time")
	}

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("failed to connect to embedded nats: %v", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("failed to create jetstream context: %v", err)
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:      "POKER_TOURNAMENT",
		Subjects:  []string{"pokerServer.>", "pokerClient.>"},
		Retention: nats.LimitsPolicy,
		MaxAge:    time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to add poker stream: %v", err)
	}
	return srv, js, nc
}

func TestHandleTurnPublishesTurnStateAndHonorsAvailableActions(t *testing.T) {
	srv, js, nc := startTurnFlowJetStream(t)
	defer srv.Shutdown()
	defer nc.Close()

	table := &Table{
		ID:           "Table-1",
		TournamentID: 42,
		BBValue:      40,
		TurnTime:     2,
		CurrentSB:    "p1",
		CurrentBB:    "p2",
		CurrentStage: StagePreFlop,
		Players: []Player{
			{ID: "p1", Chips: 200},
			{ID: "p2", Chips: 200},
			{ID: "p3", Chips: 200},
		},
	}

	updates, err := js.PullSubscribe(
		fmt.Sprintf("pokerServer.%d.%s", table.TournamentID, table.ID),
		"turn-flow-updates",
		nats.BindStream("POKER_TOURNAMENT"),
		nats.MaxAckPending(50),
	)
	if err != nil {
		t.Fatalf("failed to subscribe to player updates: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	turnSnapshots := make(map[string]Player)
	actionsSent := make(map[string]int)
	pending := make(map[string]Table)
	errCh := make(chan error, 1)
	go func() {
		errCh <- table.HandleTurn(ctx, js)
	}()

	type turnExpectation struct {
		playerID string
		required bool
	}
	expectedOrder := []turnExpectation{
		{playerID: "p3", required: true},
		{playerID: "p1", required: true},
		{playerID: "p2", required: false},
	}
	for _, expectation := range expectedOrder {
		snapshot, err := nextTurnSnapshot(t, ctx, updates, pending, expectation.playerID)
		if err != nil {
			if expectation.required {
				t.Fatalf("required turn %s missing: %v", expectation.playerID, err)
			}
			t.Logf("optional turn %s missing, sending fallback check: %v", expectation.playerID, err)
			fallback := Player{ID: expectation.playerID, LastAction: "check"}
			payload, marshalErr := json.Marshal(fallback)
			if marshalErr != nil {
				t.Fatalf("failed to encode fallback action: %v", marshalErr)
			}
			subject := fmt.Sprintf("pokerClient.%d.%s.%s", table.TournamentID, table.ID, expectation.playerID)
			if _, publishErr := js.Publish(subject, payload); publishErr != nil {
				t.Fatalf("failed to publish fallback action for %s: %v", expectation.playerID, publishErr)
			}
			actionsSent[expectation.playerID]++
			continue
		}
		player := snapshotPlayer(snapshot, expectation.playerID)
		if player.ID == "" {
			t.Fatalf("player %s not present in snapshot", expectation.playerID)
		}
		player.IsTurn = true
		turnSnapshots[expectation.playerID] = player

		action := buildTurnAction(table, player)
		time.Sleep(50 * time.Millisecond)
		payload, err := json.Marshal(action)
		if err != nil {
			t.Fatalf("failed to encode action: %v", err)
		}
		subject := fmt.Sprintf("pokerClient.%d.%s.%s", table.TournamentID, table.ID, expectation.playerID)
		if _, err := js.Publish(subject, payload); err != nil {
			t.Fatalf("failed to publish action for %s: %v", expectation.playerID, err)
		}
		actionsSent[expectation.playerID]++
		logAction(t, snapshot, player)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("handle turn failed: %v", err)
	}
	t.Logf("post-turn BBValue=%d", table.BBValue)
	if len(actionsSent) < 2 {
		t.Fatalf("expected at least 2 player actions, got %d", len(actionsSent))
	}

	btnSnap, ok := turnSnapshots["p3"]
	if !ok {
		t.Fatalf("missing turn snapshot for button player")
	}
	if !btnSnap.IsTurn || btnSnap.CallAmount != table.BBValue {
		t.Fatalf("button should face BB call, snapshot %+v", btnSnap)
	}
	if !containsAction(btnSnap.AvailableActions, "call") {
		t.Fatalf("button must be able to call: %+v", btnSnap.AvailableActions)
	}

	sbSnap, ok := turnSnapshots["p1"]
	if !ok {
		t.Fatalf("missing small blind snapshot")
	}
	if sbSnap.CallAmount != table.BBValue/2 {
		t.Fatalf("sb should owe half blind: %+v", sbSnap)
	}

	if bbSnap, ok := turnSnapshots["p2"]; ok {
		if bbSnap.CallAmount != 0 || !containsAction(bbSnap.AvailableActions, "check") {
			t.Fatalf("bb should only need to check when no raise: %+v", bbSnap)
		}
	}

	const enforceZeroCallAmounts = true
	if enforceZeroCallAmounts {
		for _, p := range table.Players {
			if p.CallAmount != 0 {
				t.Fatalf("player %s should have zero call amount after round", p.ID)
			}
			if p.LastAction == "" {
				t.Fatalf("player %s should record an action", p.ID)
			}
		}
	}
	btnState := tablePlayer(table, "p3")
	sbState := tablePlayer(table, "p1")
	bbState := tablePlayer(table, "p2")
	t.Logf("final actions -> btn=%s sb=%s bb=%s", safeAction(btnState), safeAction(sbState), safeAction(bbState))

	if action := btnState; action == nil || action.LastAction != "call" {
		t.Fatalf("button should finish with call, got %+v", action)
	}
	if action := sbState; action == nil || action.LastAction != "call" {
		t.Fatalf("small blind should call blind, got %+v", action)
	}
	if action := bbState; action == nil || (action.LastAction != "check" && action.LastAction != "BB") {
		t.Fatalf("big blind should be allowed to check, got %+v", action)
	}
}

func TestHandleTurnRequiresOutstandingCallerBeforeStageAdvances(t *testing.T) {
	srv, js, nc := startTurnFlowJetStream(t)
	defer srv.Shutdown()
	defer nc.Close()

	table := &Table{
		ID:           "Table-raise-allin",
		TournamentID: 77,
		BBValue:      100,
		TurnTime:     2,
		CurrentSB:    "p1",
		CurrentBB:    "p2",
		CurrentStage: StagePreFlop,
		Players: []Player{
			{ID: "p1", Chips: 2500},
			{ID: "p2", Chips: 2500},
			{ID: "p3", Chips: 2500},
			{ID: "p4", Chips: 2500},
			{ID: "p5", Chips: 2500},
			{ID: "p6", Chips: 2500},
		},
	}

	updates, err := js.PullSubscribe(
		fmt.Sprintf("pokerServer.%d.%s", table.TournamentID, table.ID),
		"raise-allin-updates",
		nats.BindStream("POKER_TOURNAMENT"),
		nats.MaxAckPending(50),
	)
	if err != nil {
		t.Fatalf("failed to subscribe to player updates: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- table.HandleTurn(ctx, js)
	}()

	type scriptedStep struct {
		player string
		build  func(Table, Player) Player
	}
	steps := []scriptedStep{
		{
			player: "p3",
			build: func(_ Table, player Player) Player {
				return Player{ID: player.ID, LastAction: "fold"}
			},
		},
		{
			player: "p4",
			build: func(_ Table, player Player) Player {
				raise := player.CallAmount + 400
				if raise <= 0 {
					raise = table.BBValue * 2
				}
				if raise >= player.Chips {
					raise = player.Chips - 1
				}
				if raise <= 0 {
					t.Fatalf("invalid raise amount for player %s", player.ID)
				}
				return Player{ID: player.ID, LastAction: "raise", LastBet: raise}
			},
		},
		{
			player: "p5",
			build: func(_ Table, player Player) Player {
				return Player{ID: player.ID, LastAction: "allin"}
			},
		},
		{
			player: "p6",
			build: func(_ Table, player Player) Player {
				return Player{ID: player.ID, LastAction: "allin"}
			},
		},
		{
			player: "p1",
			build: func(_ Table, player Player) Player {
				return Player{ID: player.ID, LastAction: "fold"}
			},
		},
		{
			player: "p2",
			build: func(_ Table, player Player) Player {
				return Player{ID: player.ID, LastAction: "fold"}
			},
		},
		{
			player: "p4",
			build: func(_ Table, player Player) Player {
				if player.CallAmount <= 0 {
					t.Fatalf("expected outstanding call for %s, got %d", player.ID, player.CallAmount)
				}
				return Player{ID: player.ID, LastAction: "call", LastBet: player.CallAmount}
			},
		},
	}

	pending := make(map[string]Table)
	for idx, step := range steps {
		snapshot, err := nextTurnSnapshot(t, ctx, updates, pending, step.player)
		if err != nil {
			t.Fatalf("failed waiting for turn %s at step %d: %v", step.player, idx, err)
		}
		player := snapshotPlayer(snapshot, step.player)
		if player.ID == "" {
			t.Fatalf("player %s missing in snapshot", step.player)
		}
		action := step.build(snapshot, player)
		payload, marshalErr := json.Marshal(action)
		if marshalErr != nil {
			t.Fatalf("failed encoding action for %s: %v", step.player, marshalErr)
		}
		subject := fmt.Sprintf("pokerClient.%d.%s.%s", table.TournamentID, table.ID, action.ID)
		if _, publishErr := js.Publish(subject, payload); publishErr != nil {
			t.Fatalf("failed publishing action for %s: %v", step.player, publishErr)
		}
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("handle turn failed: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("handle turn did not finish in time: %v", ctx.Err())
	}
}

func containsAction(actions []string, target string) bool {
	for _, action := range actions {
		if action == target {
			return true
		}
	}
	return false
}

func buildTurnAction(table *Table, player Player) Player {
	action := Player{ID: player.ID}
	if player.CallAmount > 0 {
		action.LastAction = "call"
		action.LastBet = player.CallAmount
	} else {
		action.LastAction = "check"
	}
	return action
}

func tablePlayer(table *Table, playerID string) *Player {
	for i := range table.Players {
		if table.Players[i].ID == playerID {
			return &table.Players[i]
		}
	}
	return nil
}

func safeAction(player *Player) string {
	if player == nil {
		return "<nil>"
	}
	return player.LastAction
}

func nextTurnSnapshot(t *testing.T, ctx context.Context, sub *nats.Subscription, pending map[string]Table, playerID string) (Table, error) {
	if snap, ok := pending[playerID]; ok {
		delete(pending, playerID)
		return snap, nil
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if time.Now().After(deadline) {
			return Table{}, fmt.Errorf("timed out waiting for turn %s", playerID)
		}
		select {
		case <-ctx.Done():
			return Table{}, fmt.Errorf("context done while waiting for %s", playerID)
		default:
		}

		msgs, err := sub.Fetch(1, nats.MaxWait(250*time.Millisecond))
		if err != nil {
			if err == nats.ErrTimeout {
				continue
			}
			return Table{}, fmt.Errorf("failed fetching updates: %w", err)
		}

		var snapshot Table
		if err := json.Unmarshal(msgs[0].Data, &snapshot); err != nil {
			return Table{}, fmt.Errorf("failed decoding player update: %w", err)
		}
		if snapshot.CurrentTurn == "" {
			if err := msgs[0].Ack(); err != nil {
				return Table{}, fmt.Errorf("failed acking player update: %w", err)
			}
			continue
		}
		if snapshot.CurrentTurn != playerID {
			t.Logf("buffering snapshot for %s during wait for %s", snapshot.CurrentTurn, playerID)
			pending[snapshot.CurrentTurn] = snapshot
			if err := msgs[0].Ack(); err != nil {
				return Table{}, fmt.Errorf("failed acking player update: %w", err)
			}
			continue
		}
		if err := msgs[0].Ack(); err != nil {
			return Table{}, fmt.Errorf("failed acking player update: %w", err)
		}
		t.Logf("consumed snapshot for %s", snapshot.CurrentTurn)
		return snapshot, nil
	}
}

func snapshotPlayer(snapshot Table, playerID string) Player {
	for _, candidate := range snapshot.Players {
		if candidate.ID == playerID {
			return candidate
		}
	}
	return Player{}
}

func logAction(t *testing.T, snapshot Table, player Player) {
	t.Logf("auto action -> player=%s stage=%s call=%d", player.ID, snapshot.CurrentStage, player.CallAmount)
}
