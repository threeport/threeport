//go:build race

package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPerInstanceStateDirIsolation_Race covers concurrent writes to distinct
// instance state files leaving each file with only its own content.
func TestPerInstanceStateDirIsolation_Race(t *testing.T) {
	const n = 100
	root := t.TempDir()

	// collect unique state file paths for 100 instances
	paths := make([]string, n)
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		w := NewPulumiWorkspace(fmt.Sprintf("inst-%d", i), "proj", WithStateDirRoot(root))
		p, err := w.GetStateFilePath()
		require.NoError(t, err)
		require.False(t, seen[p], "state file path collided across instances: %s", p)
		seen[p] = true
		paths[i] = p
	}

	// write distinct content to each path concurrently; use assert not
	// require because FailNow from a worker only stops that goroutine
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// create the state file parent directory
			if !assert.NoError(t, os.MkdirAll(filepath.Dir(paths[i]), 0755)) {
				return
			}
			// write this instance's content
			content := fmt.Sprintf("state-for-inst-%d", i)
			assert.NoError(t, os.WriteFile(paths[i], []byte(content), 0644))
		}(i)
	}
	wg.Wait()
	// skip read-back if a concurrent write failed
	if t.Failed() {
		t.Fatal("concurrent state file writes failed; skipping read-back")
	}

	// assert each file holds only its own content
	for i := 0; i < n; i++ {
		got, err := os.ReadFile(paths[i])
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("state-for-inst-%d", i), string(got),
			"each instance's state file must hold only its own content")
	}
}
