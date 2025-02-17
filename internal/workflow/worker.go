package temporal

import (
	"fmt"
	"log"
	"server/config"
	"server/internal/db"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/worker"
	"gorm.io/gorm"
)

var (
	jetStreamInstance nats.JetStreamContext
	dbInstance        *gorm.DB
	temporalInstance  client.Client
	once              sync.Once
)

func StartWorker(cfg *config.Config, dataConverter converter.DataConverter) {
	once.Do(func() {
		address := fmt.Sprintf("nats://%s:%s@%s:%d", cfg.NATS.Username, cfg.NATS.Password, cfg.NATS.Host, cfg.NATS.Port)
		natsConn, err := nats.Connect(address)
		if err != nil {
			log.Fatalf("Failed to connect to NATS: %v", err)
		}

		js, err := natsConn.JetStream()
		if err != nil {
			log.Fatalf("Failed to create JetStream context: %v", err)
		}
		jetStreamInstance = js

		dbConn, err := db.InitDB(&cfg.Database)
		if err != nil {
			log.Fatalf("Failed to initialize database: %v", err)
		}
		dbInstance = dbConn

		temporalOptions := client.Options{
			HostPort: cfg.Temporal.HostPort,
			ConnectionOptions: client.ConnectionOptions{
				MaxPayloadSize: 64 * 1024 * 1024,
				KeepAliveTime:  30000 * time.Second,
			},
		}

		c, err := client.Dial(temporalOptions)
		if err != nil {
			log.Fatalf("Failed to create Temporal client: %v", err)
		}
		temporalInstance = c
		defer c.Close()
	})

	temporalOptions := client.Options{
		HostPort: cfg.Temporal.HostPort,
		ConnectionOptions: client.ConnectionOptions{
			MaxPayloadSize: 16 * 1024 * 1024,
			KeepAliveTime:  30 * time.Second,
		},
	}

	c, err := client.Dial(temporalOptions)
	if err != nil {
		log.Fatalf("Failed to create Temporal client: %v", err)
	}

	w := worker.New(c, "poker-task-queue", worker.Options{
		MaxConcurrentActivityTaskPollers:   1000,
		MaxConcurrentWorkflowTaskPollers:   1000,
		MaxConcurrentActivityExecutionSize: 10000,

		//MaxConcurrentWorkflowTaskExecutionSize: 100,
		//WorkerActivitiesPerSecond: 100,
		//TaskQueueActivitiesPerSecond: 100,
	})
	w.RegisterWorkflow(PlayerWorkflow)
	w.RegisterWorkflow(TableWorkflow)
	w.RegisterWorkflow(TournamentWorkflow)
	w.RegisterWorkflow(RoundWorkflow)
	w.RegisterWorkflow(TournamentControllerWorkflow)
	w.RegisterActivity(DealPreFlop)
	w.RegisterActivity(CheckLastTable)
	w.RegisterActivity(CreateTablesInTournament)
	w.RegisterActivity(HandleTurns)
	w.RegisterActivity(CreatePrizePool)
	w.RegisterActivity(DeleteWorkflowExecutionActivity)
	w.RegisterActivity(HandleTableActivitie)

	go func() {
		if err := w.Run(worker.InterruptCh()); err != nil {
			log.Fatalf("Failed to start worker: %v", err)
		}
	}()
}

func GetJetStream() nats.JetStreamContext {
	return jetStreamInstance
}

func GetTemporalClient() client.Client {
	return temporalInstance
}

func GetDB() *gorm.DB {
	return dbInstance
}
