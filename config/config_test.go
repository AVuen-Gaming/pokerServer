package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	assert.NotNil(t, cfg, "Config should not be nil")
	assert.NotNil(t, cfg.Database, "Database config should not be nil")
}
