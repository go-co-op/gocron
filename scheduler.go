package gocron

import (
	"context"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"golang.org/x/exp/maps"
)

var _ Scheduler = (*scheduler)(nil)

type Scheduler interface {
	Jobs() []Job
	NewJob(JobDefinition) (Job, error)
	RemoveByTags(...string)
	RemoveJob(uuid.UUID) error
	Start()
	Stop() error
	Update(uuid.UUID, JobDefinition) (Job, error)
}

// -----------------------------------------------
// -----------------------------------------------
// ----------------- Scheduler -------------------
// -----------------------------------------------
// -----------------------------------------------

type scheduler struct {
	ctx               context.Context
	cancel            context.CancelFunc
	exec              executor
	jobs              map[uuid.UUID]internalJob
	allJobsOutRequest chan allJobsOutRequest
	jobOutRequest     chan jobOutRequest
	newJobs           chan internalJob
	removeJobs        chan uuid.UUID
	removeJobsByTags  chan []string
	location          *time.Location
	clock             clockwork.Clock
	started           bool
	start             chan struct{}
	globalJobOptions  []JobOption
}

type jobOutRequest struct {
	id      uuid.UUID
	outChan chan internalJob
}

type allJobsOutRequest struct {
	outChan chan map[uuid.UUID]internalJob
}

