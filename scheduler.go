//go:generate mockgen -destination=mocks/scheduler.go -package=gocronmocks . Scheduler
package gocron

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"runtime"
	"slices"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
)

// Default channel buffer sizes and RPC timeouts for scheduler internals.
// Extracted from previously-inlined magic numbers; not exposed via
// SchedulerOption yet (that would be an additive public-API change and
// deserves its own design). Values chosen to match the historical
// behavior on the v2 line.
const (
	// defaultJobOutRequestBuffer bounds the queue of internal
	// job-lookup requests (Job.LastRun, Job.NextRun, etc.).
	defaultJobOutRequestBuffer = 100
	// defaultJobTimingBuffer bounds the queue of internal
	// job-timing updates from the executor.
	defaultJobTimingBuffer = 100
	// defaultLimitModeQueueBuffer bounds the per-LimitMode job
	// queue. Beyond this, LimitModeWait callers block; see the
	// LimitMode docs.
	defaultLimitModeQueueBuffer = 1000
	// defaultSingletonQueueBuffer bounds the per-job queue for
	// singleton-mode jobs.
	defaultSingletonQueueBuffer = 1000
	// defaultRunNowSendTimeout bounds how long Job.RunNow will
	// wait to hand its request to the scheduler goroutine before
	// giving up with ErrJobRunNowFailed.
	defaultRunNowSendTimeout = 100 * time.Millisecond
	// defaultRunNowResultTimeout bounds how long Job.RunNow will
	// wait for a result after the request is queued.
	defaultRunNowResultTimeout = time.Second
	// defaultRequestJobTimeout bounds how long a Job.X() accessor
	// waits for the scheduler goroutine to respond before returning
	// ErrSchedulerBusy.
	defaultRequestJobTimeout = time.Second
)

var _ Scheduler = (*scheduler)(nil)

// Scheduler defines the interface for the Scheduler.
type Scheduler interface {
	// Jobs returns all the jobs currently in the scheduler.
	//
	// The returned slice is sorted by job UUID as raw bytes, giving a
	// deterministic-but-effectively-random ordering. Callers that need
	// a specific order (by name, insertion order, etc.) should re-sort
	// the returned slice themselves.
	//
	// If the scheduler has been shut down, or shuts down before this
	// call completes, Jobs returns nil, which is indistinguishable
	// from a scheduler with zero jobs. This behavior is retained for
	// backward compatibility; callers that need to disambiguate should
	// track scheduler lifecycle explicitly or coordinate via
	// Shutdown()'s return value.
	Jobs() []Job
	// NewJob creates a new job in the Scheduler. The job is scheduled per the provided
	// definition when the Scheduler is started. If the Scheduler is already running
	// the job will be scheduled when the Scheduler is started.
	// If you set the first argument of your Task func to be a context.Context,
	// gocron will pass in a context (either the default Job context, or one
	// provided via WithContext) to the job and will cancel the context on shutdown.
	// This allows you to listen for and handle cancellation within your job.
	NewJob(JobDefinition, Task, ...JobOption) (Job, error)
	// RemoveByTags removes all jobs that have at least one of the provided tags.
	RemoveByTags(...string)
	// RemoveJob removes the job with the provided id.
	RemoveJob(uuid.UUID) error
	// Shutdown should be called when you no longer need
	// the Scheduler or Job's as the Scheduler cannot
	// be restarted after calling Shutdown. This is similar
	// to a Close or Cleanup method and is often deferred after
	// starting the scheduler.
	Shutdown() error
	// ShutdownWithContext behaves like Shutdown but respects the provided context's deadline.
	ShutdownWithContext(context.Context) error
	// Start begins scheduling jobs for execution based
	// on each job's definition. Job's added to an already
	// running scheduler will be scheduled immediately based
	// on definition. Start is non-blocking.
	Start()
	// StopJobs stops the execution of all jobs in the scheduler.
	// This can be useful in situations where jobs need to be
	// paused globally and then restarted with Start().
	StopJobs() error
	// StopJobsWithContext behaves like StopJobs but respects the provided context's deadline.
	StopJobsWithContext(context.Context) error
	// Update replaces the existing Job's JobDefinition with the provided
	// JobDefinition. The Job's Job.ID() remains the same.
	Update(uuid.UUID, JobDefinition, Task, ...JobOption) (Job, error)
	// JobsWaitingInQueue number of jobs waiting in Queue in case of LimitModeWait
	// In case of LimitModeReschedule or no limit it will be always zero
	JobsWaitingInQueue() int
}

// -----------------------------------------------
// -----------------------------------------------
// ----------------- Scheduler -------------------
// -----------------------------------------------
// -----------------------------------------------

type scheduler struct {
	// context used for shutting down
	shutdownCtx context.Context
	// cancel used to signal scheduler should shut down
	shutdownCancel context.CancelFunc
	// the executor, which actually runs the jobs sent to it via the scheduler
	exec executor
	// the map of jobs registered in the scheduler
	jobs map[uuid.UUID]internalJob
	// the location used by the scheduler for scheduling when relevant
	location *time.Location
	// whether the scheduler has been started or not
	started atomic.Bool
	// globally applied JobOption's set on all jobs added to the scheduler
	// note: individually set JobOption's take precedence.
	globalJobOptions []JobOption
	// the scheduler's logger
	logger Logger

	// used to tell the scheduler to start
	startCh chan struct{}
	// used to report that the scheduler has started
	startedCh chan struct{}
	// used to tell the scheduler to stop
	stopCh chan struct{}
	// used to report that the scheduler has stopped
	stopErrCh chan error
	// used to send all the jobs out when a request is made by the client
	allJobsOutRequest chan allJobsOutRequest
	// used to send a jobs out when a request is made by the client
	jobOutRequestCh chan *jobOutRequest
	// used to run a job on-demand when requested by the client
	runJobRequestCh chan runJobRequest
	// new jobs are received here
	newJobCh chan newJobIn
	// requests from the client to remove jobs by ID are received here
	removeJobCh chan uuid.UUID
	// requests from the client to remove jobs by tags are received here
	removeJobsByTagsCh chan []string

	// scheduler monitor from which metrics can be collected
	schedulerMonitor SchedulerMonitor
}

