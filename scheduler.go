package gocron

import (
	"fmt"

	"github.com/google/uuid"
)

var _ Scheduler = (*scheduler)(nil)

type Scheduler interface {
	NewJob(Job, error) error
	Start()
}

type scheduler struct {
	jobs map[uuid.UUID]job
	exec executor
}

func (s *scheduler) NewJob(j Job, err error) error {
	if err != nil {
		return err
	}

	jt, ok := j.(*job)
	if !ok {
		return fmt.Errorf("something is really wrong")
	}

	s.jobs[jt.id] = *jt
	return nil
}

// todo use clockwork
// todo all channels, no mutexes

func (s *scheduler) Start() {
	//	for id, job := range s.jobs {
	//		next := job.next(time.Time{})
	//	}
}
