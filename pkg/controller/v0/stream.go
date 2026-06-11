package controller

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/nats-io/nats.go"
)

const (
	// streamRetryInterval is the sleep duration between stream lookup
	// attempts.
	streamRetryInterval = 2 * time.Second

	// streamMaxRetries caps the wait at ~2 minutes total. Each retry
	// is one StreamInfo() round-trip so the cost is dominated by the
	// sleep, not the lookup.
	streamMaxRetries = 60
)

// WaitForStream blocks until the named JetStream stream exists,
// retrying every 2 seconds for up to ~2 minutes. Exits with code 1
// after the retry budget so a stuck pod can restart.
func WaitForStream(js nats.JetStreamContext, streamName string, log logr.Logger) {
	log.Info("waiting for stream", "streamName", streamName)

	for i := 0; i < streamMaxRetries; i++ {
		_, err := js.StreamInfo(streamName)
		if err == nil {
			log.Info("stream is available", "streamName", streamName)
			return
		}
		if !errors.Is(err, nats.ErrStreamNotFound) {
			log.Info("stream lookup error, retrying", "err", err)
		}
		time.Sleep(streamRetryInterval)
	}

	log.Error(
		fmt.Errorf("stream not found after %d retries", streamMaxRetries),
		"failed to find stream",
		"streamName", streamName,
	)
	os.Exit(1)
}
