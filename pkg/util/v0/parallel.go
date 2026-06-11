package v0

import "sync"

// RunParallel executes tasks concurrently with the given worker count.
// All tasks run regardless of whether one fails; errors are aggregated
// via MultiError. A parallel value of 1 (or less) collapses to a
// single sequential worker. Designed to match the worker-pool shape
// in tptdev's build command so the two can converge later.
func RunParallel(parallel int, tasks []func() error) error {
	if parallel < 1 {
		parallel = 1
	}

	jobs := make(chan func() error)
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var multi MultiError

	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range jobs {
				if err := task(); err != nil {
					errMu.Lock()
					multi.AppendError(err)
					errMu.Unlock()
				}
			}
		}()
	}

	for _, task := range tasks {
		jobs <- task
	}
	close(jobs)
	wg.Wait()

	return multi.Error()
}
