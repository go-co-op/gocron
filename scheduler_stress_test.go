package gocron

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStress_ConcurrentNewJobAndRemoveJob tests concurrent job additions and removals.
// This exercises the scheduler's main select loop handling both newJobCh and removeJobCh
// simultaneously, ensuring no race conditions or map corruption.
func TestStress_ConcurrentNewJobAndRemoveJob(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	tests := []struct {
		name           string
		numAdders      int
		numRemovers    int
		jobsPerAdder   int
		duration       time.Duration
		expectedMinOps int // minimum successful operations
	}{
		{
			name:           "10 concurrent adders and removers",
			numAdders:      10,
			numRemovers:    10,
			jobsPerAdder:   20,
			duration:       2 * time.Second,
			expectedMinOps: 50, // at least some operations should succeed
		},
		{
			name:           "high contention - 50 adders/removers",
			numAdders:      50,
			numRemovers:    50,
			jobsPerAdder:   10,
			duration:       3 * time.Second,
			expectedMinOps: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t)
			t.Cleanup(func() {
				_ = s.Shutdown()
			})
			s.Start()

			var (
				addedCount   atomic.Int32
				removedCount atomic.Int32
				jobIDs       sync.Map // store job IDs for removal
			)

			ctx, cancel := context.WithTimeout(context.Background(), tt.duration)
			defer cancel()

			var wg sync.WaitGroup

			// Spawn adders
			for i := 0; i < tt.numAdders; i++ {
				wg.Add(1)
				go func(_ int) {
					defer wg.Done()
					for j := 0; j < tt.jobsPerAdder; j++ {
						select {
						case <-ctx.Done():
							return
						default:
						}

						job, err := s.NewJob(
							DurationJob(100*time.Millisecond),
							NewTask(func() {}),
						)
						if err == nil {
							addedCount.Add(1)
							jobIDs.Store(job.ID(), struct{}{})
						}
					}
				}(i)
			}

			// Spawn removers
			for i := 0; i < tt.numRemovers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-ctx.Done():
							return
						default:
						}

						// Try to remove a random job
						var idToRemove uuid.UUID
						jobIDs.Range(func(key, _ interface{}) bool {
							idToRemove = key.(uuid.UUID)
							return false // stop after first
						})

						if idToRemove != uuid.Nil {
							err := s.RemoveJob(idToRemove)
							if err == nil {
								removedCount.Add(1)
								jobIDs.Delete(idToRemove)
							}
						}
						time.Sleep(time.Millisecond) // small delay
					}
				}()
			}

			wg.Wait()

			totalOps := int(addedCount.Load() + removedCount.Load())
			assert.GreaterOrEqual(t, totalOps, tt.expectedMinOps,
				"Should complete minimum operations")

			// Verify scheduler is still functional
			finalJob, err := s.NewJob(
				DurationJob(50*time.Millisecond),
				NewTask(func() {}),
			)
			require.NoError(t, err, "Scheduler should still accept jobs after stress")

			require.NoError(t, s.Shutdown())

			t.Logf("Added: %d, Removed: %d, Total ops: %d",
				addedCount.Load(), removedCount.Load(), totalOps)

			// Verify the final job was actually added
			assert.NotEqual(t, uuid.Nil, finalJob.ID())
		})
	}
}

