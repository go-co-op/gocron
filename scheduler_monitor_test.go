package gocron

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSchedulerMonitor is a test implementation of SchedulerMonitor
// that tracks scheduler lifecycle events
type testSchedulerMonitor struct {
	mu                    sync.RWMutex
	startedCount          int64
	stoppedCount          int64
	shutdownCount         int64
	jobRegCount           int64
	jobUnregCount         int64
	jobStartCount         int64
	jobRunningCount       int64
	jobCompletedCount     int64
	jobFailedCount        int64
	concurrencyLimitCount int64
	startedCalls          []time.Time
	stoppedCalls          []time.Time
	shutdownCalls         []time.Time
	jobRegCalls           []Job
	jobUnregCalls         []Job
	jobStartCalls         []Job
	jobRunningCalls       []Job
	jobCompletedCalls     []Job
	jobExecutionTimes     []time.Duration
	jobSchedulingDelays   []time.Duration
	concurrencyLimitCalls []string
	jobFailedCalls        struct {
		jobs []Job
		errs []error
	}
}

func newTestSchedulerMonitor() *testSchedulerMonitor {
	return &testSchedulerMonitor{
		startedCalls:          make([]time.Time, 0),
		stoppedCalls:          make([]time.Time, 0),
		shutdownCalls:         make([]time.Time, 0),
		jobRegCalls:           make([]Job, 0),
		jobUnregCalls:         make([]Job, 0),
		jobStartCalls:         make([]Job, 0),
		jobRunningCalls:       make([]Job, 0),
		jobCompletedCalls:     make([]Job, 0),
		jobExecutionTimes:     make([]time.Duration, 0),
		jobSchedulingDelays:   make([]time.Duration, 0),
		concurrencyLimitCalls: make([]string, 0),
		jobFailedCalls: struct {
			jobs []Job
			errs []error
		}{
			jobs: make([]Job, 0),
			errs: make([]error, 0),
		},
	}
}

func (t *testSchedulerMonitor) SchedulerStarted() {
	t.mu.Lock()
	defer t.mu.Unlock()
	atomic.AddInt64(&t.startedCount, 1)
	t.startedCalls = append(t.startedCalls, time.Now())
}

func (t *testSchedulerMonitor) SchedulerStopped() {
	t.mu.Lock()
	defer t.mu.Unlock()
	atomic.AddInt64(&t.stoppedCount, 1)
	t.stoppedCalls = append(t.stoppedCalls, time.Now())
}

func (t *testSchedulerMonitor) SchedulerShutdown() {
	t.mu.Lock()
	defer t.mu.Unlock()
	atomic.AddInt64(&t.shutdownCount, 1)
	t.shutdownCalls = append(t.shutdownCalls, time.Now())
}

func (t *testSchedulerMonitor) getStartedCount() int64 {
	return atomic.LoadInt64(&t.startedCount)
}

func (t *testSchedulerMonitor) getShutdownCount() int64 {
	return atomic.LoadInt64(&t.shutdownCount)
}

func (t *testSchedulerMonitor) getStartedCalls() []time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return append([]time.Time{}, t.startedCalls...)
}

func (t *testSchedulerMonitor) getShutdownCalls() []time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return append([]time.Time{}, t.shutdownCalls...)
}

func (t *testSchedulerMonitor) JobRegistered(job Job) {
	t.mu.Lock()
	defer t.mu.Unlock()
	atomic.AddInt64(&t.jobRegCount, 1)
	t.jobRegCalls = append(t.jobRegCalls, job)
}

func (t *testSchedulerMonitor) JobUnregistered(job Job) {
	t.mu.Lock()
	defer t.mu.Unlock()
	atomic.AddInt64(&t.jobUnregCount, 1)
	t.jobUnregCalls = append(t.jobUnregCalls, job)
}

func (t *testSchedulerMonitor) JobStarted(job Job) {
	t.mu.Lock()
	defer t.mu.Unlock()
	atomic.AddInt64(&t.jobStartCount, 1)
	t.jobStartCalls = append(t.jobStartCalls, job)
}

