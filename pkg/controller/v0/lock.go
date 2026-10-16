package controller

import (
	"errors"

	"github.com/nats-io/nats.go"
)

// CreateLockBucketIfNotExists binds to the existing KeyValue store if it has been
// created.  If not created it will be created and the KeyValue store returned.
func CreateLockBucketIfNotExists(js nats.KeyValueManager, config *nats.KeyValueConfig) (nats.KeyValue, error) {
	var kv nats.KeyValue
	kv, err := js.KeyValue(config.Bucket)
	if err != nil {
		if errors.Is(err, nats.ErrBucketNotFound) {
			kv, err = js.CreateKeyValue(config)
			if err != nil {
				return kv, err
			}

			return kv, nil
		}
	}

	return kv, nil
}