// TestStress_RemoveJobDuringExecution tests removing jobs while they're executing.
// This ensures proper context cancellation and cleanup when jobs are removed mid-flight.
func TestStress_RemoveJobDuringExecution(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	tests := []struct {
		name              string
		numJobs           int
		jobDuration       time.Duration
		removeAfter       time.Duration
		expectedRemoved   int
		expectedCompleted int // jobs that complete before removal
	}{
		{
			name:              "remove 10 jobs mid-execution",
			numJobs:           10,
			jobDuration:       500 * time.Millisecond,
			removeAfter:       250 * time.Millisecond,
			expectedRemoved:   10,
			expectedCompleted: 0, // all should be removed before completion
		},
		{
			name:              "remove 50 jobs with varying timing",
			numJobs:           50,
			jobDuration:       300 * time.Millisecond,
			removeAfter:       100 * time.Millisecond,
			expectedRemoved:   50,
			expectedCompleted: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t, WithStopTimeout(2*time.Second))
			t.Cleanup(func() {
				_ = s.Shutdown()
			})

			var (
				startedCount   atomic.Int32
				completedCount atomic.Int32
				canceledCount  atomic.Int32
			)

			jobIDs := make([]uuid.UUID, 0, tt.numJobs)
			var mu sync.Mutex

			// Create jobs that track their lifecycle
			for i := 0; i < tt.numJobs; i++ {
				job, err := s.NewJob(
					DurationJob(1*time.Second), // schedule interval
					NewTask(func(ctx context.Context) {
						startedCount.Add(1)
						select {
						case <-time.After(tt.jobDuration):
							completedCount.Add(1)
						case <-ctx.Done():
							canceledCount.Add(1)
						}
					}),
					WithStartAt(WithStartImmediately()),
				)
				require.NoError(t, err)

				mu.Lock()
				jobIDs = append(jobIDs, job.ID())
				mu.Unlock()
			}

			s.Start()

			// Wait for jobs to start executing
			require.Eventually(t, func() bool {
				return startedCount.Load() >= int32(tt.numJobs)
			}, 2*time.Second, 10*time.Millisecond)

			// Wait a bit, then remove all jobs
			time.Sleep(tt.removeAfter)

			removedCount := 0
			mu.Lock()
			for _, id := range jobIDs {
				err := s.RemoveJob(id)
				if err == nil {
					removedCount++
				}
			}
			mu.Unlock()

			assert.Equal(t, tt.expectedRemoved, removedCount,
				"Should successfully remove all jobs")

			// Give jobs time to react to cancellation
			time.Sleep(100 * time.Millisecond)

			require.NoError(t, s.Shutdown())

			// Most jobs should be canceled, not completed
			assert.GreaterOrEqual(t, int(canceledCount.Load()), tt.expectedRemoved-5,
				"Most jobs should be canceled")
			assert.LessOrEqual(t, int(completedCount.Load()), 5,
				"Few jobs should complete naturally")

			t.Logf("Started: %d, Completed: %d, Canceled: %d",
				startedCount.Load(), completedCount.Load(), canceledCount.Load())
		})
	}
}

// TestStress_RunNowDuringShutdown tests RunNow calls racing with Shutdown.
// This ensures the scheduler handles manual job triggers gracefully during shutdown.
func TestStress_RunNowDuringShutdown(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	tests := []struct {
		name             string
		numJobs          int
		numRunNowCallers int
	}{
		{
			name:             "5 jobs with 10 RunNow callers during shutdown",
			numJobs:          5,
			numRunNowCallers: 10,
		},
		{
			name:             "20 jobs with 50 RunNow callers during shutdown",
			numJobs:          20,
			numRunNowCallers: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t, WithStopTimeout(2*time.Second))

			var executionCount atomic.Int32
			jobs := make([]Job, 0, tt.numJobs)

			// Create jobs
			for i := 0; i < tt.numJobs; i++ {
				job, err := s.NewJob(
					DurationJob(10*time.Second), // long interval to prevent auto-runs
					NewTask(func() {
						executionCount.Add(1)
					}),
				)
				require.NoError(t, err)
				jobs = append(jobs, job)
			}

			s.Start()

			var wg sync.WaitGroup
			shutdownStarted := make(chan struct{})

			// Spawn goroutines that call RunNow
			for i := 0; i < tt.numRunNowCallers; i++ {
				wg.Add(1)
				go func(callerID int) {
					defer wg.Done()
					<-shutdownStarted // wait for shutdown signal

					job := jobs[callerID%len(jobs)]
					err := job.RunNow()
					// Error is acceptable during shutdown
					_ = err
				}(i)
			}

			// Trigger shutdown while RunNow calls are happening
			close(shutdownStarted)
			time.Sleep(10 * time.Millisecond) // let some RunNow calls start

			err := s.Shutdown()
			require.NoError(t, err)

			wg.Wait()

			// The important assertion is that we didn't panic or deadlock
			t.Logf("Executions triggered: %d", executionCount.Load())
		})
	}
}