func (t *testSchedulerMonitor) JobRunning(job Job) {
	t.mu.Lock()
	defer t.mu.Unlock()
	atomic.AddInt64(&t.jobRunningCount, 1)
	t.jobRunningCalls = append(t.jobRunningCalls, job)
}

func (t *testSchedulerMonitor) JobCompleted(job Job) {
	t.mu.Lock()
	defer t.mu.Unlock()
	atomic.AddInt64(&t.jobCompletedCount, 1)
	t.jobCompletedCalls = append(t.jobCompletedCalls, job)
}

func (t *testSchedulerMonitor) JobFailed(job Job, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	atomic.AddInt64(&t.jobFailedCount, 1)
	t.jobFailedCalls.jobs = append(t.jobFailedCalls.jobs, job)
	t.jobFailedCalls.errs = append(t.jobFailedCalls.errs, err)
}

func (t *testSchedulerMonitor) JobExecutionTime(_ Job, duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.jobExecutionTimes = append(t.jobExecutionTimes, duration)
}

func (t *testSchedulerMonitor) JobSchedulingDelay(_ Job, scheduledTime time.Time, actualStartTime time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delay := actualStartTime.Sub(scheduledTime)
	if delay > 0 {
		t.jobSchedulingDelays = append(t.jobSchedulingDelays, delay)
	}
}

func (t *testSchedulerMonitor) ConcurrencyLimitReached(limitType string, _ Job) {
	t.mu.Lock()
	defer t.mu.Unlock()
	atomic.AddInt64(&t.concurrencyLimitCount, 1)
	t.concurrencyLimitCalls = append(t.concurrencyLimitCalls, limitType)
}

func (t *testSchedulerMonitor) getJobRegCount() int64 {
	return atomic.LoadInt64(&t.jobRegCount)
}

func (t *testSchedulerMonitor) getJobUnregCount() int64 {
	return atomic.LoadInt64(&t.jobUnregCount)
}

func (t *testSchedulerMonitor) getJobStartCount() int64 {
	return atomic.LoadInt64(&t.jobStartCount)
}

func (t *testSchedulerMonitor) getJobRunningCount() int64 {
	return atomic.LoadInt64(&t.jobRunningCount)
}

func (t *testSchedulerMonitor) getJobCompletedCount() int64 {
	return atomic.LoadInt64(&t.jobCompletedCount)
}

func (t *testSchedulerMonitor) getJobFailedCount() int64 {
	return atomic.LoadInt64(&t.jobFailedCount)
}

// func (t *testSchedulerMonitor) getJobRegCalls() []Job {
// 	t.mu.RLock()
// 	defer t.mu.RUnlock()
// 	return append([]Job{}, t.jobRegCalls...)
// }

// func (t *testSchedulerMonitor) getJobUnregCalls() []Job {
// 	t.mu.RLock()
// 	defer t.mu.RUnlock()
// 	return append([]Job{}, t.jobUnregCalls...)
// }

// func (t *testSchedulerMonitor) getJobStartCalls() []Job {
// 	t.mu.RLock()
// 	defer t.mu.RUnlock()
// 	return append([]Job{}, t.jobStartCalls...)
// }

// func (t *testSchedulerMonitor) getJobRunningCalls() []Job {
// 	t.mu.RLock()
// 	defer t.mu.RUnlock()
// 	return append([]Job{}, t.jobRunningCalls...)
// }

// func (t *testSchedulerMonitor) getJobCompletedCalls() []Job {
// 	t.mu.RLock()
// 	defer t.mu.RUnlock()
// 	return append([]Job{}, t.jobCompletedCalls...)
// }

func (t *testSchedulerMonitor) getJobFailedCalls() ([]Job, []error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	jobs := append([]Job{}, t.jobFailedCalls.jobs...)
	errs := append([]error{}, t.jobFailedCalls.errs...)
	return jobs, errs
}