type newJobIn struct {
	ctx    context.Context
	cancel context.CancelFunc
	job    internalJob
}

type jobOutRequest struct {
	id      uuid.UUID
	outChan chan internalJob
}

type runJobRequest struct {
	id      uuid.UUID
	outChan chan error
}

type allJobsOutRequest struct {
	outChan chan []Job
}

// NewScheduler creates a new Scheduler instance.
// The Scheduler is not started until Start() is called.
//
// NewJob will add jobs to the Scheduler, but they will not
// be scheduled until Start() is called.
func NewScheduler(options ...SchedulerOption) (Scheduler, error) {
	schCtx, cancel := context.WithCancel(context.Background())

	exec := executor{
		stopCh:           make(chan struct{}),
		stopTimeout:      time.Second * 10,
		singletonRunners: nil,
		logger:           &noOpLogger{},
		clock:            clockwork.NewRealClock(),

		jobsIn:                 make(chan jobIn),
		jobsOutForRescheduling: make(chan uuid.UUID),
		jobUpdateNextRuns:      make(chan uuid.UUID),
		jobsOutCompleted:       make(chan jobOutCompleted),
		jobOutRequest:          make(chan *jobOutRequest, defaultJobOutRequestBuffer),
		done:                   make(chan error, 1),
		jobTimingUpdateCh:      make(chan jobTimingUpdate, defaultJobTimingBuffer),
	}

	s := &scheduler{
		shutdownCtx:    schCtx,
		shutdownCancel: cancel,
		jobs:           make(map[uuid.UUID]internalJob),
		location:       time.Local,
		logger:         &noOpLogger{},

		newJobCh:           make(chan newJobIn),
		removeJobCh:        make(chan uuid.UUID),
		removeJobsByTagsCh: make(chan []string),
		startCh:            make(chan struct{}),
		startedCh:          make(chan struct{}),
		stopCh:             make(chan struct{}),
		stopErrCh:          make(chan error, 1),
		jobOutRequestCh:    make(chan *jobOutRequest),
		runJobRequestCh:    make(chan runJobRequest),
		allJobsOutRequest:  make(chan allJobsOutRequest),
	}
	exec.scheduler = s
	s.exec = exec

	for _, option := range options {
		err := option(s)
		if err != nil {
			return nil, err
		}
	}

	go func() {
		s.logger.Info("gocron: new scheduler created")
		for {
			select {
			case id := <-s.exec.jobsOutForRescheduling:
				s.selectExecJobsOutForRescheduling(id)
			case id := <-s.exec.jobUpdateNextRuns:
				s.updateNextScheduled(id)
			case completed := <-s.exec.jobsOutCompleted:
				s.selectExecJobsOutCompleted(completed)

			case update := <-s.exec.jobTimingUpdateCh:
				s.selectJobTimingUpdate(update)

			case in := <-s.newJobCh:
				s.selectNewJob(in)

			case id := <-s.removeJobCh:
				s.selectRemoveJob(id)

			case tags := <-s.removeJobsByTagsCh:
				s.selectRemoveJobsByTags(tags)

			case out := <-s.exec.jobOutRequest:
				s.selectJobOutRequest(out)

			case out := <-s.jobOutRequestCh:
				s.selectJobOutRequest(out)

			case out := <-s.allJobsOutRequest:
				s.selectAllJobsOutRequest(out)

			case run := <-s.runJobRequestCh:
				s.selectRunJobRequest(run)

			case <-s.startCh:
				s.selectStart()

			case <-s.stopCh:
				s.stopScheduler()

			case <-s.shutdownCtx.Done():
				s.stopScheduler()
				return
			}
		}
	}()

	return s, nil
}

// -----------------------------------------------
// -----------------------------------------------
// --------- Scheduler Channel Methods -----------
// -----------------------------------------------
// -----------------------------------------------

// The scheduler's channel functions are broken out here
// to allow prioritizing within the select blocks. The idea
// being that we want to make sure that scheduling tasks
// are not blocked by requests from the caller for information
// about jobs.

func (s *scheduler) stopScheduler() {
	s.logger.Debug("gocron: stopping scheduler")

	if s.started.Load() {
		s.exec.stopCh <- struct{}{}
	}

	for _, j := range s.jobs {
		j.stop()
	}
	for _, j := range s.jobs {
		<-j.ctx.Done()
	}
	var err error
	if s.started.Load() {
		t := time.NewTimer(s.exec.stopTimeout + 1*time.Second)
		select {
		case err = <-s.exec.done:
			t.Stop()
		case <-t.C:
			err = ErrStopExecutorTimedOut
		}
	}
	for id, j := range s.jobs {
		oldCtx := j.ctx
		if j.parentCtx == nil {
			j.parentCtx = s.shutdownCtx
		}
		j.ctx, j.cancel = context.WithCancel(j.parentCtx)

		// also replace the old context with the new one in the parameters
		if len(j.parameters) > 0 && j.parameters[0] == oldCtx {
			j.parameters[0] = j.ctx
		}

		s.jobs[id] = j
	}

	s.stopErrCh <- err
	s.started.Store(false)
	s.logger.Debug("gocron: scheduler stopped")

	// Notify monitor that scheduler has stopped
	s.notifySchedulerStopped()
}

