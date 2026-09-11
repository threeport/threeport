package v0

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

// TestRunParallelRunsEveryTaskDespiteAnError covers RunParallel running
// every task when one of them returns an error.
func TestRunParallelRunsEveryTaskDespiteAnError(t *testing.T) {
	// three tasks that increment a counter, the middle one failing
	var ran int32
	failure := errors.New("boom")
	tasks := []func() error{
		func() error { atomic.AddInt32(&ran, 1); return nil },
		func() error { atomic.AddInt32(&ran, 1); return failure },
		func() error { atomic.AddInt32(&ran, 1); return nil },
	}
	// run the tasks
	err := RunParallel(2, tasks)
	// assert a failure does not short-circuit the other tasks
	if got := atomic.LoadInt32(&ran); got != 3 {
		t.Errorf("ran %d tasks, want all 3", got)
	}
	// assert the returned error contains boom
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("RunParallel error = %v, want it to surface boom", err)
	}
}

// TestRunParallelRunsAllSequentiallyBelowOne covers RunParallel running
// every task at a worker count below one.
func TestRunParallelRunsAllSequentiallyBelowOne(t *testing.T) {
	// two tasks that increment a counter
	var ran int32
	tasks := []func() error{
		func() error { atomic.AddInt32(&ran, 1); return nil },
		func() error { atomic.AddInt32(&ran, 1); return nil },
	}
	// run with a worker count of zero
	if err := RunParallel(0, tasks); err != nil {
		t.Fatalf("RunParallel returned error: %v", err)
	}
	// assert every task ran
	if got := atomic.LoadInt32(&ran); got != 2 {
		t.Errorf("ran %d tasks, want all 2", got)
	}
}

// TestRunParallelAggregatesEveryError covers RunParallel returning every
// failing task's error.
func TestRunParallelAggregatesEveryError(t *testing.T) {
	// two failing tasks around one that succeeds
	tasks := []func() error{
		func() error { return errors.New("first failure") },
		func() error { return nil },
		func() error { return errors.New("second failure") },
	}
	// run mixed success and failure tasks
	err := RunParallel(3, tasks)
	// assert an error is returned
	if err == nil {
		t.Fatalf("RunParallel returned nil, want an aggregate error")
	}
	// assert both failures appear in the returned error
	if !strings.Contains(err.Error(), "first failure") || !strings.Contains(err.Error(), "second failure") {
		t.Errorf("aggregate error = %q, want both failures", err.Error())
	}
}

// TestRunParallelEmptyTasksReturnsNil covers RunParallel returning nil
// for a nil task list.
func TestRunParallelEmptyTasksReturnsNil(t *testing.T) {
	// run a nil task list
	if err := RunParallel(4, nil); err != nil {
		t.Errorf("RunParallel(nil) = %v, want nil", err)
	}
}

// TestRunParallelAllSuccessReturnsNil covers RunParallel returning nil
// when every task succeeds.
func TestRunParallelAllSuccessReturnsNil(t *testing.T) {
	// two tasks that return nil
	tasks := []func() error{
		func() error { return nil },
		func() error { return nil },
	}
	// run the tasks
	if err := RunParallel(2, tasks); err != nil {
		t.Errorf("RunParallel = %v, want nil", err)
	}
}