func TestSchedulerMonitor_Basic(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	monitor := newTestSchedulerMonitor()
	s := newTestScheduler(t, WithSchedulerMonitor(monitor))

	// Before starting, monitor should not have been called
	assert.Equal(t, int64(0), monitor.getStartedCount())
	assert.Equal(t, int64(0), monitor.getShutdownCount())

	// Add a simple job
	_, err := s.NewJob(
		DurationJob(time.Second),
		NewTask(func() {}),
	)
	require.NoError(t, err)

	// Start the scheduler
	s.Start()

	// Wait a bit for the start to complete
	time.Sleep(50 * time.Millisecond)

	// SchedulerStarted should have been called once
	assert.Equal(t, int64(1), monitor.getStartedCount())
	assert.Equal(t, int64(0), monitor.getShutdownCount())

	// Shutdown the scheduler
	err = s.Shutdown()
	require.NoError(t, err)

	// SchedulerShutdown should have been called once
	assert.Equal(t, int64(1), monitor.getStartedCount())
	assert.Equal(t, int64(1), monitor.getShutdownCount())

	// Verify the order of calls
	startedCalls := monitor.getStartedCalls()
	shutdownCalls := monitor.getShutdownCalls()
	require.Len(t, startedCalls, 1)
	require.Len(t, shutdownCalls, 1)
	assert.True(t, startedCalls[0].Before(shutdownCalls[0]),
		"SchedulerStarted should be called before SchedulerShutdown")
}

func TestSchedulerMonitor_MultipleStartStop(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	monitor := newTestSchedulerMonitor()
	s := newTestScheduler(t, WithSchedulerMonitor(monitor))

	_, err := s.NewJob(
		DurationJob(time.Second),
		NewTask(func() {}),
	)
	require.NoError(t, err)

	// Start and stop multiple times
	s.Start()
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int64(1), monitor.getStartedCount())

	err = s.StopJobs()
	require.NoError(t, err)
	// StopJobs shouldn't call SchedulerShutdown
	assert.Equal(t, int64(0), monitor.getShutdownCount())

	// Start again
	s.Start()
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int64(2), monitor.getStartedCount())

	// Final shutdown
	err = s.Shutdown()
	require.NoError(t, err)
	assert.Equal(t, int64(1), monitor.getShutdownCount())
}

func TestSchedulerMonitor_WithoutMonitor(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	// Create scheduler without monitor - should not panic
	s := newTestScheduler(t)

	_, err := s.NewJob(
		DurationJob(time.Second),
		NewTask(func() {}),
	)
	require.NoError(t, err)

	s.Start()
	time.Sleep(50 * time.Millisecond)

	err = s.Shutdown()
	require.NoError(t, err)
}

func TestSchedulerMonitor_NilMonitor(t *testing.T) {
	// Attempting to create a scheduler with nil monitor should error
	_, err := NewScheduler(WithSchedulerMonitor(nil))
	assert.Error(t, err)
	assert.Equal(t, ErrSchedulerMonitorNil, err)
}

func TestSchedulerMonitor_ConcurrentAccess(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	monitor := newTestSchedulerMonitor()
	s := newTestScheduler(t, WithSchedulerMonitor(monitor))

	// Add multiple jobs
	for i := 0; i < 10; i++ {
		_, err := s.NewJob(
			DurationJob(100*time.Millisecond),
			NewTask(func() {}),
		)
		require.NoError(t, err)
	}

	// Start scheduler once (normal use case)
	s.Start()
	time.Sleep(150 * time.Millisecond)

	// Verify monitor was called
	assert.Equal(t, int64(1), monitor.getStartedCount())

	err := s.Shutdown()
	require.NoError(t, err)

	// Monitor should be called for shutdown
	assert.Equal(t, int64(1), monitor.getShutdownCount())
}

func TestSchedulerMonitor_StartWithoutJobs(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	monitor := newTestSchedulerMonitor()
	s := newTestScheduler(t, WithSchedulerMonitor(monitor))

	// Start scheduler without any jobs
	s.Start()
	time.Sleep(50 * time.Millisecond)

	// Monitor should still be called
	assert.Equal(t, int64(1), monitor.getStartedCount())

	err := s.Shutdown()
	require.NoError(t, err)
	assert.Equal(t, int64(1), monitor.getShutdownCount())
}

