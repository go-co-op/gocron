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
}

type scheduler struct {
	ctx           context.Context
	cancel        context.CancelFunc
	exec          executor
	jobs          map[uuid.UUID]job
	newJobs       chan job
	jobOutRequest chan jobOutRequest
	timezone      *time.Location
	clock         clockwork.Clock
	started       bool
	start         chan struct{}
}

type jobOutRequest struct {
	id      uuid.UUID
	outChan chan job
}

func NewScheduler() Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	jobOutRequestChan := make(chan jobOutRequest)

	exec := executor{
		ctx:           ctx,
		jobsIDsIn:     make(chan uuid.UUID),
		jobIDsOut:     make(chan uuid.UUID),
		jobOutRequest: jobOutRequestChan,
	}

	s := &scheduler{
		ctx:           ctx,
		cancel:        cancel,
		exec:          exec,
		jobs:          make(map[uuid.UUID]job, 0),
		newJobs:       make(chan job),
		start:         make(chan struct{}),
		jobOutRequest: jobOutRequestChan,
		timezone:      time.UTC,
		clock:         clockwork.NewRealClock(),
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
				return
			}
		}
	}()

	return s
}

func (s *scheduler) NewJob(definition JobDefinition) (uuid.UUID, error) {
	j := job{
		id: uuid.New(),
	}

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

	j, err := definition.setup(j, s.timezone)
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

func (s *scheduler) GetJobLastRun(id uuid.UUID) (time.Time, error) {
	j := requestJob(id, s.jobOutRequest)
	if j.id == uuid.Nil {
		return time.Time{}, fmt.Errorf("gocron: job not found")
	}
	return j.lastRun, nil
}