// selectAllJobsOutRequest handles Scheduler.Jobs() calls. Snapshots the
// current jobs map into an owned []Job and sends it on out.outChan. The
// snapshot is sorted by raw UUID bytes; see Scheduler.Jobs() for the
// ordering rationale.
func (s *scheduler) selectAllJobsOutRequest(out allJobsOutRequest) {
	outJobs := make([]Job, len(s.jobs))
	var counter int
	for _, j := range s.jobs {
		outJobs[counter] = s.jobFromInternalJob(j)
		counter++
	}
	slices.SortFunc(outJobs, func(a, b Job) int {
		aID, bID := a.ID(), b.ID()
		return bytes.Compare(aID[:], bID[:])
	})
	select {
	case <-s.shutdownCtx.Done():
	case out.outChan <- outJobs:
	}
}

// selectRunJobRequest handles Job.RunNow() calls. Forwards the job to
// the executor's jobsIn channel and reports the outcome (or
// ErrJobNotFound / shutdown) back on run.outChan. Waits on jobsIn
// rather than dropping, so callers get accurate backpressure signal
// bounded by defaultRunNowSendTimeout.
func (s *scheduler) selectRunJobRequest(run runJobRequest) {
	j, ok := s.jobs[run.id]
	if !ok {
		select {
		case run.outChan <- ErrJobNotFound:
		case <-s.shutdownCtx.Done():
		}
		return
	}
	select {
	case <-s.shutdownCtx.Done():
		select {
		case run.outChan <- ErrJobRunNowFailed:
		case <-s.shutdownCtx.Done():
		}
	case s.exec.jobsIn <- jobIn{
		id:            j.id,
		shouldSendOut: false,
	}:
		select {
		case run.outChan <- nil:
		case <-s.shutdownCtx.Done():
		}
	}
}

// selectRemoveJob deletes a single job by id and cancels its running
// context. No-op if the id is unknown.
func (s *scheduler) selectRemoveJob(id uuid.UUID) {
	j, ok := s.jobs[id]
	if !ok {
		return
	}
	if s.schedulerMonitor != nil {
		out := s.jobFromInternalJob(j)
		s.notifyJobUnregistered(out)
	}
	j.stop()
	delete(s.jobs, id)
}

// advancePastNow advances next via j.next until it is no longer before s.now(),
// returning the new time and ok=true. Returns ok=false if next() ever produces
// the zero time or fails to make forward progress (which would otherwise spin
// the scheduler goroutine forever. Callers should treat ok=false the same way
// they treat an exhausted schedule and remove the job.
func (s *scheduler) advancePastNow(j internalJob, next time.Time) (time.Time, bool) {
	for next.Before(s.now()) {
		n := j.next(next)
		if n.IsZero() || !n.After(next) {
			return time.Time{}, false
		}
		next = n
	}
	return next, true
}

// selectExecJobsOutForRescheduling handles the executor's post-run
// notification for a job that just started. Advances j.nextRun past
// now, updates the timer, and appends to nextScheduled. No-op if the
// job was removed while running.
func (s *scheduler) selectExecJobsOutForRescheduling(id uuid.UUID) {
	select {
	case <-s.shutdownCtx.Done():
		return
	default:
	}
	j, ok := s.jobs[id]
	if !ok {
		// the job was removed while it was running, and
		// so we don't need to reschedule it.
		return
	}

	if j.stopTimeReached(s.now()) {
		s.selectRemoveJob(id)
		return
	}

	var scheduleFrom time.Time

	// If intervalFromCompletion is enabled, calculate the next run time
	// from when the job completed (lastRun) rather than when it was scheduled.
	if j.intervalFromCompletion {
		// Use the completion time (lastRun is set when the job completes)
		scheduleFrom = j.lastRun
		if scheduleFrom.IsZero() {
			// For the first run, use the start time or current time
			scheduleFrom = j.startTime
			if scheduleFrom.IsZero() {
				scheduleFrom = s.now()
			}
		}
	} else {
		// Default behavior: use the scheduled time
		if len(j.nextScheduled) > 0 {
			// always grab the last element in the slice as that is the furthest
			// out in the future and the time from which we want to calculate
			// the subsequent next run time.
			slices.SortStableFunc(j.nextScheduled, ascendingTime)
			scheduleFrom = j.nextScheduled[len(j.nextScheduled)-1]
		}

		if scheduleFrom.IsZero() {
			scheduleFrom = j.startTime
		}
	}

	next := j.next(scheduleFrom)
	if next.IsZero() {
		// the job's next function will return zero for OneTime jobs.
		// since they are one time only, they do not need rescheduling.
		return
	}

	if next.Before(s.now()) {
		// in some cases the next run time can be in the past, for example:
		// - the time on the machine was incorrect and has been synced with ntp
		// - the machine went to sleep, and woke up some time later
		// in those cases, we want to increment to the next run in the future
		// and schedule the job for that time.
		var ok bool
		next, ok = s.advancePastNow(j, next)
		if !ok {
			s.selectRemoveJob(id)
			return
		}
	}

	if nextScheduledContains(j.nextScheduled, next) {
		// if the next value is a duplicate of what's already in the nextScheduled slice, for example:
		// - the job is being rescheduled off the same next run value as before
		// increment to the next, next value
		for nextScheduledContains(j.nextScheduled, next) {
			next = j.next(next)
		}
	}

	if !j.stopTime.IsZero() && !next.Before(j.stopTime) {
		s.selectRemoveJob(id)
		return
	}

	// Clean up any existing timer to prevent leaks
	if j.timer != nil {
		j.timer.Stop()
		j.timer = nil // Ensure timer is cleared for GC
	}

	j.nextScheduled = insertNextScheduled(j.nextScheduled, next)
	j.timer = s.exec.clock.AfterFunc(next.Sub(s.now()), func() {
		// set the actual timer on the job here and listen for
		// shut down events so that the job doesn't attempt to
		// run if the scheduler has been shutdown.
		select {
		case <-s.shutdownCtx.Done():
			return
		case s.exec.jobsIn <- jobIn{
			id:            j.id,
			shouldSendOut: true,
		}:
		}
	})
	// update the job with its new next and last run times and timer.
	s.jobs[id] = j
}

