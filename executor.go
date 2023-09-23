package gocron

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

type executor struct {
	ctx             context.Context
	cancel          context.CancelFunc
	schCtx          context.Context
	jobsIDsIn       chan uuid.UUID
	jobIDsOut       chan uuid.UUID
	jobOutRequest   chan jobOutRequest
	shutdownTimeout time.Duration
}

func (e *executor) start() {
	wg := sync.WaitGroup{}
	for {
		select {
		case id := <-e.jobsIDsIn:
			wg.Add(1)
			go func() {
				j := requestJob(id, e.jobOutRequest)
				err := callJobFuncWithParams(j.function, j.parameters)
				if err != nil {
					log.Printf("error calling job function: %s\n", err)
				}
				e.jobIDsOut <- j.id
				wg.Done()
			}()

		case <-e.schCtx.Done():
			waitTimeout(&wg, e.shutdownTimeout)
			e.cancel()
			return
		}
	}
}
