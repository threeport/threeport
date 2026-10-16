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
	if err := alignLockBucketTTL(js, kv, config); err != nil {
		return nil, err
	}

	return kv, nil
}

// alignLockBucketTTL brings an existing bucket's expiry to the configured
// one, and leaves everything else about the stream alone.
func alignLockBucketTTL(js nats.JetStreamContext, kv nats.KeyValue, config *nats.KeyValueConfig) error {
	streamName := fmt.Sprintf("KV_%s", config.Bucket)

	info, err := js.StreamInfo(streamName)
	if err != nil {
		return fmt.Errorf("failed to read the lock bucket's stream %s: %w", streamName, err)
	}
	if info.Config.MaxAge == config.TTL {
		return nil
	}

	// Shortening the expiry is only safe when nothing is locked.
	//
	// The bucket is shared by every replica of a controller, and during a
	// rolling upgrade that includes pods running the version before this one
	// - which do not refresh what they hold. Shortening the expiry underneath
	// one of those takes its lock away mid-reconcile, and a second worker
	// picks up the same object; worse, the first one's release deletes by key
	// and would take the second one's lock with it.
	//
	// So a shortening waits for a moment when the bucket is empty. Every
	// controller start tries again, and the locks held during an upgrade are
	// transient, so that moment arrives on its own. Lengthening is always
	// safe and is not held back.
	if config.TTL < info.Config.MaxAge {
		held, err := kv.Keys()
		if err != nil && !errors.Is(err, nats.ErrNoKeysFound) {
			return fmt.Errorf("failed to check the lock bucket %s for held locks: %w", config.Bucket, err)
		}
		if len(held) > 0 {
			// not an error: the next controller to start will find the bucket
			// empty and shorten it then
			return nil
		}
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