func NewScheduler(options ...SchedulerOption) (Scheduler, error) {
	ctx, cancel := context.WithCancel(context.Background())
	execCtx, execCancel := context.WithCancel(context.Background())

	exec := executor{
		ctx:              execCtx,
		cancel:           execCancel,
		schCtx:           ctx,
		jobsIDsIn:        make(chan uuid.UUID),
		jobIDsOut:        make(chan uuid.UUID),
		jobOutRequest:    make(chan jobOutRequest),
		shutdownTimeout:  time.Second * 10,
		done:             make(chan error),
		singletonRunners: make(map[uuid.UUID]singletonRunner),
	}

	s := &scheduler{
		ctx:               ctx,
		cancel:            cancel,
		exec:              exec,
		jobs:              make(map[uuid.UUID]internalJob, 0),
		newJobs:           make(chan internalJob),
		removeJobs:        make(chan uuid.UUID),
		removeJobsByTags:  make(chan []string),
		start:             make(chan struct{}),
		jobOutRequest:     make(chan jobOutRequest),
		allJobsOutRequest: make(chan allJobsOutRequest),
		location:          time.Local,
		clock:             clockwork.NewRealClock(),
	}

	for _, option := range options {
		err := option(s)
		if err != nil {
			return nil, err
		}
	}

	go func() {
		for {
			select {
			case id := <-s.exec.jobIDsOut:
				j := s.jobs[id]

				j.lastRun = j.nextRun
				next := j.next(j.lastRun)
				j.nextRun = next
				j.timer = s.clock.AfterFunc(next.Sub(s.now()), func() {
					s.exec.jobsIDsIn <- id
				})
				s.jobs[id] = j

			case j := <-s.newJobs:
				if _, ok := s.jobs[j.id]; !ok {
					next := j.next(s.now())
					j.nextRun = next
					if s.started {
						id := j.id
						j.timer = s.clock.AfterFunc(next.Sub(s.now()), func() {
							s.exec.jobsIDsIn <- id
						})
					}
					s.jobs[j.id] = j
				} else {
					// the job exists already.
					// what all has to be handled here? this should mostly be new
					// jobs, but if update is used, the job would exist
					// and we'd have to handle that here
				}

			case id := <-s.removeJobs:
				j, ok := s.jobs[id]
				if !ok {
					break
				}
				j.stop()
				delete(s.jobs, id)

			case tags := <-s.removeJobsByTags:
				for _, j := range s.jobs {
					if mapKeysContainAnySliceElement(j.tags, tags) {
						j.stop()
						delete(s.jobs, j.id)
					}
				}

			case out := <-s.exec.jobOutRequest:
				if j, ok := s.jobs[out.id]; ok {
					out.outChan <- j
					close(out.outChan)
				} else {
					close(out.outChan)
				}

			case out := <-s.jobOutRequest:
				if j, ok := s.jobs[out.id]; ok {
					out.outChan <- j
					close(out.outChan)
				} else {
					close(out.outChan)
				}

			case out := <-s.allJobsOutRequest:
				s.selectAllJobsOutRequest(out)

			case <-s.start:
				s.selectStart()

			case <-s.ctx.Done():
				s.selectDone()
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

func (s *scheduler) selectDone() {
	for _, j := range s.jobs {
		j.stop()
	}
}

func (s *scheduler) selectAllJobsOutRequest(out allJobsOutRequest) {
	outJobs := make(map[uuid.UUID]internalJob, len(s.jobs))
	for id, j := range s.jobs {
		outJobs[id] = j.copy()
	}
	out.outChan <- outJobs
}

func (s *scheduler) selectStart() {
	s.started = true
	for id, j := range s.jobs {
		next := j.next(s.now())
		j.nextRun = next

		jobId := id
		j.timer = s.clock.AfterFunc(next.Sub(s.now()), func() {
			s.exec.jobsIDsIn <- jobId
		})
		s.jobs[id] = j
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
		tags:          maps.Keys(in.tags),
		jobOutRequest: s.jobOutRequest,
	}
}

func (s *scheduler) Jobs() []Job {
	outChan := make(chan map[uuid.UUID]internalJob)
	s.allJobsOutRequest <- allJobsOutRequest{outChan: outChan}
	jobs := <-outChan

	outJobs := make([]Job, len(jobs))

	var counter int
	for _, j := range jobs {
		outJobs[counter] = s.jobFromInternalJob(j)
		counter++
	}

	return outJobs
}

func (s *scheduler) NewJob(jobDefinition JobDefinition) (Job, error) {
	return s.addOrUpdateJob(uuid.Nil, jobDefinition)
}

func (s *scheduler) addOrUpdateJob(id uuid.UUID, definition JobDefinition) (Job, error) {
	j := internalJob{}
	if id == uuid.Nil {
		j.id = uuid.New()
	} else {
		s.removeJobs <- id
		j.id = id
	}

	j.ctx, j.cancel = context.WithCancel(context.Background())

	tsk := definition.task()()
	taskFunc := reflect.ValueOf(tsk.function)
	for taskFunc.Kind() == reflect.Ptr {
		taskFunc = taskFunc.Elem()
	}

	if taskFunc.Kind() != reflect.Func {
		return nil, ErrNewJobTask
	}

	j.function = tsk.function
	j.parameters = tsk.parameters

	// apply global job options
	for _, option := range s.globalJobOptions {
		if err := option(&j); err != nil {
			return nil, err
		}
	}

	// apply job specific options, which take precedence
	for _, option := range definition.options() {
		if err := option(&j); err != nil {
			return nil, err
		}
	}

	if err := definition.setup(&j, s.location); err != nil {
		return nil, err
	}

	s.newJobs <- j
	return &job{
		id:            j.id,
		name:          j.name,
		tags:          maps.Keys(j.tags),
		jobOutRequest: s.jobOutRequest,
	}, nil
}

func (s *scheduler) RemoveByTags(tags ...string) {
	s.removeJobsByTags <- tags
}

func (s *scheduler) RemoveJob(id uuid.UUID) error {
	j := requestJob(id, s.jobOutRequest)
	if j.id == uuid.Nil {
		return ErrJobNotFound
	}
	s.removeJobs <- id
	return nil
}

func (s *scheduler) Start() {
	go s.exec.start()
	s.start <- struct{}{}
}

func (s *scheduler) Stop() error {
	s.cancel()
	select {
	case err := <-s.exec.done:
		return err
	case <-time.After(s.exec.shutdownTimeout + time.Second):
		return ErrStopTimedOut
	}
}

func (s *scheduler) Update(id uuid.UUID, jobDefinition JobDefinition) (Job, error) {
	return s.addOrUpdateJob(id, jobDefinition)
}

// -----------------------------------------------
// -----------------------------------------------
// ------------- Scheduler Options ---------------
// -----------------------------------------------
// -----------------------------------------------

type SchedulerOption func(*scheduler) error

// WithDistributedElector sets the elector to be used by multiple
// Scheduler instances to determine who should be the leader.
// Only the leader runs jobs, while non-leaders wait and continue
// to check if a new leader has been elected.
func WithDistributedElector(elector Elector) SchedulerOption {
	return func(s *scheduler) error {
		if elector == nil {
			return ErrWithDistributedElector
		}
		s.exec.elector = elector
		return nil
	}
}

// WithFakeClock sets the clock used by the Scheduler
// to the clock provided. See https://github.com/jonboulle/clockwork
func WithFakeClock(clock clockwork.Clock) SchedulerOption {
	return func(s *scheduler) error {
		if clock == nil {
			return ErrWithFakeClockNil
		}
		s.clock = clock
		return nil
	}
}

// WithGlobalJobOptions sets JobOption's that will be applied to
// all jobs added to the scheduler
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
	// WithLimitConcurrentJobs to be skipped / rescheduled for the
	// next run time rather than being queued up to wait.
	LimitModeReschedule = 1

	// LimitModeWait causes jobs reaching the limit set in
	// WithLimitConcurrentJobs to wait in a queue until a slot becomes
	// available to run.
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
	//     s, _ := gocron.NewScheduler()
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
func WithLimitConcurrentJobs(limit int, mode LimitMode) SchedulerOption {
	return func(s *scheduler) error {
		if limit <= 0 {
			return ErrWithLimitConcurrentJobsZero
		}
		s.exec.limitMode = &limitMode{
			mode:  mode,
			limit: limit,
			in:    make(chan uuid.UUID, 1000),
			done:  make(chan struct{}),
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

// WithShutdownTimeout sets the amount of time the Scheduler should
// wait gracefully for jobs to complete before shutting down.
// Default: 10 * time.Second
func WithShutdownTimeout(timeout time.Duration) SchedulerOption {
	return func(s *scheduler) error {
		if timeout <= 0 {
			return ErrWithShutdownTimeoutZero
		}
		s.exec.shutdownTimeout = timeout
		return nil
	}
}
