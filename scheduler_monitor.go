package gocron

// SchedulerMonitor is called by the Scheduler to provide scheduler-level
// metrics and events.
type SchedulerMonitor interface {
	// SchedulerStarted is called when Start() is invoked on the scheduler.
	SchedulerStarted()

	// SchedulerStopped is called when Shutdown() completes successfully.
	SchedulerStopped()
}
