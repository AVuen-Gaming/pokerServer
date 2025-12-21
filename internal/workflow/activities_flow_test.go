package temporal

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"server/config"
	"server/internal/poker"

	nserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func startWorkflowJetStream(t *testing.T) (*nserver.Server, nats.JetStreamContext, *nats.Conn) {
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
		t.Fatalf("embedded jetstream not ready")
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

func TestHandleTableActivitieAdvancesFullLifecycle(t *testing.T) {
	srv, js, nc := startWorkflowJetStream(t)
	defer srv.Shutdown()
	defer nc.Close()

	SetJetStream(js)

	initialBB := 20
	table := &poker.Table{
		ID:                 "Table-1",
		TournamentID:       314,
		BBValue:            initialBB,
		TurnTime:           2,
		CurrentSB:          "p1",
		CurrentBB:          "p2",
		IncrementBlind:     1,
		LastIncrementBlind: time.Now().Add(-2 * time.Minute),
		Players: []poker.Player{
			{ID: "p1", Chips: 500},
			{ID: "p2", Chips: 500},
			{ID: "p3", Chips: 500},
			{ID: "p4", Chips: 500},
		},
	}
	expectedSB, expectedBB := computeNextBlinds(table.Players, table.CurrentSB)
	if expectedSB == "" || expectedBB == "" {
		t.Fatalf("failed to compute expected blind rotation")
	}
	prevIncrement := table.LastIncrementBlind

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var responders sync.WaitGroup
	var actions atomic.Int32
	responders.Add(1)
	go autoPlayTurns(t, ctx, js, table.TournamentID, table.ID, &responders, &actions)

	stages, stageWG := captureTableStages(t, ctx, js, table)

	updated, err := HandleTableActivitie(ctx, table, &config.Config{})
	if err != nil {
		t.Fatalf("HandleTableActivitie failed: %v", err)
	}

	cancel()
	responders.Wait()
	stageWG.Wait()
	stageSeq := stages.snapshot()
	stageSeq = append(stageSeq, updated.CurrentStage)
	stageTimeline := collapseSequentialStages(stageSeq)

	if actions.Load() == 0 {
		t.Fatalf("auto player never published an action")
	}
	t.Logf("auto actions recorded: %d", actions.Load())

	if updated.CurrentStage != poker.StageShowDown && updated.CurrentStage != poker.StageShowDownAllFoldExceptOne {
		t.Fatalf("expected table to finish at showdown, got %s", updated.CurrentStage)
	}
	if len(updated.Winners) == 0 {
		t.Fatalf("expected at least one winner after activity")
	}

	coreOrder := []string{
		poker.StageInitRound,
		poker.StagePreFlop,
		poker.StageFlop,
		poker.StageTurn,
	}
	if !containsStagesInOrder(stageTimeline, coreOrder) {
		t.Fatalf("stage timeline missing ordered phases: %v", stageTimeline)
	}
	if containsStage(stageTimeline, poker.StageRiver) {
		withRiver := append(coreOrder, poker.StageRiver)
		if !containsStagesInOrder(stageTimeline, withRiver) {
			t.Fatalf("stage timeline contains river but order is invalid: %v", stageTimeline)
		}
	}
	if final := stageTimeline[len(stageTimeline)-1]; final != poker.StageShowDown && final != poker.StageShowDownAllFoldExceptOne {
		t.Fatalf("final stage should finish at showdown, got %s (timeline=%v)", final, stageTimeline)
	}

	expectedBBValue := initialBB * 2
	if updated.BBValue != expectedBBValue {
		t.Fatalf("blind increment mismatch: want %d got %d", expectedBBValue, updated.BBValue)
	}
	if !updated.LastIncrementBlind.After(prevIncrement) {
		t.Fatalf("last increment timestamp was not updated: prev=%v new=%v", prevIncrement, updated.LastIncrementBlind)
	}
	if updated.CurrentSB != expectedSB || updated.CurrentBB != expectedBB {
		t.Fatalf("blind rotation mismatch: expected SB=%s BB=%s, got SB=%s BB=%s", expectedSB, expectedBB, updated.CurrentSB, updated.CurrentBB)
	}
}

func TestHandleTableActivitieMultipleAllInsReachShowdown(t *testing.T) {
	srv, js, nc := startWorkflowJetStream(t)
	defer srv.Shutdown()
	defer nc.Close()

	SetJetStream(js)

	table := &poker.Table{
		ID:           "Table-AllIn",
		TournamentID: 777,
		BBValue:      50,
		TurnTime:     2,
		CurrentSB:    "p1",
		CurrentBB:    "p2",
		Players: []poker.Player{
			{ID: "p1", Chips: 500},
			{ID: "p2", Chips: 500},
			{ID: "p3", Chips: 500},
			{ID: "p4", Chips: 500},
		},
	}

	script := map[string][]poker.Player{
		"p3": {{ID: "p3", LastAction: "allin"}},
		"p4": {{ID: "p4", LastAction: "allin"}},
		"p1": {{ID: "p1", LastAction: "allin"}},
		"p2": {{ID: "p2", LastAction: "allin"}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	collector, stageWG := captureTableStages(t, ctx, js, table)
	var responders sync.WaitGroup
	responders.Add(1)
	go scriptedTurnRunner(t, ctx, js, table.TournamentID, table.ID, script, nil, &responders)

	updated, err := HandleTableActivitie(ctx, table, &config.Config{})
	if err != nil {
		t.Fatalf("HandleTableActivitie failed in all-in test: %v", err)
	}

	cancel()
	responders.Wait()
	stageWG.Wait()
	stageTimeline := collapseSequentialStages(append(collector.snapshot(), updated.CurrentStage))

	if !containsStage(stageTimeline, poker.StageFlop) || !containsStage(stageTimeline, poker.StageTurn) {
		t.Fatalf("expected board stages after multiple all-ins, got %v", stageTimeline)
	}
	if updated.CurrentStage != poker.StageShowDown {
		t.Fatalf("expected final stage showDown, got %s", updated.CurrentStage)
	}
	if updated.AllFoldExceptOne {
		t.Fatalf("table incorrectly marked as AllFoldExceptOne")
	}
	if len(updated.Winners) == 0 {
		t.Fatalf("expected winners after showdown")
	}
}

func TestHandleTableActivitieAllFoldEndsPreFlop(t *testing.T) {
	srv, js, nc := startWorkflowJetStream(t)
	defer srv.Shutdown()
	defer nc.Close()

	SetJetStream(js)

	table := &poker.Table{
		ID:           "Table-Fold",
		TournamentID: 888,
		BBValue:      40,
		TurnTime:     2,
		CurrentSB:    "p1",
		CurrentBB:    "p2",
		Players: []poker.Player{
			{ID: "p1", Chips: 400},
			{ID: "p2", Chips: 400},
			{ID: "p3", Chips: 400},
			{ID: "p4", Chips: 400},
		},
	}

	script := map[string][]poker.Player{
		"p3": {{ID: "p3", LastAction: "fold"}},
		"p4": {{ID: "p4", LastAction: "fold"}},
		"p1": {{ID: "p1", LastAction: "fold"}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	collector, stageWG := captureTableStages(t, ctx, js, table)
	var responders sync.WaitGroup
	responders.Add(1)
	go scriptedTurnRunner(t, ctx, js, table.TournamentID, table.ID, script, nil, &responders)

	updated, err := HandleTableActivitie(ctx, table, &config.Config{})
	if err != nil {
		t.Fatalf("HandleTableActivitie failed in all-fold test: %v", err)
	}

	cancel()
	responders.Wait()
	stageWG.Wait()
	stageTimeline := collapseSequentialStages(append(collector.snapshot(), updated.CurrentStage))

	if containsStage(stageTimeline, poker.StageFlop) {
		t.Fatalf("expected to stop before flop, timeline=%v", stageTimeline)
	}
	if updated.CurrentStage != poker.StageShowDownAllFoldExceptOne {
		t.Fatalf("expected final stage showDownAllFoldExceptOne, got %s", updated.CurrentStage)
	}
	if len(updated.Winners) != 1 {
		t.Fatalf("expected exactly one winner, got %d", len(updated.Winners))
	}
}

func TestHandleTableActivitieAllInSurvivorSkipsBoard(t *testing.T) {
	srv, js, nc := startWorkflowJetStream(t)
	defer srv.Shutdown()
	defer nc.Close()

	SetJetStream(js)

	table := &poker.Table{
		ID:           "Table-AllInFold",
		TournamentID: 890,
		BBValue:      40,
		TurnTime:     2,
		CurrentSB:    "p1",
		CurrentBB:    "p2",
		Players: []poker.Player{
			{ID: "p1", Chips: 400},
			{ID: "p2", Chips: 400},
			{ID: "p3", Chips: 400},
			{ID: "p4", Chips: 400},
		},
	}

	script := map[string][]poker.Player{
		"p3": {{ID: "p3", LastAction: "allin"}},
		"p4": {{ID: "p4", LastAction: "fold"}},
		"p1": {{ID: "p1", LastAction: "fold"}},
		"p2": {{ID: "p2", LastAction: "fold"}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	collector, stageWG := captureTableStages(t, ctx, js, table)
	var responders sync.WaitGroup
	responders.Add(1)
	go scriptedTurnRunner(t, ctx, js, table.TournamentID, table.ID, script, nil, &responders)

	updated, err := HandleTableActivitie(ctx, table, &config.Config{})
	if err != nil {
		t.Fatalf("HandleTableActivitie failed in all-in survivor test: %v", err)
	}

	cancel()
	responders.Wait()
	stageWG.Wait()
	stageTimeline := collapseSequentialStages(append(collector.snapshot(), updated.CurrentStage))

	if containsStage(stageTimeline, poker.StageFlop) {
		t.Fatalf("expected to skip board stages when only one player remains, timeline=%v", stageTimeline)
	}
	if updated.CurrentStage != poker.StageShowDownAllFoldExceptOne {
		t.Fatalf("expected showDownAllFoldExceptOne, got %s", updated.CurrentStage)
	}
	if len(updated.Winners) != 1 || updated.Winners[0].ID != "p3" {
		t.Fatalf("expected p3 to win uncontested, winners=%v", updated.Winners)
	}
}

func TestHandleTableActivitieHeadsUpFoldVsAllIn(t *testing.T) {
	srv, js, nc := startWorkflowJetStream(t)
	defer srv.Shutdown()
	defer nc.Close()

	SetJetStream(js)

	table := &poker.Table{
		ID:           "Table-HeadsUp",
		TournamentID: 891,
		BBValue:      40,
		TurnTime:     2,
		CurrentSB:    "p1",
		CurrentBB:    "p2",
		Players: []poker.Player{
			{ID: "p1", Chips: 400},
			{ID: "p2", Chips: 400},
		},
	}

	script := map[string][]poker.Player{
		"p1": {{ID: "p1", LastAction: "fold"}},
		"p2": {{ID: "p2", LastAction: "allin"}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	collector, stageWG := captureTableStages(t, ctx, js, table)
	counters := map[string]*atomic.Int32{
		"p1": &atomic.Int32{},
		"p2": &atomic.Int32{},
	}
	var responders sync.WaitGroup
	responders.Add(1)
	go scriptedTurnRunner(t, ctx, js, table.TournamentID, table.ID, script, counters, &responders)

	updated, err := HandleTableActivitie(ctx, table, &config.Config{})
	if err != nil {
		t.Fatalf("HandleTableActivitie failed in heads-up fold/all-in test: %v", err)
	}

	cancel()
	responders.Wait()
	stageWG.Wait()
	stageTimeline := collapseSequentialStages(append(collector.snapshot(), updated.CurrentStage))

	if containsStage(stageTimeline, poker.StageFlop) {
		t.Logf("scripted actions: p1=%d p2=%d", counters["p1"].Load(), counters["p2"].Load())
		for idx, snap := range collector.snapshots {
			if snap.CurrentStage != poker.StagePreFlop {
				continue
			}
			t.Logf("snapshot[%d] stage=%s turn=%s", idx, snap.CurrentStage, snap.CurrentTurn)
			for _, p := range snap.Players {
				t.Logf("  player=%s fold=%v allin=%v chips=%d total=%d", p.ID, p.HasFold, p.HasAllIn, p.Chips, p.TotalBet)
			}
		}
		t.Fatalf("heads-up hand should not reach flop when one player folds, timeline=%v", stageTimeline)
	}
	if updated.CurrentStage != poker.StageShowDownAllFoldExceptOne {
		t.Fatalf("expected showDownAllFoldExceptOne, got %s", updated.CurrentStage)
	}
	if len(updated.Winners) != 1 || updated.Winners[0].ID != "p2" {
		t.Fatalf("expected p2 to win uncontested, winners=%v", updated.Winners)
	}
}

func TestHandleTableActivitieHeadsUpFoldSkipsBoard(t *testing.T) {
	srv, js, nc := startWorkflowJetStream(t)
	defer srv.Shutdown()
	defer nc.Close()

	SetJetStream(js)

	table := &poker.Table{
		ID:           "Table-HeadsUpFold",
		TournamentID: 892,
		BBValue:      40,
		TurnTime:     2,
		CurrentSB:    "p1",
		CurrentBB:    "p2",
		Players: []poker.Player{
			{ID: "p1", Chips: 600},
			{ID: "p2", Chips: 600},
		},
	}

	script := map[string][]poker.Player{
		"p1": {{ID: "p1", LastAction: "fold"}},
		"p2": {{ID: "p2", LastAction: "check"}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	collector, stageWG := captureTableStages(t, ctx, js, table)
	var responders sync.WaitGroup
	responders.Add(1)
	go scriptedTurnRunner(t, ctx, js, table.TournamentID, table.ID, script, nil, &responders)

	updated, err := HandleTableActivitie(ctx, table, &config.Config{})
	if err != nil {
		t.Fatalf("HandleTableActivitie failed in heads-up fold test: %v", err)
	}

	cancel()
	responders.Wait()
	stageWG.Wait()
	stageTimeline := collapseSequentialStages(append(collector.snapshot(), updated.CurrentStage))

	if containsStage(stageTimeline, poker.StageFlop) {
		t.Fatalf("heads-up fold should not reach flop, timeline=%v", stageTimeline)
	}
	if updated.CurrentStage != poker.StageShowDownAllFoldExceptOne {
		t.Fatalf("expected showDownAllFoldExceptOne, got %s", updated.CurrentStage)
	}
	if len(updated.Winners) != 1 || updated.Winners[0].ID != "p2" {
		t.Fatalf("expected p2 to win by default, winners=%v", updated.Winners)
	}
}

func TestHandleTableActivitieStopsWhenSinglePlayerRemains(t *testing.T) {
	srv, js, nc := startWorkflowJetStream(t)
	defer srv.Shutdown()
	defer nc.Close()

	SetJetStream(js)

	table := &poker.Table{
		ID:           "Table-Solo",
		TournamentID: 893,
		BBValue:      40,
		TurnTime:     2,
		CurrentSB:    "p1",
		CurrentBB:    "p1",
		Players: []poker.Player{
			{ID: "p1", Chips: 5000},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collector, stageWG := captureTableStages(t, ctx, js, table)
	updated, err := HandleTableActivitie(ctx, table, &config.Config{})
	if err != nil {
		t.Fatalf("HandleTableActivitie failed with single player: %v", err)
	}

	stageWG.Wait()
	stageTimeline := collapseSequentialStages(append(collector.snapshot(), updated.CurrentStage))

	if !containsStage(stageTimeline, poker.StageFinishTable) {
		t.Fatalf("expected finishTable stage, timeline=%v", stageTimeline)
	}
	if updated.CurrentStage != poker.StageFinishTable {
		t.Fatalf("expected CurrentStage finishTable, got %s", updated.CurrentStage)
	}
	if len(updated.Winners) != 1 || updated.Winners[0].ID != "p1" {
		t.Fatalf("expected lone player to be recorded as winner, winners=%v", updated.Winners)
	}
}

func TestHandleTableActivitieReopensTurnsAfterRaise(t *testing.T) {
	srv, js, nc := startWorkflowJetStream(t)
	defer srv.Shutdown()
	defer nc.Close()

	SetJetStream(js)

	table := &poker.Table{
		ID:           "Table-Reopen",
		TournamentID: 889,
		BBValue:      40,
		TurnTime:     2,
		CurrentSB:    "p1",
		CurrentBB:    "p2",
		Players: []poker.Player{
			{ID: "p1", Chips: 400},
			{ID: "p2", Chips: 400},
			{ID: "p3", Chips: 400},
			{ID: "p4", Chips: 400},
		},
	}

	script := map[string][]poker.Player{
		"p3": {
			{ID: "p3", LastAction: "call", LastBet: 40},
			{ID: "p3", LastAction: "call", LastBet: 80},
		},
		"p4": {{ID: "p4", LastAction: "raise", LastBet: 120}},
		"p1": {{ID: "p1", LastAction: "fold"}},
		"p2": {{ID: "p2", LastAction: "fold"}},
	}

	counters := map[string]*atomic.Int32{
		"p1": &atomic.Int32{},
		"p2": &atomic.Int32{},
		"p3": &atomic.Int32{},
		"p4": &atomic.Int32{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	collector, stageWG := captureTableStages(t, ctx, js, table)
	var responders sync.WaitGroup
	responders.Add(1)
	go scriptedTurnRunner(t, ctx, js, table.TournamentID, table.ID, script, counters, &responders)

	updated, err := HandleTableActivitie(ctx, table, &config.Config{})
	if err != nil {
		t.Fatalf("HandleTableActivitie failed in reopen test: %v", err)
	}

	cancel()
	responders.Wait()
	stageWG.Wait()
	stageTimeline := collapseSequentialStages(append(collector.snapshot(), updated.CurrentStage))

	t.Logf("stage timeline: %v", stageTimeline)
	t.Logf("table flags: allFoldExceptOne=%v roundFinish=%v", updated.AllFoldExceptOne, updated.RoundFinish)
	if last := collector.latestTable(); last != nil {
		for _, p := range last.Players {
			t.Logf("last-stage player=%s totalBet=%d call=%d hasFold=%v hasAllIn=%v", p.ID, p.TotalBet, p.CallAmount, p.HasFold, p.HasAllIn)
		}
	}
	for _, p := range updated.Players {
		t.Logf("player=%s totalBet=%d call=%d hasFold=%v hasAllIn=%v", p.ID, p.TotalBet, p.CallAmount, p.HasFold, p.HasAllIn)
	}
	for playerID, queue := range script {
		t.Logf("pending scripted actions %s=%d", playerID, len(queue))
	}
	t.Logf("actions count -> p1=%d p2=%d p3=%d p4=%d", counters["p1"].Load(), counters["p2"].Load(), counters["p3"].Load(), counters["p4"].Load())
	if counters["p3"].Load() < 2 {
		t.Fatalf("expected player p3 to act twice after raise, got %d", counters["p3"].Load())
	}
	if !containsStage(stageTimeline, poker.StageFlop) {
		t.Fatalf("expected table to progress beyond preflop, timeline=%v", stageTimeline)
	}
	if len(updated.Players) == 0 {
		t.Fatalf("expected players to remain after reopen test")
	}
}

func TestHandleTableActivitieTurnOrderAcrossStages(t *testing.T) {
	srv, js, nc := startWorkflowJetStream(t)
	defer srv.Shutdown()
	defer nc.Close()

	SetJetStream(js)

	table := &poker.Table{
		ID:           "Table-MultiRaise",
		TournamentID: 901,
		BBValue:      40,
		TurnTime:     2,
		CurrentSB:    "p4",
		CurrentBB:    "p1",
		Players: []poker.Player{
			{ID: "p1", Chips: 2000},
			{ID: "p2", Chips: 2000},
			{ID: "p3", Chips: 2000},
			{ID: "p4", Chips: 2000},
		},
	}
	expectedSB := nextSeatID(table.Players, table.CurrentSB)
	expectedBB := nextSeatID(table.Players, expectedSB)
	utg := nextSeatID(table.Players, expectedBB)
	preflopSeating := orderStartingFrom(table.Players, utg, len(table.Players))
	flopSeating := orderStartingFrom(table.Players, expectedSB, len(table.Players))

	script := map[string][]poker.Player{
		"p3": {
			{ID: "p3", LastAction: "call", LastBet: 40},
			{ID: "p3", LastAction: "call", LastBet: 80},
			{ID: "p3", LastAction: "raise", LastBet: 400},
			{ID: "p3", LastAction: "check"},
			{ID: "p3", LastAction: "check"},
		},
		"p4": {
			{ID: "p4", LastAction: "raise", LastBet: 120},
			{ID: "p4", LastAction: "fold"},
		},
		"p1": {
			{ID: "p1", LastAction: "call", LastBet: 100},
			{ID: "p1", LastAction: "check"},
			{ID: "p1", LastAction: "call", LastBet: 400},
			{ID: "p1", LastAction: "check"},
			{ID: "p1", LastAction: "check"},
		},
		"p2": {
			{ID: "p2", LastAction: "call", LastBet: 80},
			{ID: "p2", LastAction: "raise", LastBet: 200},
			{ID: "p2", LastAction: "call", LastBet: 200},
			{ID: "p2", LastAction: "check"},
			{ID: "p2", LastAction: "check"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	collector, stageWG := captureTableStages(t, ctx, js, table)
	var responders sync.WaitGroup
	responders.Add(1)
	go scriptedTurnRunner(t, ctx, js, table.TournamentID, table.ID, script, nil, &responders)

	updated, err := HandleTableActivitie(ctx, table, &config.Config{})
	if err != nil {
		t.Fatalf("HandleTableActivitie failed in turn-order test: %v", err)
	}

	cancel()
	responders.Wait()
	stageWG.Wait()

	if updated.CurrentSB != expectedSB || updated.CurrentBB != expectedBB {
		t.Fatalf("unexpected blinds after hand: got SB=%s BB=%s want SB=%s BB=%s", updated.CurrentSB, updated.CurrentBB, expectedSB, expectedBB)
	}

	preflopOrder := collector.turnTimeline(poker.StagePreFlop, 1)
	if len(preflopOrder) < len(preflopSeating) {
		t.Fatalf("preflop turn sequence too short: got %v", preflopOrder)
	}
	if !reflect.DeepEqual(preflopOrder[:len(preflopSeating)], preflopSeating) {
		t.Fatalf("preflop seating order mismatch: got %v want %v", preflopOrder[:len(preflopSeating)], preflopSeating)
	}
	// Preflop can finish on the player immediately before the last aggressor; just ensure
	// the first orbit matches seating order and action reopened when expected.
	if countOccurrences(preflopOrder, utg) < 2 {
		t.Fatalf("UTG %s should act twice after raise, sequence=%v", utg, preflopOrder)
	}

	flopOrder := collector.turnTimeline(poker.StageFlop, 1)
	if len(flopOrder) < len(flopSeating) {
		t.Fatalf("flop turn sequence too short: got %v", flopOrder)
	}
	if !reflect.DeepEqual(flopOrder[:len(flopSeating)], flopSeating) {
		t.Fatalf("flop seating order mismatch: got %v want %v", flopOrder[:len(flopSeating)], flopSeating)
	}
	if len(flopOrder) == 0 || flopOrder[0] != expectedSB {
		t.Fatalf("flop should start with small blind %s, got %v", expectedSB, flopOrder)
	}
	if countOccurrences(flopOrder, expectedSB) < 2 {
		t.Fatalf("small blind %s should act twice on flop, got %v", expectedSB, flopOrder)
	}

	turnOrder := collector.turnTimeline(poker.StageTurn, 1)
	if len(turnOrder) == 0 || turnOrder[0] != expectedSB {
		t.Fatalf("turn should start with small blind %s, got %v", expectedSB, turnOrder)
	}
	if containsStage(collector.snapshot(), poker.StageRiver) {
		riverOrder := collector.turnTimeline(poker.StageRiver, 1)
		if len(riverOrder) > 0 && riverOrder[0] != expectedSB {
			t.Fatalf("river should start with small blind %s, got %v", expectedSB, riverOrder)
		}
	}

	if updated.CurrentStage != poker.StageShowDown && updated.CurrentStage != poker.StageShowDownAllFoldExceptOne {
		t.Fatalf("expected showdown, got %s", updated.CurrentStage)
	}
}

func TestHandleTableActivitiePreFlopEndsOnBigBlind(t *testing.T) {
	srv, js, nc := startWorkflowJetStream(t)
	defer srv.Shutdown()
	defer nc.Close()

	SetJetStream(js)

	table := &poker.Table{
		ID:           "Table-NoRaise",
		TournamentID: 902,
		BBValue:      40,
		TurnTime:     2,
		CurrentSB:    "p4",
		CurrentBB:    "p1",
		Players: []poker.Player{
			{ID: "p1", Chips: 1000},
			{ID: "p2", Chips: 1000},
			{ID: "p3", Chips: 1000},
			{ID: "p4", Chips: 1000},
		},
	}
	expectedSB := nextSeatID(table.Players, table.CurrentSB)
	expectedBB := nextSeatID(table.Players, expectedSB)
	utg := nextSeatID(table.Players, expectedBB)
	preflopSeating := orderStartingFrom(table.Players, utg, len(table.Players))
	flopSeating := orderStartingFrom(table.Players, expectedSB, len(table.Players))

	script := map[string][]poker.Player{
		"p3": {
			{ID: "p3", LastAction: "call", LastBet: 40},
			{ID: "p3", LastAction: "check"},
			{ID: "p3", LastAction: "check"},
			{ID: "p3", LastAction: "check"},
		},
		"p4": {
			{ID: "p4", LastAction: "call", LastBet: 40},
			{ID: "p4", LastAction: "check"},
			{ID: "p4", LastAction: "check"},
			{ID: "p4", LastAction: "check"},
		},
		"p1": {
			{ID: "p1", LastAction: "call", LastBet: 20},
			{ID: "p1", LastAction: "check"},
			{ID: "p1", LastAction: "check"},
			{ID: "p1", LastAction: "check"},
		},
		"p2": {
			{ID: "p2", LastAction: "check"},
			{ID: "p2", LastAction: "check"},
			{ID: "p2", LastAction: "check"},
			{ID: "p2", LastAction: "check"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collector, stageWG := captureTableStages(t, ctx, js, table)
	var responders sync.WaitGroup
	responders.Add(1)
	go scriptedTurnRunner(t, ctx, js, table.TournamentID, table.ID, script, nil, &responders)

	updated, err := HandleTableActivitie(ctx, table, &config.Config{})
	if err != nil {
		t.Fatalf("HandleTableActivitie failed in no-raise test: %v", err)
	}

	cancel()
	responders.Wait()
	stageWG.Wait()

	if updated.CurrentSB != expectedSB || updated.CurrentBB != expectedBB {
		t.Fatalf("unexpected blinds after hand: got SB=%s BB=%s want SB=%s BB=%s", updated.CurrentSB, updated.CurrentBB, expectedSB, expectedBB)
	}

	preflopOrder := collector.turnTimeline(poker.StagePreFlop, 1)
	if !reflect.DeepEqual(preflopOrder, preflopSeating) {
		t.Fatalf("preflop order mismatch: got %v want %v", preflopOrder, preflopSeating)
	}
	if preflopOrder[len(preflopOrder)-1] != expectedBB {
		t.Fatalf("preflop should end on big blind %s, got %s", expectedBB, preflopOrder[len(preflopOrder)-1])
	}

	flopOrder := collector.turnTimeline(poker.StageFlop, 1)
	if len(flopOrder) == 0 || flopOrder[0] != expectedSB {
		t.Fatalf("flop should start with small blind %s, got %v", expectedSB, flopOrder)
	}
	if !reflect.DeepEqual(flopOrder, flopSeating) {
		t.Fatalf("flop order mismatch: got %v want %v", flopOrder, flopSeating)
	}

	if updated.CurrentStage != poker.StageShowDown && updated.CurrentStage != poker.StageShowDownAllFoldExceptOne {
		t.Fatalf("expected showdown, got %s", updated.CurrentStage)
	}
}

func autoPlayTurns(t *testing.T, ctx context.Context, js nats.JetStreamContext, tournamentID int, tableID string, wg *sync.WaitGroup, counter *atomic.Int32) {
	defer wg.Done()
	subject := fmt.Sprintf("pokerServer.%d.%s", tournamentID, tableID)
	sub, err := js.PullSubscribe(subject, fmt.Sprintf("autoplay-%d", time.Now().UnixNano()),
		nats.BindStream("POKER_TOURNAMENT"), nats.MaxAckPending(200))
	if err != nil {
		t.Fatalf("failed to subscribe for auto play: %v", err)
	}

	seen := make(map[string]struct{})
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgs, err := sub.Fetch(1, nats.MaxWait(250*time.Millisecond))
		if err != nil {
			if err == nats.ErrTimeout {
				continue
			}
			t.Fatalf("auto play fetch failed: %v", err)
		}

		var snapshot poker.Table
		if err := json.Unmarshal(msgs[0].Data, &snapshot); err != nil {
			t.Fatalf("auto play decode failed: %v", err)
		}
		msgs[0].Ack()
		if snapshot.CurrentTurn == "" {
			continue
		}

		key := fmt.Sprintf("%s-%s-%d-%d-%d", snapshot.CurrentTurn, snapshot.CurrentStage, snapshot.Round, snapshot.PlayerActedInRound, snapshot.TotalBet)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		var player poker.Player
		for _, candidate := range snapshot.Players {
			if candidate.ID == snapshot.CurrentTurn {
				player = candidate
				break
			}
		}
		if player.ID == "" {
			continue
		}

		action := poker.Player{ID: player.ID}
		if player.CallAmount > 0 {
			bet := player.CallAmount
			if bet > player.Chips {
				bet = player.Chips
			}
			action.LastAction = "call"
			action.LastBet = bet
		} else {
			action.LastAction = "check"
		}

		subject := fmt.Sprintf("pokerClient.%d.%s.%s", tournamentID, snapshot.ID, player.ID)
		payload, err := json.Marshal(action)
		if err != nil {
			t.Fatalf("auto play encode failed: %v", err)
		}
		if _, err := js.Publish(subject, payload); err != nil {
			t.Fatalf("auto play publish failed: %v", err)
		}
		if counter != nil {
			counter.Add(1)
		}
		t.Logf("auto action -> player=%s stage=%s call=%d round=%d", player.ID, snapshot.CurrentStage, player.CallAmount, snapshot.Round)
	}
}

func scriptedTurnRunner(t *testing.T, ctx context.Context, js nats.JetStreamContext, tournamentID int, tableID string, script map[string][]poker.Player, counters map[string]*atomic.Int32, wg *sync.WaitGroup) {
	defer wg.Done()
	subject := fmt.Sprintf("pokerServer.%d.%s", tournamentID, tableID)
	sub, err := js.PullSubscribe(subject, fmt.Sprintf("scripted-%d", time.Now().UnixNano()),
		nats.BindStream("POKER_TOURNAMENT"), nats.MaxAckPending(200))
	if err != nil {
		t.Fatalf("failed to subscribe scripted runner: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgs, err := sub.Fetch(1, nats.MaxWait(250*time.Millisecond))
		if err != nil {
			if err == nats.ErrTimeout {
				continue
			}
			t.Fatalf("scripted runner fetch failed: %v", err)
		}

		var snapshot poker.Table
		if err := json.Unmarshal(msgs[0].Data, &snapshot); err != nil {
			t.Fatalf("scripted runner decode failed: %v", err)
		}
		msgs[0].Ack()

		turn := snapshot.CurrentTurn
		if turn == "" {
			continue
		}
		queue := script[turn]
		if len(queue) == 0 {
			continue
		}
		action := queue[0]
		script[turn] = queue[1:]
		if counter, ok := counters[turn]; ok {
			counter.Add(1)
		}
		if action.ID == "" {
			action.ID = turn
		}
		payload, err := json.Marshal(action)
		if err != nil {
			t.Fatalf("scripted runner encode failed: %v", err)
		}
		clientSubject := fmt.Sprintf("pokerClient.%d.%s.%s", tournamentID, snapshot.ID, turn)
		if _, err := js.Publish(clientSubject, payload); err != nil {
			t.Fatalf("scripted runner publish failed: %v", err)
		}
	}
}

func captureTableStages(t *testing.T, ctx context.Context, js nats.JetStreamContext, table *poker.Table) (*stageCollector, *sync.WaitGroup) {
	subject := fmt.Sprintf("pokerServer.%d.%s", table.TournamentID, table.ID)
	sub, err := js.PullSubscribe(subject, fmt.Sprintf("stage-tracker-%d", time.Now().UnixNano()),
		nats.BindStream("POKER_TOURNAMENT"), nats.MaxAckPending(100))
	if err != nil {
		t.Fatalf("failed to subscribe to table updates: %v", err)
	}

	collector := &stageCollector{stages: make([]string, 0, 16)}
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			msgs, err := sub.Fetch(1, nats.MaxWait(250*time.Millisecond))
			if err != nil {
				if err == nats.ErrTimeout {
					continue
				}
				t.Fatalf("failed to fetch stage update: %v", err)
			}
			var snapshot poker.Table
			if err := json.Unmarshal(msgs[0].Data, &snapshot); err != nil {
				t.Fatalf("failed to decode stage update: %v", err)
			}
			msgs[0].Ack()
			collector.add(snapshot.CurrentStage, snapshot)
		}
	}()

	return collector, &wg
}

func containsStagesInOrder(sequence []string, expected []string) bool {
	if len(sequence) == 0 {
		return false
	}
	idx := 0
	for _, stage := range sequence {
		if idx >= len(expected) {
			break
		}
		if stage == expected[idx] {
			idx++
		}
	}
	return idx == len(expected)
}

func containsStage(sequence []string, target string) bool {
	for _, stage := range sequence {
		if stage == target {
			return true
		}
	}
	return false
}

func collapseSequentialStages(sequence []string) []string {
	if len(sequence) == 0 {
		return nil
	}
	collapsed := make([]string, 0, len(sequence))
	last := ""
	for _, stage := range sequence {
		if stage == "" {
			continue
		}
		if stage == last {
			continue
		}
		collapsed = append(collapsed, stage)
		last = stage
	}
	return collapsed
}

func computeNextBlinds(players []poker.Player, currentSB string) (string, string) {
	sbIndex := -1
	for i, p := range players {
		if p.ID == currentSB {
			sbIndex = i
			break
		}
	}
	if sbIndex == -1 {
		sbIndex = 0
	}
	newSBIndex := nextActiveIndex(players, (sbIndex+1)%len(players))
	if newSBIndex == -1 {
		return "", ""
	}
	newBBIndex := nextActiveIndex(players, (newSBIndex+1)%len(players))
	if newBBIndex == -1 {
		return "", ""
	}
	return players[newSBIndex].ID, players[newBBIndex].ID
}

func nextActiveIndex(players []poker.Player, start int) int {
	if len(players) == 0 {
		return -1
	}
	for offset := 0; offset < len(players); offset++ {
		idx := (start + offset) % len(players)
		if !players[idx].IsEliminated {
			return idx
		}
	}
	return -1
}

type stageCollector struct {
	mu        sync.Mutex
	stages    []string
	snapshots []poker.Table
}

func (s *stageCollector) add(stage string, snapshot poker.Table) {
	s.mu.Lock()
	s.stages = append(s.stages, stage)
	s.snapshots = append(s.snapshots, snapshot)
	s.mu.Unlock()
}

func (s *stageCollector) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.stages))
	copy(out, s.stages)
	return out
}

func (s *stageCollector) latestTable() *poker.Table {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.snapshots) == 0 {
		return nil
	}
	tbl := s.snapshots[len(s.snapshots)-1]
	return &tbl
}

func (s *stageCollector) turnTimeline(stage string, round int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.snapshots) == 0 {
		return nil
	}
	sequence := make([]string, 0, 8)
	last := ""
	for _, snap := range s.snapshots {
		if snap.CurrentStage != stage {
			continue
		}
		if round > 0 && snap.Round != round {
			continue
		}
		if snap.CurrentTurn == "" {
			continue
		}
		if snap.CurrentTurn == last {
			continue
		}
		sequence = append(sequence, snap.CurrentTurn)
		last = snap.CurrentTurn
	}
	return sequence
}

func countOccurrences(sequence []string, target string) int {
	count := 0
	for _, entry := range sequence {
		if entry == target {
			count++
		}
	}
	return count
}

func nextSeatID(players []poker.Player, current string) string {
	if len(players) == 0 {
		return ""
	}
	idx := -1
	for i, p := range players {
		if p.ID == current {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ""
	}
	for offset := 1; offset <= len(players); offset++ {
		candidate := players[(idx+offset)%len(players)]
		if candidate.IsEliminated {
			continue
		}
		return candidate.ID
	}
	return ""
}

func orderStartingFrom(players []poker.Player, start string, count int) []string {
	if len(players) == 0 || count <= 0 {
		return nil
	}
	idx := -1
	for i, p := range players {
		if p.ID == start {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil
	}
	sequence := make([]string, 0, count)
	offset := 0
	for len(sequence) < count && offset < len(players)*count {
		candidate := players[(idx+offset)%len(players)]
		offset++
		if candidate.IsEliminated {
			continue
		}
		sequence = append(sequence, candidate.ID)
	}
	return sequence
}
