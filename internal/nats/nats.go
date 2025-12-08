package nats

import (
	"errors"
	"fmt"
	"server/config"
	"time"

	"github.com/nats-io/nats.go"
)

func Connect(cfg *config.Config) (*nats.Conn, nats.JetStreamContext, error) {
	address := fmt.Sprintf("nats://%s:%s@%s:%d", cfg.NATS.Username, cfg.NATS.Password, cfg.NATS.Host, cfg.NATS.Port)
	opts := []nats.Option{
		nats.Name("PokerServer"),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			fmt.Println("Reconnected to NATS!")
		}),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			fmt.Printf("Disconnected from NATS: %v\n", err)
		}),
	}

	nc, err := nats.Connect(address, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	_, err = js.AddStream(&nats.StreamConfig{
		Name:      "POKER_TOURNAMENT",
		Subjects:  []string{"pokerServer.>", "pokerClient.>"},
		Retention: nats.LimitsPolicy,
		MaxAge:    1 * time.Hour,
	})
	if err != nil && !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
		return nil, nil, fmt.Errorf("failed to add stream: %w", err)
	}

	return nc, js, nil
}

func ConfigureStream(js nats.JetStreamContext, streamCfg *config.StreamConfig) error {
	_, err := js.AddStream(&nats.StreamConfig{
		Name:      streamCfg.Name,
		Subjects:  streamCfg.Subjects,
		Retention: nats.LimitsPolicy,
		MaxAge:    1 * time.Hour,
	})
	if err != nil {
		return fmt.Errorf("failed to add stream: %w", err)
	}
	return nil
}