func (s *scheduler) updateNextScheduled(id uuid.UUID) {
	j, ok := s.jobs[id]
	if !ok {
		return
	}
	j.pruneStaleScheduled(s.now())
	s.jobs[id] = j
}

// selectExecJobsOutCompleted handles the executor's post-run
// notification for a completed run. Records lastRun, prunes past
// entries from j.nextScheduled, and evaluates the WithLimitedRuns
// stop condition. Runs that were skipped before execution do NOT
// arrive here (see C3 in Plan #3).
func (s *scheduler) selectExecJobsOutCompleted(completed jobOutCompleted) {
	j, ok := s.jobs[completed.id]
	if !ok {
		return
	}

	// if the job has nextScheduled time in the past,
	// we need to remove any that are in the past or at the current time (just executed).
	j.pruneStaleScheduled(s.now())

	// Skipped runs (for example, when BeforeJobRunsSkipIfBeforeFuncErrors
	// returns an error) don't consume a WithLimitedRuns slot and don't
	// update lastRun — the task function never executed.
	if completed.skipped {
		s.jobs[completed.id] = j
		return
	}

	// if the job has a limited number of runs set, we need to
	// check how many runs have occurred and stop running this
	// job if it has reached the limit. Removal is deferred until
	// the task function actually completes (signaled via a non-zero
	// completedAt on jobTimingUpdateCh) so that the job's context is
	// not canceled while the task is still executing. See #925.
	if j.limitRunsTo != nil {
		j.limitRunsTo.runCount = j.limitRunsTo.runCount + 1
		if j.limitRunsTo.runCount >= j.limitRunsTo.limit {
			s.jobs[completed.id] = j
			return
		}
	}

	j.lastRun = s.now()
	s.jobs[completed.id] = j
}

// selectJobTimingUpdate applies a start/stop-time change to an
// existing job while the scheduler is running, re-evaluating
// nextRun so the change takes effect on the next tick.
func (s *scheduler) selectJobTimingUpdate(update jobTimingUpdate) {
	j, ok := s.jobs[update.id]
	if !ok {
		return
	}
	if !update.startedAt.IsZero() {
		j.lastRunStartedAt = update.startedAt
	}
	if !update.completedAt.IsZero() {
		j.lastRunCompletedAt = update.completedAt
	}
	s.jobs[update.id] = j

	// If the job has hit its WithLimitedRuns limit and the task function
	// has finished (completedAt is set), remove the job now. This is done
	// here rather than in selectExecJobsOutCompleted to avoid canceling
	// the job's context while the task is still running. See #925.
	if !update.completedAt.IsZero() && j.limitRunsTo != nil && j.limitRunsTo.runCount >= j.limitRunsTo.limit {
		go func(id uuid.UUID) {
			select {
			case <-s.shutdownCtx.Done():
				return
			case s.removeJobCh <- id:
			}
		}(update.id)
	}
}

// selectJobOutRequest handles Job.X() accessor queries (LastRun,
// NextRun, IsRunning, etc.). If the id is unknown the outChan is
// closed WITHOUT a send, which requestJobCtx interprets as
// ErrJobNotFound. A slow/absent receiver is bounded by the caller's
// requestJob timeout (surfaced as ErrSchedulerBusy).
func (s *scheduler) selectJobOutRequest(out *jobOutRequest) {
	if j, ok := s.jobs[out.id]; ok {
		select {
		case out.outChan <- j:
		case <-s.shutdownCtx.Done():
		}
	}
	close(out.outChan)
}

// selectNewJob installs a job produced by addOrUpdateJob into s.jobs.
// Runs the job's initial nextRun computation and, if the scheduler is
// already started, wires it into the executor immediately. Signals
// completion via in.cancel() so NewJob can return.
func (s *scheduler) selectNewJob(in newJobIn) {
	j := in.job
	if s.started.Load() {
		next := j.startTime
		if j.startImmediately {
			next = s.now()
			select {
			case <-s.shutdownCtx.Done():
			case s.exec.jobsIn <- jobIn{
				id:            j.id,
				shouldSendOut: true,
			}:
			}
		} else {
			if next.IsZero() {
				next = j.next(s.now())
			}

			if next.Before(s.now()) {
				var ok bool
				next, ok = s.advancePastNow(j, next)
				if !ok {
					s.jobs[j.id] = j
					in.cancel()
					s.selectRemoveJob(j.id)
					return
				}
			}

			if !j.stopTime.IsZero() && !next.Before(j.stopTime) {
				s.jobs[j.id] = j
				in.cancel()
				s.selectRemoveJob(j.id)
				return
			}

			id := j.id
			j.timer = s.exec.clock.AfterFunc(next.Sub(s.now()), func() {
				select {
				case <-s.shutdownCtx.Done():
				case s.exec.jobsIn <- jobIn{
					id:            id,
					shouldSendOut: true,
				}:
				}
			})
		}
		j.startTime = next
		j.nextScheduled = insertNextScheduled(j.nextScheduled, next)
	}

	s.jobs[j.id] = j
	in.cancel()
}

