package gocron

import "context"

type executor struct {
	ctx           context.Context
	jobs          chan job
	jobsCompleted chan job
}

func (e *executor) start() {
	select {
	case task := <-e.jobs:

	case <-e.ctx.Done():

	}
}
