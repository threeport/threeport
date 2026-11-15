package controller

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEveryControllerReplicatesItsLockBucket checks each generated
// controller main: it reads how many servers the message broker runs from
// --msg-broker-replicas, which MSG_BROKER_REPLICAS in its config secret
// sets, and creates its lock bucket with that many replicas, so a
// production broker keeps the locks through the loss of a server.
func TestEveryControllerReplicatesItsLockBucket(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..", "..")
	mains, err := filepath.Glob(filepath.Join(root, "cmd", "*-controller", "main_gen.go"))
	require.NoError(t, err)
	require.NotEmpty(t, mains)

	flagDefinition := `flag.Int("msg-broker-replicas", 1,`
	bucketReplicas := regexp.MustCompile(`(?s)KeyValueConfig\{.*?Replicas:\s+\*msgBrokerReplicas,.*?\}`)
	for _, path := range mains {
		source, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.True(t, strings.Contains(string(source), flagDefinition), "%s does not read --msg-broker-replicas", path)
		assert.True(t, bucketReplicas.Match(source), "%s does not replicate its lock bucket", path)
	}
}
