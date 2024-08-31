package temporal

import (
	"context"
	"log"
	"testing"
	"time"

	"server/config"
	"server/internal/poker"

	"github.com/nats-io/nats.go"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func TestTableWorkflow(t *testing.T) {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	StartWorker(cfg)

	temporalOptions := client.Options{
		HostPort: cfg.Temporal.HostPort,
	}

	c, err := client.Dial(temporalOptions)
	if err != nil {
		log.Fatalf("Failed to create Temporal client: %v", err)
	}
	defer c.Close()

	natsConn, err := nats.Connect(cfg.NATS.Host)
	if err != nil {
		log.Fatalf("Failed to create Temporal client: %v", err)
	}
	defer natsConn.Close()

	js, err := natsConn.JetStream()
	if err != nil {
		log.Fatalf("Failed to create Temporal client: %v", err)
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:      "POKER_TOURNAMENT",
		Subjects:  []string{"pokerServer.tournament.>", "pokerClient.tournament.>"},
		Retention: nats.WorkQueuePolicy, // Usar política de WorkQueue para asegurar que cada mensaje sea procesado solo una vez.
	})
	workerOptions := worker.Options{}
	w := worker.New(c, "poker-task-queue", workerOptions)
	w.RegisterWorkflow(TableWorkflow)
	w.RegisterActivity(DealPreFlop)
	w.RegisterActivity(DealCardsActivity)
	w.RegisterActivity(DealFlop)
	w.RegisterActivity(DealTurn)
	w.RegisterActivity(DealRiver)
	w.RegisterActivity(HandleTurns)
	w.RegisterActivity(ShowDown)
	w.RegisterActivity(ShowDownAllFoldExecptOne)
	// Start worker
	go func() {
		if err := w.Run(worker.InterruptCh()); err != nil {
			log.Fatalf("Failed to start worker: %v", err)
		}
	}()

	// Wait for worker to be ready
	time.Sleep(2 * time.Second) // Adjust sleep duration if needed

	// Define test tables
	table1 := poker.Table{
		ID:                 "1",
		CurrentBB:          "player2",
		CurrentSB:          "player1",
		CurrentTurn:        "player8",
		NextTurn:           "player9",
		CurrentStage:       "dealing",
		BBValue:            100,
		TurnTime:           20,
		FlopCards:          []poker.Card{},
		TurnCard:           nil,
		RiverCard:          nil,
		TotalBetIndividual: make(map[string]int),
		Players: []poker.Player{
			{ID: "player1", Chips: 1000},
			{ID: "player2", Chips: 1000},
		},
	}

	// Start workflows
	workflowID1 := "table-workflow-799"

	_, err = c.ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{
		ID:        workflowID1,
		TaskQueue: "poker-task-queue",
	}, TableWorkflow, table1, cfg)
	if err != nil {
		t.Fatalf("Failed to start workflow 1: %v", err)
	}

	// Increase timeout to ensure the workflows have enough time to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Hour) // Increased timeout
	defer cancel()

	// Check results of workflows
	err = c.GetWorkflow(ctx, workflowID1, "").Get(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to get workflow 1 result: %v", err)
	}

	c.Close()
}
