package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"server/config"
	"server/internal/poker"

	"github.com/nats-io/nats.go"
)

func TestWorkflowSimulationRunsTablesConcurrently(t *testing.T) {
	cfg := &config.Config{}
	tables := []poker.Table{
		testSimTable("40"),
		testSimTable("41"),
	}

	var inFlight int32
	var maxConcurrent int32

	sim := &workflowSimulation{
		cfg:    cfg,
		tables: tables,
		bots:   []*botEngine{},
		rng:    nil,
		handleTable: func(ctx context.Context, table *poker.Table, cfg *config.Config) (*poker.Table, error) {
			current := atomic.AddInt32(&inFlight, 1)
			updateMaxInt32(&maxConcurrent, current)
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&inFlight, -1)
			table.Round++
			return table, nil
		},
		checkTournament: func(tables []poker.Table, js nats.JetStreamContext) (bool, error) {
			return false, nil
		},
		reshuffle: func(tables []poker.Table, updated poker.Table, js nats.JetStreamContext) []poker.Table {
			for i := range tables {
				if tables[i].ID == updated.ID {
					tables[i] = updated
					break
				}
			}
			return tables
		},
	}

	finished, err := sim.playHand(context.Background(), 1)
	if err != nil {
		t.Fatalf("playHand returned error: %v", err)
	}
	if finished {
		t.Fatalf("expected tournament to continue after single hand")
	}
	if atomic.LoadInt32(&maxConcurrent) < 2 {
		t.Fatalf("expected concurrent table execution, max=%d", maxConcurrent)
	}
}

func updateMaxInt32(target *int32, candidate int32) {
	for {
		current := atomic.LoadInt32(target)
		if candidate <= current {
			return
		}
		if atomic.CompareAndSwapInt32(target, current, candidate) {
			return
		}
	}
}

func testSimTable(id string) poker.Table {
	return poker.Table{
		ID:           id,
		Players:      []poker.Player{{ID: id + "-p1"}, {ID: id + "-p2"}},
		TurnTime:     1,
		BBValue:      50,
		MinPlayers:   2,
		MaxPlayers:   9,
		TournamentID: 1,
	}
}
