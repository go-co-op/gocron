package v2

var _ Scheduler = (*scheduler)(nil)

type Scheduler interface {
	NewJob(Job, error) error
}

type scheduler struct {
}

func (s scheduler) NewJob(j Job, err error) error {
	//TODO implement me
	panic("implement me")
}
