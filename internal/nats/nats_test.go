package nats

import (
	"server/config"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConnectAndCreateStream(t *testing.T) {
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	nc, js, err := Connect(cfg)
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	assert.NotNil(t, nc, "NATS connection should not be nil")
	assert.NotNil(t, js, "NATS JS connection should not be nil")

	err = ConfigureStream(js, &cfg.NATS.Stream)
	if err != nil {
		t.Fatalf("Failed to configure JetStream: %v", err)
	}

	streamInfo, err := js.StreamInfo(cfg.NATS.Stream.Name)
	if err != nil {
		t.Fatalf("Failed to get stream info: %v", err)
	}

	assert.Equal(t, cfg.NATS.Stream.Name, streamInfo.Config.Name, "Stream name should match")
	assert.ElementsMatch(t, cfg.NATS.Stream.Subjects, streamInfo.Config.Subjects, "Stream subjects should match")
}