// selectRemoveJobsByTags deletes every job whose tag set intersects
// tags. Cancels each removed job's running context.
func (s *scheduler) selectRemoveJobsByTags(tags []string) {
	for _, j := range s.jobs {
		for _, tag := range tags {
			if slices.Contains(j.tags, tag) {
				if s.schedulerMonitor != nil {
					out := s.jobFromInternalJob(j)
					s.notifyJobUnregistered(out)
				}
				j.stop()
				delete(s.jobs, j.id)
				break
			}
		}
	}
}

func (s *scheduler) selectStart() {
	s.logger.Debug("gocron: scheduler starting")
	go s.exec.start()

	s.started.Store(true)
	for id, j := range s.jobs {
		next := j.startTime
		if j.startImmediately {
			next = s.now()
			select {
			case <-s.shutdownCtx.Done():
			case s.exec.jobsIn <- jobIn{
				id:            id,
				shouldSendOut: true,
			}:
			}
		} else {
			if next.IsZero() {
				next = j.next(s.now())
			}
			if next.Before(s.now()) {
				var ok bool
				next, ok = s.advancePastNow(j, next)
				if !ok {
					s.selectRemoveJob(id)
					continue
				}
			}

			if !j.stopTime.IsZero() && !next.Before(j.stopTime) {
				s.selectRemoveJob(id)
				continue
			}

			jobID := id
			j.timer = s.exec.clock.AfterFunc(next.Sub(s.now()), func() {
				select {
				case <-s.shutdownCtx.Done():
				case s.exec.jobsIn <- jobIn{
					id:            jobID,
					shouldSendOut: true,
				}:
				}
			})
		}
		j.startTime = next
		j.nextScheduled = insertNextScheduled(j.nextScheduled, next)
		s.jobs[id] = j
	}
	select {
	case <-s.shutdownCtx.Done():
	case s.startedCh <- struct{}{}:
		s.logger.Info("gocron: scheduler started")
	}
}

// -----------------------------------------------
// -----------------------------------------------
// ------------- Scheduler Methods ---------------
// -----------------------------------------------
// -----------------------------------------------

func (s *scheduler) now() time.Time {
	return s.exec.clock.Now().In(s.location)
}

func (s *scheduler) jobFromInternalJob(in internalJob) job {
	return job{
		in.id,
		in.name,
		slices.Clone(in.tags),
		s.jobOutRequestCh,
		s.runJobRequestCh,
		s.jobScheduleFromInternal(in.jobSchedule),
	}
}

func (s *scheduler) jobScheduleFromInternal(js jobSchedule) JobSchedule {
	switch v := js.(type) {
	case *cronJob:
		return CronJobSchedule{
			Crontab: v.crontab,
		}
	case *durationJob:
		return DurationJobSchedule{
			Duration: v.duration,
		}
	case *durationRandomJob:
		return DurationRandomJobSchedule{
			Min: v.min,
			Max: v.max,
		}
	case dailyJob:
		return DailyJobSchedule{
			Interval: v.interval,
			AtTimes:  slices.Clone(v.atTimes),
		}
	case weeklyJob:
		return WeeklyJobSchedule{
			Interval:   v.interval,
			DaysOfWeek: slices.Clone(v.daysOfWeek),
			AtTimes:    slices.Clone(v.atTimes),
		}
	case monthlyJob:
		return MonthlyJobSchedule{
			Interval:    v.interval,
			Days:        slices.Clone(v.days),
			DaysFromEnd: slices.Clone(v.daysFromEnd),
			AtTimes:     slices.Clone(v.atTimes),
		}
	case oneTimeJob:
		return OneTimeJobSchedule{
			StartAt: slices.Clone(v.sortedTimes),
		}
	default:
		return nil
	}
}

func (s *scheduler) Jobs() []Job {
	outChan := make(chan []Job)
	select {
	case <-s.shutdownCtx.Done():
	case s.allJobsOutRequest <- allJobsOutRequest{outChan: outChan}:
	}

	var jobs []Job
	select {
	case <-s.shutdownCtx.Done():
	case jobs = <-outChan:
	}

	return jobs
}

func (s *scheduler) NewJob(jobDefinition JobDefinition, task Task, options ...JobOption) (Job, error) {
	return s.addOrUpdateJob(uuid.Nil, jobDefinition, task, options)
}

func (s *scheduler) verifyInterfaceVariadic(taskFunc reflect.Value, tsk task, variadicStart int) error {
	ifaceType := taskFunc.Type().In(variadicStart).Elem()
	for i := variadicStart; i < len(tsk.parameters); i++ {
		if !reflect.TypeOf(tsk.parameters[i]).Implements(ifaceType) {
			return ErrNewJobWrongTypeOfParameters
		}
	}
	return nil
}

func (s *scheduler) verifyVariadic(taskFunc reflect.Value, tsk task, variadicStart int) error {
	if err := s.verifyNonVariadic(taskFunc, tsk, variadicStart); err != nil {
		return err
	}
	parameterType := taskFunc.Type().In(variadicStart)
	parameterTypeKind := parameterType.Elem().Kind()
	if parameterTypeKind == reflect.Interface {
		return s.verifyInterfaceVariadic(taskFunc, tsk, variadicStart)
	}
	if parameterTypeKind == reflect.Pointer {
		parameterTypeKind = reflect.Indirect(reflect.ValueOf(parameterType)).Kind()
	}

	for i := variadicStart; i < len(tsk.parameters); i++ {
		argumentType := reflect.TypeOf(tsk.parameters[i])
		argumentTypeKind := argumentType.Kind()
		if argumentTypeKind == reflect.Interface || argumentTypeKind == reflect.Pointer {
			argumentTypeKind = argumentType.Elem().Kind()
		}
		if argumentTypeKind != parameterTypeKind {
			return ErrNewJobWrongTypeOfParameters
		}
	}
	return nil
}

