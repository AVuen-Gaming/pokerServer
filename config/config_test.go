package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func withTestEnv(t *testing.T) func() {
	t.Helper()
	content := "" +
		"DB_HOST=localhost\n" +
		"DB_PORT=5432\n" +
		"DB_USER=test\n" +
		"DB_PASSWORD=test\n" +
		"DB_NAME=test\n" +
		"DB_SSLMODE=disable\n" +
		"NATS_HOST=localhost\n" +
		"NATS_PORT=4222\n" +
		"NATS_USERNAME=user\n" +
		"NATS_PASSWORD=pass\n" +
		"NATS_STREAM_NAME=TEST_STREAM\n" +
		"NATS_SUBJECTS=pokerServer.>,pokerClient.>\n" +
		"SERVER_PORT=8080\n" +
		"SERVER_ALLOWED_ORIGIN=*\n" +
		"STATIC_TOKEN=test\n" +
		"Sign=secret\n" +
		"HTTP_ONLY=false\n" +
		"SECURE=false\n" +
		"Wallet=0xReceiver\n" +
		"SepApiKey=sep\n" +
		"BcsApiKey=bcs\n" +
		"SepoliaPrivateKey=0xabc\n" +
		"BNBPrivateKey=0xdef\n" +
		"TEMPORAL_HOSTPORT=localhost:7233\n"

	original, err := os.ReadFile(".env")
	backupExists := err == nil
	if err := os.WriteFile(".env", []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp .env: %v", err)
	}

	return func() {
		if backupExists {
			_ = os.WriteFile(".env", original, 0644)
		} else {
			_ = os.Remove(".env")
		}
	}
}

func TestLoadConfig(t *testing.T) {
	cleanup := withTestEnv(t)
	defer cleanup()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	assert.NotNil(t, cfg, "Config should not be nil")
	assert.NotNil(t, cfg.Database, "Database config should not be nil")
}
