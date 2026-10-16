package controller

import (
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
)

// CreateLockBucketIfNotExists binds to the existing KeyValue store if it has been
// created.  If not created it will be created and the KeyValue store returned.
func CreateLockBucketIfNotExists(js nats.JetStreamContext, config *nats.KeyValueConfig) (nats.KeyValue, error) {
	kv, err := js.KeyValue(config.Bucket)
	if err != nil {
		if errors.Is(err, nats.ErrBucketNotFound) {
			return js.CreateKeyValue(config)
		}

		return nil, err
	}

	// An existing bucket keeps whatever configuration it was created with.
	// Creating it again does nothing, so a control plane that was installed
	// before this TTL changed would go on using the old one - the change
	// would only ever reach a fresh install, which is not where the long
	// expiry hurts.
	//
	// A key-value bucket is a stream named KV_<bucket>, and its TTL is that
	// stream's MaxAge, so the alignment goes through the stream.
	if err := alignLockBucketTTL(js, config); err != nil {
		return nil, err
	}

	return kv, nil
}

// alignLockBucketTTL brings an existing bucket's expiry to the configured
// one, and leaves everything else about the stream alone.
func alignLockBucketTTL(js nats.JetStreamContext, config *nats.KeyValueConfig) error {
	streamName := fmt.Sprintf("KV_%s", config.Bucket)

	info, err := js.StreamInfo(streamName)
	if err != nil {
		return fmt.Errorf("failed to read the lock bucket's stream %s: %w", streamName, err)
	}
	if info.Config.MaxAge == config.TTL {
		return nil
	}

	updated := info.Config
	updated.MaxAge = config.TTL
	if _, err := js.UpdateStream(&updated); err != nil {
		return fmt.Errorf(
			"failed to change the lock bucket's expiry from %s to %s: %w",
			info.Config.MaxAge, config.TTL, err,
		)
	}

	return nil
}
