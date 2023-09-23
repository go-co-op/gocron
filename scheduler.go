package gocron

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
)

var _ Scheduler = (*scheduler)(nil)

type Scheduler interface {
	GetJobLastRun(id uuid.UUID) (time.Time, error)
	NewJob(JobDefinition) (uuid.UUID, error)
	Start()
	Stop()
}

// -----------------------------------------------
// -----------------------------------------------
// ------------- Scheduler Options ---------------
// -----------------------------------------------
// -----------------------------------------------

type SchedulerOption func(*scheduler) error

func WithLocation(location *time.Location) SchedulerOption {
	return func(s *scheduler) error {
		if location == nil {
			return fmt.Errorf("gocron: WithLocation: location was nil")
		}
		s.location = location
		return nil
	}
}

func WithShutdownTimeout(timeout time.Duration) SchedulerOption {
	return func(s *scheduler) error {
		if timeout == 0 {
			return fmt.Errorf("gocron: shutdown timeout cannot be zero")
		}
		s.exec.shutdownTimeout = timeout
		return nil
	}
}

// -----------------------------------------------
// -----------------------------------------------
// ----------------- Scheduler -------------------
// -----------------------------------------------
// -----------------------------------------------

type scheduler struct {
	ctx           context.Context
	cancel        context.CancelFunc
	exec          executor
	jobs          map[uuid.UUID]job
	newJobs       chan job
	jobOutRequest chan jobOutRequest
	location      *time.Location
	//todo - clock should be mockable, allow user to pass a fake clock
	clock   clockwork.Clock
	started bool
	start   chan struct{}
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
		ctx:             execCtx,
		cancel:          execCancel,
		schCtx:          ctx,
		jobsIDsIn:       make(chan uuid.UUID),
		jobIDsOut:       make(chan uuid.UUID),
		jobOutRequest:   jobOutRequestChan,
		shutdownTimeout: time.Second * 10,
	}

	s := &scheduler{
		ctx:           ctx,
		cancel:        cancel,
		exec:          exec,
		jobs:          make(map[uuid.UUID]job, 0),
		newJobs:       make(chan job),
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
				lastRun := s.clock.Now()
				j := s.jobs[id]
				j.lastRun = lastRun
				s.jobs[id] = j
			case j := <-s.newJobs:
				if _, ok := s.jobs[j.id]; !ok {
					if s.started {
						next := j.next(time.Now())
						id := j.id
						j.timer = s.clock.AfterFunc(time.Until(next), func() {
							s.exec.jobsIDsIn <- id
						})
					}
					s.jobs[j.id] = j
				} else {
					// the job exists already.
					// what all has to be handled here? this should mostly be new
					//jobs, but if update is used, the job would exist
					// and we'd have to handle that here
				}

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
					next := j.next(time.Now())

					jobId := id
					j.timer = s.clock.AfterFunc(time.Until(next), func() {
						s.exec.jobsIDsIn <- jobId
					})
					s.jobs[id] = j
				}
			case <-s.ctx.Done():
				for _, j := range s.jobs {
					j.cancel()
				}
				return
			}
		}
	}()

	return s, nil
}

func (s *scheduler) NewJob(definition JobDefinition) (uuid.UUID, error) {
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
		return uuid.Nil, fmt.Errorf("gocron: Task.Function was not a reflect.Func")
	}

	j.function = task.Function
	j.parameters = task.Parameters

	for _, option := range definition.options() {
		err := option(&j)
		if err != nil {
			return uuid.Nil, err
		}
	}

	err := definition.setup(&j, s.location)
	if err != nil {
		return uuid.Nil, err
	}

	s.newJobs <- j
	return j.id, nil
}

func (s *scheduler) Start() {
	go s.exec.start()
	s.start <- struct{}{}
}

func (s *scheduler) Stop() {
	s.cancel()
	<-s.exec.ctx.Done()
}

func (s *scheduler) GetJobLastRun(id uuid.UUID) (time.Time, error) {
	j := requestJob(id, s.jobOutRequest)
	if j.id == uuid.Nil {
		return time.Time{}, fmt.Errorf("gocron: job not found")
	}
	return j.lastRun, nil
}
