package nats

import (
	"fmt"
	"server/config"

	"github.com/nats-io/nats.go"
)

func Connect(cfg *config.Config) (*nats.Conn, nats.JetStreamContext, error) {
	address := fmt.Sprintf("%s:%d", cfg.NATS.Host, cfg.NATS.Port)
	nc, err := nats.Connect(address)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	return nc, js, nil
}

func ConfigureStream(js nats.JetStreamContext, streamCfg *config.StreamConfig) error {
	_, err := js.AddStream(&nats.StreamConfig{
		Name:     streamCfg.Name,
		Subjects: streamCfg.Subjects,
	})
	if err != nil {
		return fmt.Errorf("failed to add stream: %w", err)
	}
	return nil
}