func TestSchedulerMonitor_ShutdownWithoutStart(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	monitor := newTestSchedulerMonitor()
	s := newTestScheduler(t, WithSchedulerMonitor(monitor))

	_, err := s.NewJob(
		DurationJob(time.Second),
		NewTask(func() {}),
	)
	require.NoError(t, err)

	// Shutdown without starting
	err = s.Shutdown()
	require.NoError(t, err)

	// SchedulerStarted should not be called
	assert.Equal(t, int64(0), monitor.getStartedCount())
	// SchedulerShutdown should not be called if scheduler was never started
	assert.Equal(t, int64(0), monitor.getShutdownCount())
}

func TestSchedulerMonitor_ThreadSafety(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	monitor := newTestSchedulerMonitor()

	// Simulate concurrent calls to the monitor from multiple goroutines
	var wg sync.WaitGroup
	iterations := 100

	for i := 0; i < iterations; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			monitor.SchedulerStarted()
		}()
		go func() {
			defer wg.Done()
			monitor.SchedulerShutdown()
		}()
	}

	wg.Wait()

	// Verify all calls were recorded
	assert.Equal(t, int64(iterations), monitor.getStartedCount())
	assert.Equal(t, int64(iterations), monitor.getShutdownCount())
	assert.Len(t, monitor.getStartedCalls(), iterations)
	assert.Len(t, monitor.getShutdownCalls(), iterations)
}

func TestSchedulerMonitor_IntegrationWithJobs(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	monitor := newTestSchedulerMonitor()
	s := newTestScheduler(t, WithSchedulerMonitor(monitor))

	// Test successful job
	jobRunCount := atomic.Int32{}
	j, err := s.NewJob(
		DurationJob(50*time.Millisecond),
		NewTask(func() {
			jobRunCount.Add(1)
		}),
		WithStartAt(WithStartImmediately()),
	)
	require.NoError(t, err)

	// Test failing job
	_, err = s.NewJob(
		DurationJob(50*time.Millisecond),
		NewTask(func() error {
			return fmt.Errorf("test error")
		}),
		WithStartAt(WithStartImmediately()),
	)
	require.NoError(t, err)

	// Start scheduler
	s.Start()
	time.Sleep(150 * time.Millisecond) // Wait for jobs to execute

	// Verify scheduler lifecycle events
	assert.Equal(t, int64(1), monitor.getStartedCount())
	assert.GreaterOrEqual(t, jobRunCount.Load(), int32(1))

	// Verify job registration
	assert.Equal(t, int64(2), monitor.getJobRegCount(), "Should have registered 2 jobs")

	// Verify job execution events
	assert.GreaterOrEqual(t, monitor.getJobStartCount(), int64(1), "Jobs should have started")
	assert.GreaterOrEqual(t, monitor.getJobRunningCount(), int64(1), "Jobs should be running")
	assert.GreaterOrEqual(t, monitor.getJobCompletedCount(), int64(1), "Successful job should complete")
	assert.GreaterOrEqual(t, monitor.getJobFailedCount(), int64(1), "Failing job should fail")

	// Get failed job details
	failedJobs, errors := monitor.getJobFailedCalls()
	assert.NotEmpty(t, failedJobs, "Should have recorded failed jobs")
	assert.NotEmpty(t, errors, "Should have recorded job errors")
	assert.Contains(t, errors[0].Error(), "test error", "Should record the correct error")

	// Test unregistration
	err = s.RemoveJob(j.ID())
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond) // Wait for async removal
	assert.Equal(t, int64(1), monitor.getJobUnregCount(), "Should have unregistered 1 job")

	// Shutdown
	err = s.Shutdown()
	require.NoError(t, err)
	assert.Equal(t, int64(1), monitor.getShutdownCount())
}
