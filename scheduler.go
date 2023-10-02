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
	newJobs          chan job
	removeJobs       chan uuid.UUID
	jobOutRequest    chan jobOutRequest
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
		ctx:           ctx,
		cancel:        cancel,
		exec:          exec,
		jobs:          make(map[uuid.UUID]job, 0),
		newJobs:       make(chan job),
		removeJobs:    make(chan uuid.UUID),
		start:         make(chan struct{}),
		jobOutRequest: jobOutRequestChan,
		location:      time.Local,
		clock:         clockwork.NewRealClock(),
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

func (s *scheduler) NewJob(definition JobDefinition) (Job, error) {
	j := job{
		id: uuid.New(),
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

func (s *scheduler) Start() {
	go s.exec.start()
	s.start <- struct{}{}
}

func (s *scheduler) Stop() error {
	s.cancel()
	return <-s.exec.done
}

func (s *scheduler) Update(id uuid.UUID, jobDefinition JobDefinition) (Job, error) {
	//TODO implement me
	panic("implement me")
}

func (s *scheduler) RemoveJob(uuid2 uuid.UUID) error {
	//TODO implement me
	panic("implement me")
}

// -----------------------------------------------
// -----------------------------------------------
// ------------- Scheduler Options ---------------
// -----------------------------------------------
// -----------------------------------------------

type SchedulerOption func(*scheduler) error

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

type LimitMode int

const (
	LimitModeReschedule = 1
	LimitModeWait       = 2
)

func WithLimit(limit int, mode LimitMode) SchedulerOption {
	return func(s *scheduler) error {
		if limit <= 0 {
			return ErrWithLimitZero
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

func WithLocation(location *time.Location) SchedulerOption {
	return func(s *scheduler) error {
		if location == nil {
			return ErrWithLocationNil
		}
		s.location = location
		return nil
	}
}

func WithShutdownTimeout(timeout time.Duration) SchedulerOption {
	return func(s *scheduler) error {
		if timeout <= 0 {
			return ErrWithShutdownTimeoutZero
		}
		s.exec.shutdownTimeout = timeout
		return nil
	}
}
