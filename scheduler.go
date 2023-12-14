//go:generate mockgen -destination=mocks/scheduler.go -package=gocronmocks . Scheduler
package gocron

import (
	"context"
	"reflect"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"golang.org/x/exp/slices"
)

var _ Scheduler = (*scheduler)(nil)

// Scheduler defines the interface for the Scheduler.
type Scheduler interface {
	Jobs() []Job
	NewJob(JobDefinition, Task, ...JobOption) (Job, error)
	RemoveByTags(...string)
	RemoveJob(uuid.UUID) error
	Start()
	StopJobs() error
	Shutdown() error
	Update(uuid.UUID, JobDefinition, Task, ...JobOption) (Job, error)
}

// -----------------------------------------------
// -----------------------------------------------
// ----------------- Scheduler -------------------
// -----------------------------------------------
// -----------------------------------------------

type scheduler struct {
	shutdownCtx      context.Context
	shutdownCancel   context.CancelFunc
	exec             executor
	jobs             map[uuid.UUID]internalJob
	location         *time.Location
	clock            clockwork.Clock
	started          bool
	globalJobOptions []JobOption
	logger           Logger

	startCh            chan struct{}
	startedCh          chan struct{}
	stopCh             chan struct{}
	stopErrCh          chan error
	allJobsOutRequest  chan allJobsOutRequest
	jobOutRequestCh    chan jobOutRequest
	newJobCh           chan internalJob
	removeJobCh        chan uuid.UUID
	removeJobsByTagsCh chan []string
}

type jobOutRequest struct {
	id      uuid.UUID
	outChan chan internalJob
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
		singletonRunners: make(map[uuid.UUID]singletonRunner),
		logger:           &noOpLogger{},

		jobsIDsIn:     make(chan uuid.UUID),
		jobIDsOut:     make(chan uuid.UUID),
		jobOutRequest: make(chan jobOutRequest, 1000),
		done:          make(chan error),
	}

	s := &scheduler{
		shutdownCtx:    schCtx,
		shutdownCancel: cancel,
		exec:           exec,
		jobs:           make(map[uuid.UUID]internalJob),
		location:       time.Local,
		clock:          clockwork.NewRealClock(),
		logger:         &noOpLogger{},

		newJobCh:           make(chan internalJob),
		removeJobCh:        make(chan uuid.UUID),
		removeJobsByTagsCh: make(chan []string),
		startCh:            make(chan struct{}),
		startedCh:          make(chan struct{}),
		stopCh:             make(chan struct{}),
		stopErrCh:          make(chan error, 1),
		jobOutRequestCh:    make(chan jobOutRequest),
		allJobsOutRequest:  make(chan allJobsOutRequest),
	}

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
			case id := <-s.exec.jobIDsOut:
				s.selectExecJobIDsOut(id)

			case j := <-s.newJobCh:
				s.selectNewJob(j)

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
	if s.started {
		s.exec.stopCh <- struct{}{}
	}

	for _, j := range s.jobs {
		j.stop()
	}
	for id, j := range s.jobs {
		<-j.ctx.Done()

		j.ctx, j.cancel = context.WithCancel(s.shutdownCtx)
		s.jobs[id] = j
	}
	var err error
	if s.started {
		select {
		case err = <-s.exec.done:
		case <-time.After(s.exec.stopTimeout + 1*time.Second):
			err = ErrStopExecutorTimedOut
		}
	}
	s.stopErrCh <- err
	s.started = false
	s.logger.Debug("gocron: scheduler stopped")
}

func (s *scheduler) selectAllJobsOutRequest(out allJobsOutRequest) {
	outJobs := make([]Job, len(s.jobs))
	var counter int
	for _, j := range s.jobs {
		outJobs[counter] = s.jobFromInternalJob(j)
		counter++
	}
	select {
	case <-s.shutdownCtx.Done():
	case out.outChan <- outJobs:
	}
}

func (s *scheduler) selectRemoveJob(id uuid.UUID) {
	j, ok := s.jobs[id]
	if !ok {
		return
	}
	j.stop()
	delete(s.jobs, id)
}

func (s *scheduler) selectExecJobIDsOut(id uuid.UUID) {
	j := s.jobs[id]
	j.lastRun = j.nextRun

	if j.limitRunsTo != nil {
		j.limitRunsTo.runCount = j.limitRunsTo.runCount + 1
		if j.limitRunsTo.runCount == j.limitRunsTo.limit {
			go func() {
				select {
				case <-s.shutdownCtx.Done():
					return
				case s.removeJobCh <- id:
				}
			}()
			return
		}
	}

	next := j.next(j.lastRun)
	j.nextRun = next
	j.timer = s.clock.AfterFunc(next.Sub(s.now()), func() {
		select {
		case <-s.shutdownCtx.Done():
			return
		case s.exec.jobsIDsIn <- id:
		}
	})
	s.jobs[id] = j
}

func (s *scheduler) selectJobOutRequest(out jobOutRequest) {
	if j, ok := s.jobs[out.id]; ok {
		select {
		case out.outChan <- j:
		case <-s.shutdownCtx.Done():
		}
	}
	close(out.outChan)
}

func (s *scheduler) selectNewJob(j internalJob) {
	if s.started {
		next := j.startTime
		if j.startImmediately {
			next = s.now()
			select {
			case <-s.shutdownCtx.Done():
			case s.exec.jobsIDsIn <- j.id:
			}
		} else {
			if next.IsZero() {
				next = j.next(s.now())
			}

			id := j.id
			j.timer = s.clock.AfterFunc(next.Sub(s.now()), func() {
				select {
				case <-s.shutdownCtx.Done():
				case s.exec.jobsIDsIn <- id:
				}
			})
		}
		j.nextRun = next
	}

	s.jobs[j.id] = j
}