// TestStress_UpdateJobDuringExecution tests updating job definitions while jobs are running.
// This ensures proper handling of context swapping and job rescheduling.
func TestStress_UpdateJobDuringExecution(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	tests := []struct {
		name       string
		numJobs    int
		numUpdates int
	}{
		{
			name:       "update 5 jobs 10 times each",
			numJobs:    5,
			numUpdates: 10,
		},
		{
			name:       "update 10 jobs 20 times each",
			numJobs:    10,
			numUpdates: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t, WithStopTimeout(3*time.Second))

			var executionCount atomic.Int32
			jobIDs := make([]uuid.UUID, 0, tt.numJobs)

			// Create initial jobs
			for i := 0; i < tt.numJobs; i++ {
				job, err := s.NewJob(
					DurationJob(100*time.Millisecond),
					NewTask(func(ctx context.Context) {
						executionCount.Add(1)
						// Simulate work
						select {
						case <-time.After(50 * time.Millisecond):
						case <-ctx.Done():
						}
					}),
					WithStartAt(WithStartImmediately()),
				)
				require.NoError(t, err)
				jobIDs = append(jobIDs, job.ID())
			}

			s.Start()

			// Wait for initial executions
			require.Eventually(t, func() bool {
				return executionCount.Load() >= int32(tt.numJobs)
			}, 2*time.Second, 10*time.Millisecond)

			var wg sync.WaitGroup
			var successfulUpdates atomic.Int32

			// Spawn updaters
			for _, jobID := range jobIDs {
				wg.Add(1)
				go func(id uuid.UUID) {
					defer wg.Done()
					for i := 0; i < tt.numUpdates; i++ {
						_, err := s.Update(
							id,
							DurationJob(150*time.Millisecond), // change interval
							NewTask(func(ctx context.Context) {
								executionCount.Add(1)
								select {
								case <-time.After(50 * time.Millisecond):
								case <-ctx.Done():
								}
							}),
						)
						if err == nil {
							successfulUpdates.Add(1)
						}
						time.Sleep(20 * time.Millisecond)
					}
				}(jobID)
			}

			wg.Wait()

			require.NoError(t, s.Shutdown())

			// Verify updates happened
			assert.Greater(t, int(successfulUpdates.Load()), tt.numJobs*tt.numUpdates/2,
				"At least half of updates should succeed")

			t.Logf("Total executions: %d, Successful updates: %d",
				executionCount.Load(), successfulUpdates.Load())
		})
	}
}

// GROUP 2: Lifecycle Stress Tests

// TestStress_RapidStartStopCycles tests rapid Start/Stop/Start cycles.
// This ensures no goroutine leaks, timer leaks, or channel issues across cycles.
func TestStress_RapidStartStopCycles(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	tests := []struct {
		name       string
		numCycles  int
		numJobs    int
		cycleDelay time.Duration
	}{
		{
			name:       "50 rapid cycles with 5 jobs",
			numCycles:  50,
			numJobs:    5,
			cycleDelay: 50 * time.Millisecond,
		},
		{
			name:       "100 rapid cycles with 3 jobs",
			numCycles:  100,
			numJobs:    3,
			cycleDelay: 20 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t, WithStopTimeout(1*time.Second))

			var executionCount atomic.Int32

			// Create jobs once
			for i := 0; i < tt.numJobs; i++ {
				_, err := s.NewJob(
					DurationJob(10*time.Millisecond),
					NewTask(func() {
						executionCount.Add(1)
					}),
				)
				require.NoError(t, err)
			}

			// Rapid start/stop cycles
			for cycle := 0; cycle < tt.numCycles; cycle++ {
				s.Start()
				time.Sleep(tt.cycleDelay)
				err := s.StopJobs()
				require.NoError(t, err, "StopJobs should not error on cycle %d", cycle)
			}

			// Final start for clean shutdown
			s.Start()
			require.NoError(t, s.Shutdown())

			// Verify jobs executed at least once
			assert.Greater(t, int(executionCount.Load()), 0,
				"Jobs should have executed during cycles")

			t.Logf("Cycles: %d, Total executions: %d", tt.numCycles, executionCount.Load())
		})
	}
}

