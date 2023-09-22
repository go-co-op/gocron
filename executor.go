package gocron

import (
	"context"
	"log"

	"github.com/google/uuid"
)

type executor struct {
	ctx           context.Context
	jobsIDsIn     chan uuid.UUID
	jobIDsOut     chan uuid.UUID
	jobOutRequest chan jobOutRequest
}

func (e *executor) start() {
	select {
	case id := <-e.jobsIDsIn:
		go func() {
			j := requestJob(id, e.jobOutRequest)
			err := callJobFuncWithParams(j.function, j.parameters)
			if err != nil {
				log.Printf("error calling job function: %s\n", err)
			}
			e.jobIDsOut <- j.id
		}()

	case <-e.ctx.Done():
		return
	}
}
