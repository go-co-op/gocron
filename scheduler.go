package gocron

import (
	"context"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
)

var _ Scheduler = (*scheduler)(nil)

type Scheduler interface {
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
	ctx              context.Context
	cancel           context.CancelFunc
	exec             executor
	jobs             map[uuid.UUID]job
	jobsOutRequest   chan jobsOutRequest
	jobOutRequest    chan jobOutRequest
	newJobs          chan job
	removeJobs       chan uuid.UUID
	location         *time.Location
	clock            clockwork.Clock
	started          bool
	start            chan struct{}
	globalJobOptions []JobOption
}

type jobOutRequest struct {
	id      uuid.UUID
	outChan chan job
}

type jobsOutRequest struct {
	outChan chan map[uuid.UUID]job
}

func NewScheduler(options ...SchedulerOption) (Scheduler, error) {
	ctx, cancel := context.WithCancel(context.Background())
	execCtx, execCancel := context.WithCancel(context.Background())

	jobOutRequestChan := make(chan jobOutRequest)

	exec := executor{
		ctx:              execCtx,
		cancel:           execCancel,
		schCtx:           ctx,
		jobsIDsIn:        make(chan uuid.UUID),
		jobIDsOut:        make(chan uuid.UUID),
		jobOutRequest:    jobOutRequestChan,
		shutdownTimeout:  time.Second * 10,
		done:             make(chan error),
		singletonRunners: make(map[uuid.UUID]singletonRunner),
	}

	s := &scheduler{
		ctx:            ctx,
		cancel:         cancel,
		exec:           exec,
		jobs:           make(map[uuid.UUID]job, 0),
		newJobs:        make(chan job),
		removeJobs:     make(chan uuid.UUID),
		start:          make(chan struct{}),
		jobOutRequest:  jobOutRequestChan,
		jobsOutRequest: make(chan jobsOutRequest),
		location:       time.Local,
		clock:          clockwork.NewRealClock(),
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

			case out := <-s.jobOutRequest:
				if j, ok := s.jobs[out.id]; ok {
					out.outChan <- j
					close(out.outChan)
				} else {
					close(out.outChan)
				}

			case out := <-s.jobsOutRequest:
				out.outChan <- s.jobs

			case <-s.start:
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

			case <-s.ctx.Done():
				for _, j := range s.jobs {
					j.stop()
				}
				return
			}
		}
	}()

	return s, nil
}

func (s *scheduler) now() time.Time {
	return s.clock.Now().In(s.location)
}

func (s *scheduler) NewJob(jobDefinition JobDefinition) (Job, error) {
	return s.addOrUpdateJob(uuid.Nil, jobDefinition)
}

func (s *scheduler) addOrUpdateJob(id uuid.UUID, definition JobDefinition) (Job, error) {
	j := job{}
	if id == uuid.Nil {
		j.id = uuid.New()
	} else {
		s.removeJobs <- id
		j.id = id
	}

	j.ctx, j.cancel = context.WithCancel(context.Background())

	task := definition.task()
	taskFunc := reflect.ValueOf(task.Function)
	for taskFunc.Kind() == reflect.Ptr {
		taskFunc = taskFunc.Elem()
	}

	if taskFunc.Kind() != reflect.Func {
		return nil, ErrNewJobTask
	}

	j.function = task.Function
	j.parameters = task.Parameters

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
	return &publicJob{
		id:            j.id,
		jobOutRequest: s.jobOutRequest,
	}, nil
}

func (s *scheduler) RemoveByTags(tags ...string) {
	jr := jobsOutRequest{outChan: make(chan map[uuid.UUID]job)}
	s.jobsOutRequest <- jr
	jobs := <-jr.outChan

	for _, j := range jobs {
		if contains(j.tags, tags) {
			s.removeJobs <- j.id
		}
	}
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
	return <-s.exec.done
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
// Example: clockwork.NewFakeClock()
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
