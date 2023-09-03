package v2

import "github.com/google/uuid"

var _ Scheduler = (*scheduler)(nil)

type Scheduler interface {
	NewJob(Job, error) error
}

type scheduler struct {
	jobs map[uuid.UUID]job
}

func (s scheduler) NewJob(j Job, err error) error {
	//TODO implement me
	panic("implement me")
}
