package v0

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	notifications "github.com/threeport/threeport/pkg/notifications/v0"
)

// TestNatsConfSetsDiskCeiling checks the message broker names its disk ceiling
// and that the ceiling and the stream caps fit in the volume.
func TestNatsConfSetsDiskCeiling(t *testing.T) {
	conf := natsConf()

	if !strings.Contains(conf, "max_file: 4Gi") {
		t.Errorf("nats.conf does not set max_file to 4Gi:\n%s", conf)
	}
	if notifications.MessageBrokerMaxFileGiB >= notifications.MessageBrokerVolumeGiB {
		t.Errorf(
			"max_file %dGi is not below the %dGi volume",
			notifications.MessageBrokerMaxFileGiB, notifications.MessageBrokerVolumeGiB,
		)
	}

	streams := int64(notifications.ControllerStreamCount) * notifications.ControllerStreamMaxBytes
	if ceiling := int64(notifications.MessageBrokerMaxFileGiB) << 30; streams > ceiling {
		t.Errorf("stream caps total %d bytes, over the %d byte max_file", streams, ceiling)
	}
}

// TestNatsConfChecksumTracksConfig checks the pod template annotation moves
// with the configuration, because the server ignores a reloaded JetStream
// limit and has to be restarted.
func TestNatsConfChecksumTracksConfig(t *testing.T) {
	want := fmt.Sprintf("%x", sha256.Sum256([]byte(natsConf())))
	if got := natsConfChecksum(); got != want {
		t.Errorf("checksum %s, want %s", got, want)
	}
	if got := natsConfChecksum(); got == "3b398e973c292bf8c2eb90d62acb846274c0489643aad560d8c4aed123f20ce7" {
		t.Error("checksum is still the hard-coded value")
	}
}
