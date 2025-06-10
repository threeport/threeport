package handlers

import (
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Handler struct {
	DB     *gorm.DB
	NC     *nats.Conn
	JS     nats.JetStreamContext
	Logger *zap.Logger
}

func New(db *gorm.DB, nc *nats.Conn, rc nats.JetStreamContext, logger *zap.Logger) Handler {
	return Handler{db, nc, rc, logger}
}