func (s *scheduler) verifyNonVariadic(taskFunc reflect.Value, tsk task, length int) error {
	for i := 0; i < length; i++ {
		argumentType := reflect.TypeOf(tsk.parameters[i])
		t1 := argumentType.Kind()
		if t1 == reflect.Interface || t1 == reflect.Pointer {
			t1 = argumentType.Elem().Kind()
		}
		parameterType := taskFunc.Type().In(i)
		t2 := reflect.New(parameterType).Elem().Kind()
		if t2 == reflect.Interface || t2 == reflect.Pointer {
			t2 = reflect.Indirect(reflect.ValueOf(parameterType)).Kind()
		}
		if t1 != t2 {
			return ErrNewJobWrongTypeOfParameters
		}
	}
	return nil
}

func (s *scheduler) verifyParameterType(taskFunc reflect.Value, tsk task) error {
	taskFuncType := taskFunc.Type()
	isVariadic := taskFuncType.IsVariadic()
	if isVariadic {
		variadicStart := taskFuncType.NumIn() - 1
		return s.verifyVariadic(taskFunc, tsk, variadicStart)
	}
	expectedParameterLength := taskFuncType.NumIn()
	if len(tsk.parameters) != expectedParameterLength {
		return ErrNewJobWrongNumberOfParameters
	}
	return s.verifyNonVariadic(taskFunc, tsk, expectedParameterLength)
}

var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()

func (s *scheduler) addOrUpdateJob(id uuid.UUID, definition JobDefinition, taskWrapper Task, options []JobOption) (Job, error) {
	j := internalJob{}
	if id == uuid.Nil {
		j.id = uuid.New()
	} else {
		currentJob := requestJobCtx(s.shutdownCtx, id, s.jobOutRequestCh)
		if currentJob != nil && currentJob.id != uuid.Nil {
			select {
			case <-s.shutdownCtx.Done():
				return nil, nil
			case s.removeJobCh <- id:
				<-currentJob.ctx.Done()
			}
		}

		j.id = id
	}

	if taskWrapper == nil {
		return nil, ErrNewJobTaskNil
	}

	tsk := taskWrapper()
	taskFunc := reflect.ValueOf(tsk.function)
	for taskFunc.Kind() == reflect.Pointer {
		taskFunc = taskFunc.Elem()
	}

	if taskFunc.Kind() != reflect.Func {
		return nil, ErrNewJobTaskNotFunc
	}

	j.name = runtime.FuncForPC(taskFunc.Pointer()).Name()
	j.function = tsk.function
	// Defensive copy: stopScheduler rewrites j.parameters[0] to swap
	// in a refreshed context whenever j.parameters[0] happens to be
	// the old j.ctx. Today, all code paths that produce that state
	// (addOrUpdateJob below at the append() branches) already yield a
	// fresh slice, so the mutation cannot touch the user's original.
	// This copy keeps that invariant explicit and cheap, so future
	// changes to those branches can't quietly introduce user-visible
	// aliasing.
	if len(tsk.parameters) > 0 {
		j.parameters = append([]any(nil), tsk.parameters...)
	} else {
		j.parameters = tsk.parameters
	}

	// apply global job options
	for _, option := range s.globalJobOptions {
		if err := option(&j, s.now()); err != nil {
			return nil, err
		}
	}

	// apply job specific options, which take precedence
	for _, option := range options {
		if err := option(&j, s.now()); err != nil {
			return nil, err
		}
	}

	if j.parentCtx == nil {
		j.parentCtx = s.shutdownCtx
	}
	j.ctx, j.cancel = context.WithCancel(j.parentCtx)

	if !taskFunc.IsZero() && taskFunc.Type().NumIn() > 0 {
		// if the first parameter is a context.Context and params have no context.Context, add current ctx to the params
		if taskFunc.Type().In(0) == contextType {
			if len(tsk.parameters) == 0 {
				tsk.parameters = []any{j.ctx}
				j.parameters = []any{j.ctx}
			} else if _, ok := tsk.parameters[0].(context.Context); !ok {
				tsk.parameters = append([]any{j.ctx}, tsk.parameters...)
				j.parameters = append([]any{j.ctx}, j.parameters...)
			}
		}
	}

	if err := s.verifyParameterType(taskFunc, tsk); err != nil {
		j.cancel()
		return nil, err
	}

	if err := definition.setup(&j, s.location, s.exec.clock.Now()); err != nil {
		j.cancel()
		return nil, err
	}

	newJobCtx, newJobCancel := context.WithCancel(context.Background())
	select {
	case <-s.shutdownCtx.Done():
		newJobCancel()
	case s.newJobCh <- newJobIn{
		ctx:    newJobCtx,
		cancel: newJobCancel,
		job:    j,
	}:
	}

	select {
	case <-newJobCtx.Done():
	case <-s.shutdownCtx.Done():
		newJobCancel()
	}

	out := s.jobFromInternalJob(j)
	if s.schedulerMonitor != nil {
		s.notifyJobRegistered(out)
	}
	return &out, nil
}

func (s *scheduler) RemoveByTags(tags ...string) {
	select {
	case <-s.shutdownCtx.Done():
	case s.removeJobsByTagsCh <- tags:
	}
}

func (s *scheduler) RemoveJob(id uuid.UUID) error {
	j := requestJobCtx(s.shutdownCtx, id, s.jobOutRequestCh)
	if j == nil || j.id == uuid.Nil {
		return ErrJobNotFound
	}
	select {
	case <-s.shutdownCtx.Done():
	case s.removeJobCh <- id:
	}

	return nil
}

