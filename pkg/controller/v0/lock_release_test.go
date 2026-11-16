package controller

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jetStreamKeyValue returns a bucket in a JetStream server started for the
// test, which enforces the revision a delete is given as NATS does.
func jetStreamKeyValue(t *testing.T) nats.KeyValue {
	t.Helper()

	srv, err := server.NewServer(&server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	})
	require.NoError(t, err)
	go srv.Start()
	t.Cleanup(srv.Shutdown)
	require.True(t, srv.ReadyForConnections(10*time.Second))

	conn, err := nats.Connect(srv.ClientURL())
	require.NoError(t, err)
	t.Cleanup(conn.Close)
	js, err := conn.JetStream()
	require.NoError(t, err)
	keyValue, err := js.CreateKeyValue(&nats.KeyValueConfig{Bucket: "test-lock-bucket"})
	require.NoError(t, err)

	return keyValue
}

// TestAReleaseDoesNotDeleteALockSomebodyElseTook covers what ReleaseLock's
// delete by revision is for.  A controller whose lock expired, and was taken
// by another controller while it was still reconciling, releases with the
// revision it last wrote.  NATS refuses that delete, so the new owner's
// lock survives, rather than handing the object to a third controller.
//
// It runs against a real bucket: fakeKeyValue can't check a delete's
// revision, since nats.LastRevision keeps it where only the nats package can
// read it.
func TestAReleaseDoesNotDeleteALockSomebodyElseTook(t *testing.T) {
	keyValue := jetStreamKeyValue(t)
	reconciler := reconcilerFor(t, keyValue)
	object := &lockTestObject{id: 1}
	lockKey := reconciler.lockKey(object.GetId())

	require.True(t, reconciler.Lock(object))
	ours, err := keyValue.Get(lockKey)
	require.NoError(t, err)

	// the lock expires and another controller takes it
	require.NoError(t, keyValue.Purge(lockKey))
	_, err = keyValue.Create(lockKey, []byte("another-controller"))
	require.NoError(t, err)

	assert.False(
		t, reconciler.ReleaseLock(object, make(chan bool, 1), nil, false),
		"the delete must be refused, since the key's revision is no longer the one this controller wrote",
	)

	entry, err := keyValue.Get(lockKey)
	require.NoError(t, err, "the new owner's lock must survive the release")
	assert.Equal(t, "another-controller", string(entry.Value()))
	assert.NotEqual(t, string(ours.Value()), string(entry.Value()))
}

// TestAReleaseDeletesTheLockItHolds is the other half: a controller that
// still holds its lock deletes it, refreshed revision and all.
func TestAReleaseDeletesTheLockItHolds(t *testing.T) {
	keyValue := jetStreamKeyValue(t)
	reconciler := reconcilerFor(t, keyValue)
	object := &lockTestObject{id: 1}

	require.True(t, reconciler.Lock(object))
	// long enough for the heartbeat to refresh the revision more than once
	time.Sleep(40 * time.Millisecond)

	assert.True(t, reconciler.ReleaseLock(object, make(chan bool, 1), nil, false))
	_, err := keyValue.Get(reconciler.lockKey(object.GetId()))
	assert.ErrorIs(t, err, nats.ErrKeyNotFound, "the lock is gone once released")
}