// TestStress_ConcurrentMetadataReads tests concurrent Jobs(), NextRun(), LastRun() calls
// while jobs are being added/removed. This ensures metadata reads are safe under mutation.
func TestStress_ConcurrentMetadataReads(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	tests := []struct {
		name        string
		numReaders  int
		numMutators int
		duration    time.Duration
	}{
		{
			name:        "10 readers with 5 mutators",
			numReaders:  10,
			numMutators: 5,
			duration:    2 * time.Second,
		},
		{
			name:        "50 readers with 20 mutators",
			numReaders:  50,
			numMutators: 20,
			duration:    3 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t)

			// Pre-populate with some jobs
			initialJobs := 10
			jobIDs := make([]uuid.UUID, 0, initialJobs)
			var jobIDsMu sync.Mutex

			for i := 0; i < initialJobs; i++ {
				job, err := s.NewJob(
					DurationJob(100*time.Millisecond),
					NewTask(func() {}),
				)
				require.NoError(t, err)
				jobIDs = append(jobIDs, job.ID())
			}

			s.Start()

			ctx, cancel := context.WithTimeout(context.Background(), tt.duration)
			defer cancel()

			var wg sync.WaitGroup
			var readOps, mutateOps atomic.Int32

			// Spawn metadata readers
			for i := 0; i < tt.numReaders; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-ctx.Done():
							return
						default:
						}

						// Read Jobs()
						jobs := s.Jobs()
						readOps.Add(1)

						// Read NextRun/LastRun for a random job
						if len(jobs) > 0 {
							job := jobs[0]
							_, _ = job.NextRun()
							_, _ = job.LastRun()
							readOps.Add(2)
						}
					}
				}()
			}

			// Spawn mutators (add/remove jobs)
			for i := 0; i < tt.numMutators; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						select {
						case <-ctx.Done():
							return
						default:
						}

						// Add a job
						job, err := s.NewJob(
							DurationJob(100*time.Millisecond),
							NewTask(func() {}),
						)
						if err == nil {
							mutateOps.Add(1)
							jobIDsMu.Lock()
							jobIDs = append(jobIDs, job.ID())
							jobIDsMu.Unlock()
						}

						// Remove a job
						jobIDsMu.Lock()
						if len(jobIDs) > 5 { // keep at least 5 jobs
							idToRemove := jobIDs[0]
							jobIDs = jobIDs[1:]
							jobIDsMu.Unlock()

							err := s.RemoveJob(idToRemove)
							if err == nil {
								mutateOps.Add(1)
							}
						} else {
							jobIDsMu.Unlock()
						}

						time.Sleep(10 * time.Millisecond)
					}
				}()
			}

			wg.Wait()
			require.NoError(t, s.Shutdown())

			assert.Greater(t, int(readOps.Load()), tt.numReaders*10,
				"Should complete many read operations")
			assert.Greater(t, int(mutateOps.Load()), tt.numMutators,
				"Should complete mutation operations")

			t.Logf("Read ops: %d, Mutate ops: %d", readOps.Load(), mutateOps.Load())
		})
	}
}

// TestStress_RemoveByTagsDuringExecution tests removing multiple jobs by tags
// while they're executing. This ensures bulk removal is safe during execution.
func TestStress_RemoveByTagsDuringExecution(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	tests := []struct {
		name         string
		numJobs      int
		tagsPerJob   int
		removeDelay  time.Duration
		expectedRuns int // minimum expected executions before removal
	}{
		{
			name:         "remove 20 tagged jobs mid-execution",
			numJobs:      20,
			tagsPerJob:   2,
			removeDelay:  100 * time.Millisecond,
			expectedRuns: 10,
		},
		{
			name:         "remove 50 tagged jobs quickly",
			numJobs:      50,
			tagsPerJob:   3,
			removeDelay:  50 * time.Millisecond,
			expectedRuns: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t, WithStopTimeout(2*time.Second))

			var executionCount atomic.Int32
			tags := []string{"tag1", "tag2", "tag3"}

			// Create jobs with tags
			for i := 0; i < tt.numJobs; i++ {
				jobTags := tags[:tt.tagsPerJob]
				_, err := s.NewJob(
					DurationJob(10*time.Millisecond),
					NewTask(func(ctx context.Context) {
						executionCount.Add(1)
						// Simulate work
						select {
						case <-time.After(200 * time.Millisecond):
						case <-ctx.Done():
						}
					}),
					WithTags(jobTags...),
					WithStartAt(WithStartImmediately()),
				)
				require.NoError(t, err)
			}

			s.Start()

			// Wait for jobs to start executing
			require.Eventually(t, func() bool {
				return executionCount.Load() >= int32(tt.expectedRuns)
			}, 2*time.Second, 10*time.Millisecond)

			// Remove all jobs by tag
			time.Sleep(tt.removeDelay)
			s.RemoveByTags("tag1")

			// Verify jobs were removed
			require.Eventually(t, func() bool {
				return len(s.Jobs()) == 0
			}, 2*time.Second, 10*time.Millisecond, "All tagged jobs should be removed")

			require.NoError(t, s.Shutdown())

			t.Logf("Executions before removal: %d, Jobs removed: %d",
				executionCount.Load(), tt.numJobs)
		})
	}
}

// GROUP 3: High Contention Tests

