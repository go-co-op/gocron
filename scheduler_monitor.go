package gocron

import "time"

// SchedulerMonitor is called by the Scheduler to provide scheduler-level
// metrics and events.
type SchedulerMonitor interface {
	// SchedulerStarted is called when Start() is invoked on the scheduler.
	SchedulerStarted()

	// SchedulerStopped is called when the scheduler's main loop stops,
	// but before final cleanup in Shutdown().
	SchedulerStopped()

	// SchedulerShutdown is called when Shutdown() completes successfully.
	SchedulerShutdown()

	// JobRegistered is called when a job is registered with the scheduler.
	JobRegistered(job Job)

	// JobUnregistered is called when a job is unregistered from the scheduler.
	JobUnregistered(job Job)

	// JobStarted is called when a job starts running.
	JobStarted(job Job)

	// JobRunning is called when a job is running.
	JobRunning(job Job)

	// JobFailed is called when a job fails to complete successfully.
	JobFailed(job Job, err error)

	// JobCompleted is called when a job has completed running.
	JobCompleted(job Job)

	// JobExecutionTime is called after a job completes (success or failure)
	// with the time it took to execute. This enables calculation of metrics
	// like AverageExecutionTime.
	JobExecutionTime(job Job, duration time.Duration)

	// JobSchedulingDelay is called when a job starts running, providing both
	// the scheduled time and actual start time. This enables calculation of
	// SchedulingLag metrics to detect when jobs are running behind schedule.
	JobSchedulingDelay(job Job, scheduledTime time.Time, actualStartTime time.Time)

	// ConcurrencyLimitReached is called when a job cannot start immediately
	// due to concurrency limits (singleton or limit mode).
	// limitType will be "singleton" or "limit".
	ConcurrencyLimitReached(limitType string, job Job)
}