func (s *scheduler) Start() {
	if s.started.Load() {
		s.logger.Warn("gocron: scheduler already started")
		return
	}

	select {
	case <-s.shutdownCtx.Done():
		// Scheduler already shut down, don't notify
		return
	case s.startCh <- struct{}{}:
		<-s.startedCh // Wait for scheduler to actually start

		// Scheduler has started
		s.notifySchedulerStarted()
	}
}

func (s *scheduler) StopJobs() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.exec.stopTimeout+2*time.Second)
	defer cancel()

	err := s.StopJobsWithContext(ctx)
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrStopSchedulerTimedOut
	}
	return err
}

func (s *scheduler) StopJobsWithContext(ctx context.Context) error {
	select {
	case <-s.shutdownCtx.Done():
		return nil
	case s.stopCh <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-s.stopErrCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *scheduler) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.exec.stopTimeout+2*time.Second)
	defer cancel()

	err := s.ShutdownWithContext(ctx)
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrStopSchedulerTimedOut
	}
	return err
}

func (s *scheduler) ShutdownWithContext(ctx context.Context) error {
	s.logger.Debug("scheduler shutting down")

	s.shutdownCancel()
	if !s.started.Load() {
		return nil
	}

	select {
	case err := <-s.stopErrCh:
		// notify monitor that scheduler stopped
		s.notifySchedulerShutdown()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *scheduler) Update(id uuid.UUID, jobDefinition JobDefinition, task Task, options ...JobOption) (Job, error) {
	return s.addOrUpdateJob(id, jobDefinition, task, options)
}

func (s *scheduler) JobsWaitingInQueue() int {
	if s.exec.limitMode != nil && s.exec.limitMode.mode == LimitModeWait {
		return len(s.exec.limitMode.in)
	}
	return 0
}

// -----------------------------------------------
// -----------------------------------------------
// ------------- Scheduler Options ---------------
// -----------------------------------------------
// -----------------------------------------------

// SchedulerOption defines the function for setting
// options on the Scheduler.
type SchedulerOption func(*scheduler) error

// WithClock sets the clock used by the Scheduler
// to the clock provided. See https://github.com/jonboulle/clockwork
func WithClock(clock clockwork.Clock) SchedulerOption {
	return func(s *scheduler) error {
		if clock == nil {
			return ErrWithClockNil
		}
		s.exec.clock = clock
		return nil
	}
}

// WithDistributedElector sets the elector to be used by multiple
// Scheduler instances to determine who should be the leader.
// Only the leader runs jobs, while non-leaders wait and continue
// to check if a new leader has been elected.
func WithDistributedElector(elector Elector) SchedulerOption {
	return func(s *scheduler) error {
		if elector == nil {
			return ErrWithDistributedElectorNil
		}
		s.exec.elector = elector
		return nil
	}
}

// WithDistributedLocker sets the locker to be used by multiple
// Scheduler instances to ensure that only one instance of each
// job is run.
// To disable this global locker for specific jobs, see
// WithDisabledDistributedJobLocker.
func WithDistributedLocker(locker Locker) SchedulerOption {
	return func(s *scheduler) error {
		if locker == nil {
			return ErrWithDistributedLockerNil
		}
		s.exec.locker = locker
		return nil
	}
}

// WithGlobalJobOptions sets JobOption's that will be applied to
// all jobs added to the scheduler. JobOption's set on the job
// itself will override if the same JobOption is set globally.
//
// WithGlobalJobOptions may be called multiple times; options from all
// calls are appended in order and applied to each job in that order.
func WithGlobalJobOptions(jobOptions ...JobOption) SchedulerOption {
	return func(s *scheduler) error {
		s.globalJobOptions = append(s.globalJobOptions, jobOptions...)
		return nil
	}
}

// LimitMode defines the modes used for handling jobs that reach
// the limit provided in WithLimitConcurrentJobs
type LimitMode int

const (
	// LimitModeReschedule causes jobs reaching the limit set in
	// WithLimitConcurrentJobs or WithSingletonMode to be skipped
	// and rescheduled for the next run time rather than being
	// queued up to wait.
	LimitModeReschedule LimitMode = iota + 1

	// LimitModeWait causes jobs reaching the limit set in
	// WithLimitConcurrentJobs or WithSingletonMode to wait
	// in a queue until a slot becomes available to run.
	//
	// Note: this mode can produce unpredictable results as
	// job execution order isn't guaranteed. For example, a job that
	// executes frequently may pile up in the wait queue and be executed
	// many times back to back when the queue opens.
	//
	// Warning: do not use this mode if your jobs will continue to stack
	// up beyond the ability of the limit workers to keep up. An example of
	// what NOT to do:
	//
	//     s, _ := gocron.NewScheduler(gocron.WithLimitConcurrentJobs)
	//     s.NewJob(
	//         gocron.DurationJob(
	//				time.Second,
	//				Task{
	//					Function: func() {
	//						time.Sleep(10 * time.Second)
	//					},
	//				},
	//			),
	//      )
	LimitModeWait
)

// WithLimitConcurrentJobs sets the limit and mode to be used by the
// Scheduler for limiting the number of jobs that may be running at
// a given time.
//
// Note: the limit mode selected for WithLimitConcurrentJobs takes initial
// precedence in the event you are also running a limit mode at the job level
// using WithSingletonMode.
//
// Warning: a single time consuming job can dominate your limit in the event
// you are running both the scheduler limit WithLimitConcurrentJobs(1, LimitModeWait)
// and a job limit WithSingletonMode(LimitModeReschedule).
func WithLimitConcurrentJobs(limit uint, mode LimitMode) SchedulerOption {
	return func(s *scheduler) error {
		if limit == 0 {
			return ErrWithLimitConcurrentJobsZero
		}
		s.exec.limitMode = &limitModeConfig{
			mode:          mode,
			limit:         limit,
			in:            make(chan jobIn, defaultLimitModeQueueBuffer),
			singletonJobs: make(map[uuid.UUID]struct{}),
		}
		if mode == LimitModeReschedule {
			s.exec.limitMode.rescheduleLimiter = make(chan struct{}, limit)
		}
		return nil
	}
}

// WithLocation sets the location (i.e. timezone) that the scheduler
// should operate within. In many systems time.Local is UTC.
// Default: time.Local
func WithLocation(location *time.Location) SchedulerOption {
	return func(s *scheduler) error {
		if location == nil {
			return ErrWithLocationNil
		}
		s.location = location
		return nil
	}
}

// WithLogger sets the logger to be used by the Scheduler.
func WithLogger(logger Logger) SchedulerOption {
	return func(s *scheduler) error {
		if logger == nil {
			return ErrWithLoggerNil
		}
		s.logger = logger
		s.exec.logger = logger
		return nil
	}
}

// WithStopTimeout sets the amount of time the Scheduler should
// wait gracefully for jobs to complete before returning when
// StopJobs() or Shutdown() are called.
// Default: 10 * time.Second
func WithStopTimeout(timeout time.Duration) SchedulerOption {
	return func(s *scheduler) error {
		if timeout <= 0 {
			return ErrWithStopTimeoutZeroOrNegative
		}
		s.exec.stopTimeout = timeout
		return nil
	}
}

// WithMonitor sets the metrics provider to be used by the Scheduler.
func WithMonitor(monitor Monitor) SchedulerOption {
	return func(s *scheduler) error {
		if monitor == nil {
			return ErrWithMonitorNil
		}
		s.exec.monitor = monitor
		return nil
	}
}

// WithMonitorStatus sets the metrics provider to be used by the Scheduler.
func WithMonitorStatus(monitor MonitorStatus) SchedulerOption {
	return func(s *scheduler) error {
		if monitor == nil {
			return ErrWithMonitorNil
		}
		s.exec.monitorStatus = monitor
		return nil
	}
}

// WithSchedulerMonitor sets a monitor that will be called with scheduler-level events.
func WithSchedulerMonitor(monitor SchedulerMonitor) SchedulerOption {
	return func(s *scheduler) error {
		if monitor == nil {
			return ErrSchedulerMonitorNil
		}
		s.schedulerMonitor = monitor
		return nil
	}
}

// notifySchedulerStarted notifies the monitor that scheduler has started
func (s *scheduler) notifySchedulerStarted() {
	if s.schedulerMonitor != nil {
		s.schedulerMonitor.SchedulerStarted()
	}
}

// notifySchedulerShutdown notifies the monitor that scheduler has stopped
func (s *scheduler) notifySchedulerShutdown() {
	if s.schedulerMonitor != nil {
		s.schedulerMonitor.SchedulerShutdown()
	}
}

// notifyJobRegistered notifies the monitor that a job has been registered
func (s *scheduler) notifyJobRegistered(job Job) {
	if s.schedulerMonitor != nil {
		s.schedulerMonitor.JobRegistered(job)
	}
}

// notifyJobUnregistered notifies the monitor that a job has been unregistered
func (s *scheduler) notifyJobUnregistered(job Job) {
	if s.schedulerMonitor != nil {
		s.schedulerMonitor.JobUnregistered(job)
	}
}

// notifyJobStarted notifies the monitor that a job has started
func (s *scheduler) notifyJobStarted(job Job) {
	if s.schedulerMonitor != nil {
		s.schedulerMonitor.JobStarted(job)
	}
}

// notifyJobRunning notifies the monitor that a job is running.
func (s *scheduler) notifyJobRunning(job Job) {
	if s.schedulerMonitor != nil {
		s.schedulerMonitor.JobRunning(job)
	}
}

// notifyJobCompleted notifies the monitor that a job has completed.
func (s *scheduler) notifyJobCompleted(job Job) {
	if s.schedulerMonitor != nil {
		s.schedulerMonitor.JobCompleted(job)
	}
}

// notifyJobFailed notifies the monitor that a job has failed.
func (s *scheduler) notifyJobFailed(job Job, err error) {
	if s.schedulerMonitor != nil {
		s.schedulerMonitor.JobFailed(job, err)
	}
}

// notifySchedulerStopped notifies the monitor that the scheduler has stopped
func (s *scheduler) notifySchedulerStopped() {
	if s.schedulerMonitor != nil {
		s.schedulerMonitor.SchedulerStopped()
	}
}

// notifyJobExecutionTime notifies the monitor of a job's execution time
func (s *scheduler) notifyJobExecutionTime(job Job, duration time.Duration) {
	if s.schedulerMonitor != nil {
		s.schedulerMonitor.JobExecutionTime(job, duration)
	}
}

// notifyJobSchedulingDelay notifies the monitor of scheduling delay
func (s *scheduler) notifyJobSchedulingDelay(job Job, scheduledTime time.Time, actualStartTime time.Time) {
	if s.schedulerMonitor != nil {
		s.schedulerMonitor.JobSchedulingDelay(job, scheduledTime, actualStartTime)
	}
}

// notifyConcurrencyLimitReached notifies the monitor that a concurrency limit was reached
func (s *scheduler) notifyConcurrencyLimitReached(limitType string, job Job) {
	if s.schedulerMonitor != nil {
		s.schedulerMonitor.ConcurrencyLimitReached(limitType, job)
	}
}
