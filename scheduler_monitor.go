package gocron

// SchedulerMonitor is called by the Scheduler to provide scheduler-level
// metrics and events.
type SchedulerMonitor interface {
	// SchedulerStarted is called when Start() is invoked on the scheduler.
	SchedulerStarted()

	// SchedulerShutdown is called when Shutdown() completes successfully.
	SchedulerShutdown()

	// JobRegistered is called when a job is registered with the scheduler.
	JobRegistered(job *Job)

	// JobUnregistered is called when a job is unregistered from the scheduler.
	JobUnregistered(job *Job)

	// JobStarted is called when a job starts running.
	JobStarted(job *Job)

	// JobRunning is called when a job is running.
	JobRunning(job *Job)

	// JobFailed is called when a job fails to complete successfully.
	JobFailed(job *Job, err error)

	// JobCompleted is called when a job has completed running.
	JobCompleted(job *Job)
}
