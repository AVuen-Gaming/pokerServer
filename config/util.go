package config

import (
	"context"

	"github.com/nats-io/nats.go"
	"gorm.io/gorm"
)

type contextKey string

const (
	jsContextKey = contextKey("jsContext")
	dbContextKey = contextKey("dbContext")
)

func (cfg *Config) WithJetStream(ctx context.Context, js nats.JetStreamContext) context.Context {
	return context.WithValue(ctx, jsContextKey, js)
}

func (cfg *Config) JetStreamFromContext(ctx context.Context) (nats.JetStreamContext, bool) {
	js, ok := ctx.Value(jsContextKey).(nats.JetStreamContext)
	return js, ok
}

func (cfg *Config) WithDB(ctx context.Context, db *gorm.DB) context.Context {
	return context.WithValue(ctx, dbContextKey, db)
}

func (cfg *Config) DBFromContext(ctx context.Context) (*gorm.DB, bool) {
	db, ok := ctx.Value(dbContextKey).(*gorm.DB)
	return db, ok
}
