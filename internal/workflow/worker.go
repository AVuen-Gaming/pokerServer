package temporal

import (
	"log"
	"server/config"
	"server/internal/db"
	"sync"

	"github.com/nats-io/nats.go"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"gorm.io/gorm"
)

var (
	jetStreamInstance nats.JetStreamContext
	dbInstance        *gorm.DB
	once              sync.Once
)

func StartWorker(cfg *config.Config) {
	once.Do(func() {
		natsConn, err := nats.Connect(cfg.NATS.Host)
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
	})

	c, err := client.Dial(client.Options{
		HostPort: cfg.Temporal.HostPort,
	})
	if err != nil {
		log.Fatalf("Failed to create Temporal client: %v", err)
	}

	w := worker.New(c, "poker-task-queue", worker.Options{})
	w.RegisterWorkflow(TableWorkflow)
	w.RegisterActivity(DealPreFlop)
	w.RegisterActivity(DealCardsActivity)
	w.RegisterActivity(DealFlop)
	w.RegisterActivity(DealTurn)
	w.RegisterActivity(DealRiver)
	w.RegisterActivity(ShowDown)
	w.RegisterActivity(ShowDownAllFoldExecptOne)

	go func() {
		if err := w.Run(worker.InterruptCh()); err != nil {
			log.Fatalf("Failed to start worker: %v", err)
		}
	}()
}

func GetJetStream() nats.JetStreamContext {
	return jetStreamInstance
}

func GetDB() *gorm.DB {
	return dbInstance
}
