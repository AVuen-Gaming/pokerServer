package temporal

import (
	"fmt"
	"sync"
	"testing"

	"server/config"
	"server/internal/poker"

	"github.com/nats-io/nats.go"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

func TestTournamentWorkflowPropagatesTableState(t *testing.T) {
	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	script := map[string][]int{
		"40": {3, 2, 1},
		"41": {3, 3},
	}
	observed := map[string][]int{}
	var obsMu sync.Mutex

	mockPlayerWorkflow := func(ctx workflow.Context, table poker.Table, cfg *config.Config) (poker.Table, error) {
		seq := script[table.ID]
		roundIdx := table.Round
		if roundIdx >= len(seq) {
			roundIdx = len(seq) - 1
		}
		expected := seq[roundIdx]
		if len(table.Players) != expected {
			return table, fmt.Errorf("table %s round %d expected %d players, got %d", table.ID, table.Round, expected, len(table.Players))
		}

		obsMu.Lock()
		observed[table.ID] = append(observed[table.ID], len(table.Players))
		obsMu.Unlock()

		table.Round++
		if roundIdx+1 < len(seq) {
			nextCount := seq[roundIdx+1]
			table.Players = clonePlayersForTest(table.Players, nextCount)
		}
		return table, nil
	}

	mockReshuffle := func(tables []poker.Table, updated poker.Table, js nats.JetStreamContext) []poker.Table {
		replaced := false
		for i := range tables {
			if tables[i].ID == updated.ID {
				tables[i] = updated
				replaced = true
				break
			}
		}
		if !replaced {
			tables = append(tables, updated)
		}
		return tables
	}

	mockCheckLastTable := func(tables []poker.Table, js nats.JetStreamContext) (bool, error) {
		for _, tbl := range tables {
			seq := script[tbl.ID]
			finalCount := seq[len(seq)-1]
			if len(tbl.Players) != finalCount {
				return false, nil
			}
		}
		return true, nil
	}

	originalPlayerFn := playerWorkflowFn
	originalReshuffleFn := reshuffleTablesFn
	originalCheckFn := checkLastTableFn
	defer func() {
		playerWorkflowFn = originalPlayerFn
		reshuffleTablesFn = originalReshuffleFn
		checkLastTableFn = originalCheckFn
	}()

	playerWorkflowFn = mockPlayerWorkflow
	reshuffleTablesFn = mockReshuffle
	checkLastTableFn = mockCheckLastTable

	env.RegisterWorkflow(TournamentWorkflow)
	env.RegisterWorkflow(mockPlayerWorkflow)

	tables := []poker.Table{
		makeWorkflowTestTable("40", 3),
		makeWorkflowTestTable("41", 3),
	}

	cfg := &config.Config{}
	env.ExecuteWorkflow(TournamentWorkflow, tables, cfg)

	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow errored: %v", err)
	}

	var result []poker.Table
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatalf("failed to decode result: %v", err)
	}

	if len(result) != len(tables) {
		t.Fatalf("expected %d tables, got %d", len(tables), len(result))
	}

	for _, tbl := range result {
		finalCount := script[tbl.ID][len(script[tbl.ID])-1]
		if len(tbl.Players) != finalCount {
			t.Fatalf("table %s should have %d players, got %d", tbl.ID, finalCount, len(tbl.Players))
		}
	}

	for id, seq := range script {
		got := observed[id]
		prefixLen := len(seq) - 1
		if prefixLen < 0 {
			prefixLen = 0
		}
		if len(got) < prefixLen {
			t.Fatalf("table %s recorded insufficient rounds: got %v, want prefix %v", id, got, seq[:prefixLen])
		}
		for idx := 0; idx < prefixLen; idx++ {
			if got[idx] != seq[idx] {
				t.Fatalf("table %s round %d expected %d players, got %d", id, idx, seq[idx], got[idx])
			}
		}
	}
}

func makeWorkflowTestTable(id string, players int) poker.Table {
	table := poker.Table{ID: id, TournamentID: 99}
	table.Players = make([]poker.Player, players)
	for i := 0; i < players; i++ {
		table.Players[i] = poker.Player{ID: fmt.Sprintf("%s-p%d", id, i+1), Chips: 1000}
	}
	return table
}

func clonePlayersForTest(players []poker.Player, keep int) []poker.Player {
	if keep <= 0 {
		return nil
	}
	if keep > len(players) {
		keep = len(players)
	}
	cloned := make([]poker.Player, keep)
	copy(cloned, players[:keep])
	return cloned
}