func (s *scheduler) selectRemoveJobsByTags(tags []string) {
	for _, j := range s.jobs {
		for _, tag := range tags {
			if slices.Contains(j.tags, tag) {
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

	s.started = true
	for id, j := range s.jobs {
		next := j.startTime
		if j.startImmediately {
			next = s.now()
			select {
			case <-s.shutdownCtx.Done():
			case s.exec.jobsIDsIn <- id:
			}
		} else {
			if next.IsZero() {
				next = j.next(s.now())
			}

			jobID := id
			j.timer = s.clock.AfterFunc(next.Sub(s.now()), func() {
				select {
				case <-s.shutdownCtx.Done():
				case s.exec.jobsIDsIn <- jobID:
				}
			})
		}
		j.nextRun = next
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
	return s.clock.Now().In(s.location)
}

func (s *scheduler) jobFromInternalJob(in internalJob) job {
	return job{
		id:            in.id,
		name:          in.name,
		tags:          slices.Clone(in.tags),
		jobOutRequest: s.jobOutRequestCh,
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

	j.ctx, j.cancel = context.WithCancel(s.shutdownCtx)

	if taskWrapper == nil {
		return nil, ErrNewJobTaskNil
	}

	tsk := taskWrapper()
	taskFunc := reflect.ValueOf(tsk.function)
	for taskFunc.Kind() == reflect.Ptr {
		taskFunc = taskFunc.Elem()
	}

	if taskFunc.Kind() != reflect.Func {
		return nil, ErrNewJobTaskNotFunc
	}

	expectedParameterLength := taskFunc.Type().NumIn()
	if len(tsk.parameters) != expectedParameterLength {
		return nil, ErrNewJobWrongNumberOfParameters
	}

	for i := 0; i < expectedParameterLength; i++ {
		t1 := reflect.TypeOf(tsk.parameters[i]).Kind()
		if t1 == reflect.Interface || t1 == reflect.Pointer {
			t1 = reflect.TypeOf(tsk.parameters[i]).Elem().Kind()
		}
		t2 := reflect.New(taskFunc.Type().In(i)).Elem().Kind()
		if t2 == reflect.Interface || t2 == reflect.Pointer {
			t2 = reflect.Indirect(reflect.ValueOf(taskFunc.Type().In(i))).Kind()
		}
		if t1 != t2 {
			return nil, ErrNewJobWrongTypeOfParameters
		}
	}

	j.name = runtime.FuncForPC(taskFunc.Pointer()).Name()
	j.function = tsk.function
	j.parameters = tsk.parameters

	// apply global job options
	for _, option := range s.globalJobOptions {
		if err := option(&j); err != nil {
			return nil, err
		}
	}

	// apply job specific options, which take precedence
	for _, option := range options {
		if err := option(&j); err != nil {
			return nil, err
		}
	}

	if err := definition.setup(&j, s.location); err != nil {
		return nil, err
	}

	select {
	case <-s.shutdownCtx.Done():
	case s.newJobCh <- j:
	}

	return &job{
		id:            j.id,
		name:          j.name,
		tags:          slices.Clone(j.tags),
		jobOutRequest: s.jobOutRequestCh,
	}, nil
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

// Start begins scheduling jobs for execution based
// on each job's definition. Job's added to an already
// running scheduler will be scheduled immediately based
// on definition.
func (s *scheduler) Start() {
	select {
	case <-s.shutdownCtx.Done():
	case s.startCh <- struct{}{}:
		<-s.startedCh
	}
}

// StopJobs stops the execution of all jobs in the scheduler.
// This can be useful in situations where jobs need to be
// paused globally and then restarted with Start().
func (s *scheduler) StopJobs() error {
	select {
	case <-s.shutdownCtx.Done():
		return nil
	case s.stopCh <- struct{}{}:
	}
	select {
	case err := <-s.stopErrCh:
		return err
	case <-time.After(s.exec.stopTimeout + 2*time.Second):
		return ErrStopSchedulerTimedOut
	}
}

// Shutdown should be called when you no longer need
// the Scheduler or Job's as the Scheduler cannot
// be restarted after calling Shutdown.
func (s *scheduler) Shutdown() error {
	s.shutdownCancel()
	select {
	case err := <-s.stopErrCh:
		return err
	case <-time.After(s.exec.stopTimeout + 2*time.Second):
		return ErrStopSchedulerTimedOut
	}
}

// Update replaces the existing Job's JobDefinition with the provided
// JobDefinition. The Job's Job.ID() remains the same.
func (s *scheduler) Update(id uuid.UUID, jobDefinition JobDefinition, task Task, options ...JobOption) (Job, error) {
	return s.addOrUpdateJob(id, jobDefinition, task, options)
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
		s.clock = clock
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
func WithGlobalJobOptions(jobOptions ...JobOption) SchedulerOption {
	return func(s *scheduler) error {
		s.globalJobOptions = jobOptions
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
	LimitModeReschedule = 1

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
	LimitModeWait = 2
)

// WithLimitConcurrentJobs sets the limit and mode to be used by the
// Scheduler for limiting the number of jobs that may be running at
// a given time.
func WithLimitConcurrentJobs(limit uint, mode LimitMode) SchedulerOption {
	return func(s *scheduler) error {
		if limit == 0 {
			return ErrWithLimitConcurrentJobsZero
		}
		s.exec.limitMode = &limitModeConfig{
			mode:          mode,
			limit:         limit,
			in:            make(chan uuid.UUID, 1000),
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
