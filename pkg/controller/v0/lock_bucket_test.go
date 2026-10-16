package controller

import (
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeJetStream answers the two calls the TTL alignment makes. Everything
// else panics rather than returning a zero value.
type fakeJetStream struct {
	nats.JetStreamContext

	maxAge     time.Duration
	infoErr    error
	updateErr  error
	updatedTo  time.Duration
	updates    int
	infoCalled int
}

func (j *fakeJetStream) StreamInfo(name string, opts ...nats.JSOpt) (*nats.StreamInfo, error) {
	j.infoCalled++
	if j.infoErr != nil {
		return nil, j.infoErr
	}

	return &nats.StreamInfo{Config: nats.StreamConfig{Name: name, MaxAge: j.maxAge}}, nil
}

func (j *fakeJetStream) UpdateStream(cfg *nats.StreamConfig, opts ...nats.JSOpt) (*nats.StreamInfo, error) {
	j.updates++
	if j.updateErr != nil {
		return nil, j.updateErr
	}
	j.updatedTo = cfg.MaxAge

	return &nats.StreamInfo{Config: *cfg}, nil
}

// bucketWithKeys is a key-value store holding the given lock keys.
type bucketWithKeys struct {
	nats.KeyValue

	keys    []string
	keysErr error
}

func (b *bucketWithKeys) Keys(opts ...nats.WatchOpt) ([]string, error) {
	if b.keysErr != nil {
		return nil, b.keysErr
	}
	if len(b.keys) == 0 {
		return nil, nats.ErrNoKeysFound
	}

	return b.keys, nil
}

// TestShorteningTheLockExpiryWaitsForAnEmptyBucket covers the window a
// rolling upgrade opens.
//
// The bucket is shared by every replica of a controller, and during an
// upgrade that includes pods running the version before this one, which do
// not refresh what they hold. Shortening the expiry underneath one of those
// takes its lock away mid-reconcile and lets a second worker pick up the same
// object - and the first one's release, which deletes by key, would then take
// the second one's lock with it.
func TestShorteningTheLockExpiryWaitsForAnEmptyBucket(t *testing.T) {
	config := &nats.KeyValueConfig{Bucket: "lock-bucket", TTL: time.Minute}

	t.Run("a held lock defers the shortening", func(t *testing.T) {
		js := &fakeJetStream{maxAge: 20 * time.Minute}
		bucket := &bucketWithKeys{keys: []string{"control-plane-instance.1"}}

		require.NoError(t, alignLockBucketTTL(js, bucket, config))
		assert.Zero(
			t, js.updates,
			"an old worker still holding this lock would lose it mid-reconcile",
		)
	})

	t.Run("an empty bucket is shortened", func(t *testing.T) {
		js := &fakeJetStream{maxAge: 20 * time.Minute}
		bucket := &bucketWithKeys{}

		require.NoError(t, alignLockBucketTTL(js, bucket, config))
		assert.Equal(t, 1, js.updates, "nothing is held, so nothing can lose a lock")
		assert.Equal(t, time.Minute, js.updatedTo)
	})

	t.Run("lengthening is not held back by a held lock", func(t *testing.T) {
		js := &fakeJetStream{maxAge: time.Second}
		bucket := &bucketWithKeys{keys: []string{"control-plane-instance.1"}}

		require.NoError(t, alignLockBucketTTL(js, bucket, config))
		assert.Equal(
			t, 1, js.updates,
			"a longer expiry cannot take a lock away from anybody",
		)
	})

	t.Run("an expiry that already matches is left alone", func(t *testing.T) {
		js := &fakeJetStream{maxAge: time.Minute}
		bucket := &bucketWithKeys{}

		require.NoError(t, alignLockBucketTTL(js, bucket, config))
		assert.Zero(t, js.updates)
	})
}

// TestTheTtlAlignmentReportsWhatWentWrong covers the errors not being
// swallowed.
//
// A failure here leaves the old expiry in place. Returning nil would hide
// that behind a controller that started normally.
func TestTheTtlAlignmentReportsWhatWentWrong(t *testing.T) {
	config := &nats.KeyValueConfig{Bucket: "lock-bucket", TTL: time.Minute}

	t.Run("the stream cannot be read", func(t *testing.T) {
		js := &fakeJetStream{infoErr: errors.New("stream unavailable")}

		err := alignLockBucketTTL(js, &bucketWithKeys{}, config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stream unavailable")
	})

	t.Run("the update is refused", func(t *testing.T) {
		js := &fakeJetStream{maxAge: 20 * time.Minute, updateErr: errors.New("refused")}

		err := alignLockBucketTTL(js, &bucketWithKeys{}, config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refused")
		assert.Contains(t, err.Error(), "20m0s", "the error says what it was changing from")
	})

	t.Run("the held locks cannot be listed", func(t *testing.T) {
		js := &fakeJetStream{maxAge: 20 * time.Minute}
		bucket := &bucketWithKeys{keysErr: errors.New("bucket unavailable")}

		err := alignLockBucketTTL(js, bucket, config)
		require.Error(t, err, "not knowing whether a lock is held is not the same as none being held")
		assert.Contains(t, err.Error(), "bucket unavailable")
	})
}
