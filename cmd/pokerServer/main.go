package main

import (
	"context"
	"log"
	"server/config"
	"server/internal/db"
	"server/internal/nats"
	"server/internal/poker"
	internal "server/internal/server"
	"server/internal/server/routes"
	temporal "server/internal/workflow"

	"go.temporal.io/sdk/client"
	"gorm.io/gorm"

	natsG "github.com/nats-io/nats.go"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	db.InitDB(&cfg.Database)
	err = db.Migrate()
	if err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	natsConn, js, err := nats.Connect(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer natsConn.Close()

	err = nats.ConfigureStream(js, &cfg.NATS.Stream)
	if err != nil {
		log.Fatalf("Failed to configure JetStream: %v", err)
	}

	temporalOptions := client.Options{
		HostPort: cfg.Temporal.HostPort,
	}

	c, err := client.Dial(temporalOptions)
	if err != nil {
		log.Fatalf("Failed to create Temporal client: %v", err)
	}
	defer c.Close()

	temporal.StartWorker(cfg)

	//w := worker.New(c, "poker-task-queue", worker.Options{})
	//w.RegisterWorkflowWithOptions(temporal.TableWorkflow, workflow.RegisterOptions{Name: "TableWorkflow"})
	//w.RegisterWorkflow(PlayerWorkflow)
	//w.RegisterWorkflow(TableWorkflow)
	//w.RegisterWorkflow(TournamentWorkflow)
	//w.RegisterWorkflow(RoundWorkflow)
	//w.RegisterWorkflow(TournamentControllerWorkflow)
	//w.RegisterActivity(DealPreFlop)
	//w.RegisterActivity(DealCardsActivity)
	//w.RegisterActivity(DealFlop)
	//w.RegisterActivity(DealTurn)
	//w.RegisterActivity(DealRiver)
	//w.RegisterActivity(ShowDown)
	//w.RegisterActivity(ShowDownAllFoldExecptOne)
	//w.RegisterActivity(CheckLastTable)
	//w.RegisterActivity(Reshuffle)
	//w.RegisterActivity(CreateTablesInTournament)

	//err = w.Run(worker.InterruptCh())
	//if err != nil {
	//	log.Fatalf("Failed to start worker: %v", err)
	//}

	// runTestWorkflows(c, db.GetDB(), js)

	server := internal.NewServer(&cfg.Server)

	routes.DefineRoutes(server.Router, c, cfg)

	server.Start()
}

func runTestWorkflows(c client.Client, db *gorm.DB, js natsG.JetStreamContext) {
	// Define dos tablas con diferentes jugadores
	table1 := poker.Table{
		CurrentBB:          "player1",
		CurrentSB:          "player2",
		CurrentTurn:        "player3",
		NextTurn:           "player4",
		CurrentStage:       "dealing",
		FlopCards:          []poker.Card{},
		TurnCard:           nil,
		RiverCard:          nil,
		TotalBetIndividual: make(map[string]int),
	}

	table2 := poker.Table{
		CurrentBB:          "player6",
		CurrentSB:          "player7",
		CurrentTurn:        "player8",
		NextTurn:           "player9",
		CurrentStage:       "dealing",
		FlopCards:          []poker.Card{},
		TurnCard:           nil,
		RiverCard:          nil,
		TotalBetIndividual: make(map[string]int),
	}

	// Crea y ejecuta dos workflows
	workflowID1 := "test-workflow-1"
	workflowID2 := "test-workflow-2"

	_, err := c.ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{
		ID:        workflowID1,
		TaskQueue: "poker-task-queue",
	}, "TableWorkflow", table1, db, js)
	if err != nil {
		log.Fatalf("Failed to start workflow 1: %v", err)
	}

	_, err = c.ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{
		ID:        workflowID2,
		TaskQueue: "poker-task-queue",
	}, "TableWorkflow", table2, db, js)
	if err != nil {
		log.Fatalf("Failed to start workflow 2: %v", err)
	}
}
