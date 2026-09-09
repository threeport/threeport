package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// peakWatcher is a sampler that records the highest in-flight operation
// count observed while it runs. Sampling can miss a spike and cannot
// invent one, so a peak above the cap proves the cap broke.
type peakWatcher struct {
	// The highest sampled in-flight count
	peak int64

	// The channel that stops the sampling goroutine
	stop chan struct{}

	// The channel closed when the sampling goroutine returns
	done chan struct{}
}

// peakSampleInterval is how often the peak watcher samples in-flight
// count. Without an interval the sampler pins a core and competes with
// the goroutines it measures.
const peakSampleInterval = 100 * time.Microsecond

// startPeakWatcher starts a sampler that records the highest in-flight
// count observed.
func startPeakWatcher() *peakWatcher {
	// construct watcher with stop and done channels
	w := &peakWatcher{stop: make(chan struct{}), done: make(chan struct{})}
	// sample in-flight count until stopped
	go func() {
		defer close(w.done)
		ticker := time.NewTicker(peakSampleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-w.stop:
				return
			case <-ticker.C:
				// read current in-flight count
				cur := inFlightCount()
				// raise peak when current is higher
				for {
					old := atomic.LoadInt64(&w.peak)
					if cur <= old || atomic.CompareAndSwapInt64(&w.peak, old, cur) {
						break
					}
				}
			}
		}
	}()
	return w
}

// stopAndPeak stops the sampler and returns the highest in-flight count
// seen.
func (w *peakWatcher) stopAndPeak() int64 {
	// stop the sampler
	close(w.stop)
	// wait for the sampler to return
	<-w.done
	// return the highest in-flight count seen
	return atomic.LoadInt64(&w.peak)
}

// waitForInFlightZero fatals if in-flight operations have not drained
// to zero within the given duration.
func waitForInFlightZero(t *testing.T, within time.Duration) {
	t.Helper()
	// bound the wait at within
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		// return once no operations are in flight
		if inFlightCount() == 0 {
			return
		}
		// wait 1ms between polls
		time.Sleep(time.Millisecond)
	}
	// fail if operations have not drained within the bound
	t.Fatalf("in-flight operations did not drain to zero within %s (still %d)", within, inFlightCount())
}

// TestReconcilerLoop_2000Instances_SemaphoreCapped covers 2000 blocked
// creates against a capacity-5 pool never exceeding five in flight.
func TestReconcilerLoop_2000Instances_SemaphoreCapped(t *testing.T) {
	const (
		n = 2000
		k = 5
	)
	// set semaphore capacity to 5
	configureSemaphoreTest(t, k)

	// block 2000 independent deploys so they hold semaphore slots
	fis := make([]*fakeInfra, n)
	fls := make([]*fakeLifecycle, n)
	for i := range fis {
		fis[i] = newFakeInfra()
		fis[i].setDeploy(infraBlock, nil)
		fls[i] = newFakeLifecycle()
		fls[i].setInfra(fis[i])
	}
	// release blocked deploys after the test
	t.Cleanup(func() {
		for _, fi := range fis {
			fi.releaseDeploy()
		}
	})

	// sample peak in-flight count
	watcher := startPeakWatcher()
	log := newTestLogger()

	// run five create-handler passes over all 2000 instances
	for pass := 0; pass < 5; pass++ {
		for i := 0; i < n; i++ {
			requeue, err := HandleInfraCreate(fls[i], log)
			require.NoError(t, err)
			// assert each create requeues 30 or 120
			require.Contains(t, []int64{30, 120}, requeue)
		}
	}

	// wait until in-flight count saturates at capacity; lastCount is
	// atomic because Eventually runs the condition on another goroutine
	var lastCount atomic.Int64
	reached := assert.Eventually(t, func() bool {
		c := inFlightCount()
		lastCount.Store(c)
		return c == int64(k)
	}, 5*time.Second, time.Millisecond)
	require.True(t, reached,
		"expected exactly %d blocked operations, last saw %d", k, lastCount.Load())

	// stop sampling and read peak
	peak := watcher.stopAndPeak()
	// assert peak never exceeds capacity
	assert.LessOrEqual(t, peak, int64(k), "concurrently executing operations must never exceed the semaphore capacity")
	// assert the pool saturates at capacity
	assert.Equal(t, int64(k), peak, "with blocking deploys the pool should saturate at the capacity")

	// release the held slots and wait for drain
	for _, fi := range fis {
		fi.releaseDeploy()
	}
	waitForInFlightZero(t, 10*time.Second)
}

// TestReconcilerLoop_2000CreateAndDelete_Mixed covers 1000 creates and
// 1000 deletes interleaved under a capacity-25 pool.
func TestReconcilerLoop_2000CreateAndDelete_Mixed(t *testing.T) {
	const (
		creates = 1000
		deletes = 1000
		k       = 25
	)
	// set semaphore capacity to 25
	configureSemaphoreTest(t, k)

	// succeed 1000 independent creates
	createFls := make([]*fakeLifecycle, creates)
	for i := range createFls {
		fi := newFakeInfra()
		fi.setDeploy(infraSucceed, nil)
		fl := newFakeLifecycle()
		fl.setInfra(fi)
		createFls[i] = fl
	}
	// succeed 1000 independent deletes with deletion already scheduled
	deleteFls := make([]*fakeLifecycle, deletes)
	for i := range deleteFls {
		fi := newFakeInfra()
		fi.setDestroy(infraSucceed, nil)
		fl := newFakeLifecycle(&ReconciliationSnapshot{
			DeletionScheduled: util.Ptr(deleteTestBase.Add(-time.Minute)),
		})
		fl.setInfra(fi)
		deleteFls[i] = fl
	}

	// sample peak in-flight count
	watcher := startPeakWatcher()
	log := newTestLogger()

	// stop the driver if the test returns
	stop := make(chan struct{})
	defer close(stop)

	// close done when the driver returns
	done := make(chan struct{})
	// collect driver errors instead of calling t.Error; a late t.Error
	// panics with Log in goroutine after <test> has completed
	var driverErr error

	// run create and delete handlers on a driver goroutine
	go func() {
		defer close(done)
		for pass := 0; pass < 10; pass++ {
			for i := 0; i < creates; i++ {
				// return if the test has finished
				select {
				case <-stop:
					return
				default:
				}
				// call create then delete for this pair
				if _, err := HandleInfraCreate(createFls[i], log); err != nil {
					driverErr = fmt.Errorf("create handler errored: %w", err)
					return
				}
				if _, err := HandleInfraDelete(deleteFls[i], log); err != nil {
					driverErr = fmt.Errorf("delete handler errored: %w", err)
					return
				}
			}
		}
	}()

	// wait for the driver to finish or time out
	select {
	case <-done:
		require.NoError(t, driverErr)
	case <-time.After(60 * time.Second):
		t.Fatal("mixed reconciler loop deadlocked or made no progress within 60s")
	}

	// stop sampling and read peak
	peak := watcher.stopAndPeak()
	// assert peak never exceeds capacity
	assert.LessOrEqual(t, peak, int64(k), "mixed create/delete churn must never exceed the semaphore capacity")

	// wait for in-flight operations to drain
	waitForInFlightZero(t, 10*time.Second)
}

// TestPerInstanceStateDirIsolation covers concurrent writes to distinct
// instance state files leaving each file with only its own content.
func TestPerInstanceStateDirIsolation(t *testing.T) {
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
