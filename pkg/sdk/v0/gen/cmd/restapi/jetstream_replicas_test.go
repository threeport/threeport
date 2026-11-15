package restapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTheAPIServerReplicatesEveryStream checks the generated REST API: it
// reads how many servers the message broker runs from NATS_REPLICAS in its
// config, and creates every controller stream with that many replicas, so a
// production broker keeps the notifications through the loss of a server.
func TestTheAPIServerReplicatesEveryStream(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..", "..")

	main, err := os.ReadFile(filepath.Join(root, "cmd", "rest-api", "main_gen.go"))
	require.NoError(t, err)
	assert.Contains(t, string(main), `messagebroker.ParseReplicas(os.Getenv("NATS_REPLICAS"))`)
	assert.Contains(t, string(main), `util.InitJetStream(nc, natsReplicas)`)

	streams, err := os.ReadFile(filepath.Join(root, "cmd", "rest-api", "util", "controller_stream_gen.go"))
	require.NoError(t, err)
	added := strings.Count(string(streams), "js.AddStream(")
	require.NotZero(t, added)
	assert.Equal(t, added, strings.Count(string(streams), "Replicas: replicas,"), "every stream is created with the broker's replicas")
}
