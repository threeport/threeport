package util

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	secret_notif "github.com/threeport/threeport/internal/secret/notif"
	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

// startJetStream runs an in-process server with the message broker's disk
// ceiling and returns a connection to it.
func startJetStream(t *testing.T) (*server.Server, *nats.Conn) {
	t.Helper()
	srv, err := server.NewServer(&server.Options{
		Port:              -1,
		JetStream:         true,
		StoreDir:          t.TempDir(),
		JetStreamMaxStore: notifications.MessageBrokerMaxFileGiB << 30,
		NoLog:             true,
		NoSigs:            true,
	})
	if err != nil {
		t.Fatalf("failed to create nats server: %v", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(10 * time.Second) {
		t.Fatal("nats server did not become ready")
	}
	t.Cleanup(srv.Shutdown)
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("failed to connect to nats: %v", err)
	}
	t.Cleanup(nc.Close)
	return srv, nc
}

func TestInitJetStreamBoundsEveryStream(t *testing.T) {
	_, nc := startJetStream(t)

	js, err := InitJetStream(nc)
	if err != nil {
		t.Fatalf("failed to initialize streams: %v", err)
	}
	// a second start of the REST API finds the same configuration
	if _, err := InitJetStream(nc); err != nil {
		t.Fatalf("second initialization failed: %v", err)
	}

	count := 0
	for info := range (*js).StreamsInfo() {
		count++
		cfg := info.Config
		if cfg.Retention != nats.WorkQueuePolicy {
			t.Errorf("%s: retention %v, want work queue", cfg.Name, cfg.Retention)
		}
		if cfg.Discard != nats.DiscardNew {
			t.Errorf("%s: discard %v, want discard new", cfg.Name, cfg.Discard)
		}
		if cfg.MaxBytes != notifications.ControllerStreamMaxBytes {
			t.Errorf("%s: max bytes %d, want %d", cfg.Name, cfg.MaxBytes, int64(notifications.ControllerStreamMaxBytes))
		}
		if cfg.Storage != nats.FileStorage {
			t.Errorf("%s: storage %v, want file", cfg.Name, cfg.Storage)
		}
	}
	if count != notifications.ControllerStreamCount {
		t.Errorf("got %d streams, want %d", count, notifications.ControllerStreamCount)
	}
}

func TestAcknowledgedNotificationIsRemoved(t *testing.T) {
	_, nc := startJetStream(t)
	js, err := InitJetStream(nc)
	if err != nil {
		t.Fatalf("failed to initialize streams: %v", err)
	}

	subject := secret_notif.SecretDefinitionCreateSubject
	if _, err := (*js).Publish(subject, []byte("notification")); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}
	sub, err := (*js).PullSubscribe(subject, "SecretDefinitionReconcilerConsumer", nats.BindStream(secret_notif.SecretStreamName))
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}
	msgs, err := sub.Fetch(1, nats.MaxWait(5*time.Second))
	if err != nil || len(msgs) != 1 {
		t.Fatalf("failed to fetch the notification: %v", err)
	}
	if err := msgs[0].AckSync(); err != nil {
		t.Fatalf("failed to acknowledge: %v", err)
	}

	info, err := (*js).StreamInfo(secret_notif.SecretStreamName)
	if err != nil {
		t.Fatalf("failed to get stream info: %v", err)
	}
	if info.State.Msgs != 0 {
		t.Errorf("stream retains %d acknowledged notifications", info.State.Msgs)
	}
}

// TestConsumerSharedDuringRollingUpdate covers a rolling update of a
// controller.  The old pod keeps its consumer until the new pod is ready, and a
// work queue stream refuses a second consumer on the same subjects.  Controllers
// bind a durable consumer so both pods can attach, and an ephemeral one would
// leave the new pod refused for as long as the old pod runs.
func TestConsumerSharedDuringRollingUpdate(t *testing.T) {
	srv, nc := startJetStream(t)
	if _, err := InitJetStream(nc); err != nil {
		t.Fatalf("failed to initialize streams: %v", err)
	}

	subject := secret_notif.SecretDefinitionSubject
	consumer := "SecretDefinitionReconcilerConsumer"
	bind := nats.BindStream(secret_notif.SecretStreamName)

	// attach as a controller pod does, with its own connection
	attach := func(durable bool) (nats.JetStreamContext, *nats.Subscription, error) {
		conn, err := nats.Connect(srv.ClientURL())
		if err != nil {
			t.Fatalf("failed to connect to nats: %v", err)
		}
		t.Cleanup(conn.Close)
		js, err := conn.JetStream()
		if err != nil {
			t.Fatalf("failed to create jetstream context: %v", err)
		}
		if !durable {
			sub, err := js.PullSubscribe(subject, "", bind)
			return js, sub, err
		}
		if _, err := js.AddConsumer(secret_notif.SecretStreamName, &nats.ConsumerConfig{
			AckPolicy:     nats.AckExplicitPolicy,
			Durable:       consumer,
			FilterSubject: subject,
		}); err != nil {
			return js, nil, err
		}
		sub, err := js.PullSubscribe(subject, consumer, bind)
		return js, sub, err
	}

	// the old pod stays connected through the update
	oldJS, oldSub, err := attach(true)
	if err != nil {
		t.Fatalf("old pod failed to attach: %v", err)
	}
	if _, _, err := attach(true); err != nil {
		t.Fatalf("new pod refused while the old pod is running: %v", err)
	}

	// a notification is still processed once
	if _, err := oldJS.Publish(secret_notif.SecretDefinitionCreateSubject, []byte("notification")); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}
	msgs, err := oldSub.Fetch(1, nats.MaxWait(5*time.Second))
	if err != nil || len(msgs) != 1 {
		t.Fatalf("old pod did not receive the notification: %v", err)
	}
	if err := msgs[0].AckSync(); err != nil {
		t.Fatalf("failed to acknowledge: %v", err)
	}
}

// TestEphemeralConsumerBlocksRollingUpdate records why controllers do not bind
// ephemeral consumers: the new pod is refused while the old pod is running.
func TestEphemeralConsumerBlocksRollingUpdate(t *testing.T) {
	srv, nc := startJetStream(t)
	js, err := InitJetStream(nc)
	if err != nil {
		t.Fatalf("failed to initialize streams: %v", err)
	}

	subject := secret_notif.SecretDefinitionSubject
	bind := nats.BindStream(secret_notif.SecretStreamName)

	oldConn, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(oldConn.Close)
	oldJS, _ := oldConn.JetStream()
	oldSub, err := oldJS.PullSubscribe(subject, "", bind)
	if err != nil {
		t.Fatalf("old pod failed to attach: %v", err)
	}
	// the old pod keeps pulling, which keeps its consumer alive
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = oldSub.Fetch(1, nats.MaxWait(250*time.Millisecond))
			}
		}
	}()

	for i := 0; i < 3; i++ {
		if _, err := (*js).PullSubscribe(subject, "", bind); err == nil {
			t.Fatal("new pod attached an ephemeral consumer while the old pod is running")
		}
		time.Sleep(time.Second)
	}
}