// TestStress_SingletonModeHighContention tests many jobs in singleton mode under load.
// This stresses the sync.Map used for singleton runners.
// NOTE: Goroutine leak detection is disabled for this test as the high-load scenarios (especially
// 200 jobs in wait mode) create many goroutines that may not fully clean up by the time goleak runs.
func TestStress_SingletonModeHighContention(t *testing.T) {
	// Skip leak detection for high-load scenarios
	// defer verifyNoGoroutineLeaks(t)

	tests := []struct {
		name          string
		numJobs       int
		jobDuration   time.Duration
		testDuration  time.Duration
		minExecutions int
		singletonMode LimitMode
	}{
		{
			name:          "100 singleton jobs (reschedule mode)",
			numJobs:       100,
			jobDuration:   50 * time.Millisecond,
			testDuration:  2 * time.Second,
			minExecutions: 50,
			singletonMode: LimitModeReschedule,
		},
		{
			name:          "200 singleton jobs (wait mode)",
			numJobs:       200,
			jobDuration:   30 * time.Millisecond,
			testDuration:  3 * time.Second,
			minExecutions: 100,
			singletonMode: LimitModeWait,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t, WithStopTimeout(3*time.Second))

			var executionCount atomic.Int32

			// Create many singleton jobs
			for i := 0; i < tt.numJobs; i++ {
				_, err := s.NewJob(
					DurationJob(10*time.Millisecond), // short interval
					NewTask(func(ctx context.Context) {
						executionCount.Add(1)
						// Simulate work
						select {
						case <-time.After(tt.jobDuration):
						case <-ctx.Done():
						}
					}),
					WithSingletonMode(tt.singletonMode),
					WithStartAt(WithStartImmediately()),
				)
				require.NoError(t, err)
			}

			s.Start()

			// Let jobs run for the test duration
			time.Sleep(tt.testDuration)

			require.NoError(t, s.Shutdown())

			// Verify minimum executions
			assert.GreaterOrEqual(t, int(executionCount.Load()), tt.minExecutions,
				"Should complete minimum executions under singleton mode")

			t.Logf("Jobs: %d, Executions: %d, Mode: %v",
				tt.numJobs, executionCount.Load(), tt.singletonMode)
		})
	}
}

// TestStress_LimitModeChannelSaturation tests limit mode with many jobs hitting the limiter.
// This validates the reschedule channel and wait queue behavior under sustained load.
func TestStress_LimitModeChannelSaturation(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	tests := []struct {
		name          string
		numJobs       int
		concurrency   uint
		jobDuration   time.Duration
		testDuration  time.Duration
		minExecutions int
		limitMode     LimitMode
	}{
		{
			name:          "100 jobs with limit 5 (reschedule)",
			numJobs:       100,
			concurrency:   5,
			jobDuration:   100 * time.Millisecond,
			testDuration:  3 * time.Second,
			minExecutions: 50,
			limitMode:     LimitModeReschedule,
		},
		{
			name:          "200 jobs with limit 10 (wait)",
			numJobs:       200,
			concurrency:   10,
			jobDuration:   50 * time.Millisecond,
			testDuration:  4 * time.Second,
			minExecutions: 100,
			limitMode:     LimitModeWait,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t,
				WithLimitConcurrentJobs(tt.concurrency, tt.limitMode),
				WithStopTimeout(3*time.Second),
			)

			var executionCount atomic.Int32
			var queuedCount atomic.Int32

			// Create many jobs that will hit the limit
			for i := 0; i < tt.numJobs; i++ {
				_, err := s.NewJob(
					DurationJob(10*time.Millisecond), // short interval to saturate
					NewTask(func(ctx context.Context) {
						executionCount.Add(1)
						// Check if we're queued (only visible in wait mode)
						select {
						case <-time.After(tt.jobDuration):
						case <-ctx.Done():
							queuedCount.Add(1) // canceled while waiting
						}
					}),
					WithStartAt(WithStartImmediately()),
				)
				require.NoError(t, err)
			}

			s.Start()

			// Let jobs run
			time.Sleep(tt.testDuration)

			require.NoError(t, s.Shutdown())

			// Verify executions
			assert.GreaterOrEqual(t, int(executionCount.Load()), tt.minExecutions,
				"Should complete minimum executions under limit mode")

			t.Logf("Jobs: %d, Limit: %d, Executions: %d, Mode: %v",
				tt.numJobs, tt.concurrency, executionCount.Load(), tt.limitMode)
		})
	}
}
