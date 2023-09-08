package handlers

import (
	"github.com/nats-io/nats.go"
	"gorm.io/gorm"
)

type Handler struct {
	DB *gorm.DB
	NC *nats.Conn
	JS nats.JetStreamContext
}

func New(db *gorm.DB, nc *nats.Conn, rc nats.JetStreamContext) Handler {
	return Handler{db, nc, rc}
}
