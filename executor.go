package gocron

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type executor struct {
	ctx              context.Context
	cancel           context.CancelFunc
	schCtx           context.Context
	jobsIDsIn        chan uuid.UUID
	jobIDsOut        chan uuid.UUID
	jobOutRequest    chan jobOutRequest
	shutdownTimeout  time.Duration
	done             chan error
	singletonRunners map[uuid.UUID]singletonRunner
	limitMode        *limitMode
}

type singletonRunner struct {
	in   chan uuid.UUID
	done chan struct{}
}

type limitMode struct {
	started           bool
	mode              LimitMode
	limit             int
	rescheduleLimiter chan struct{}
	in                chan uuid.UUID
	done              chan struct{}
}

func (e *executor) start() {
	wg := sync.WaitGroup{}
	for {
		select {
		case id := <-e.jobsIDsIn:
			if e.limitMode != nil {
				if !e.limitMode.started {
					for i := e.limitMode.limit; i > 0; i-- {
						go e.limitModeRunner(e.limitMode.in, e.limitMode.done)
					}
				}
				if e.limitMode.mode == LimitModeReschedule {
					select {
					case e.limitMode.rescheduleLimiter <- struct{}{}:
						e.limitMode.in <- id
					default:
						// all runners are busy, reschedule the work for later
						// which means we just skip it here and do nothing
						// TODO when metrics are added, this should increment a rescheduled metric
					}
				} else {
					// TODO when metrics are added, this should increment a wait metric
					e.limitMode.in <- id
				}
			} else {
				j := requestJob(id, e.jobOutRequest)
				if j.singletonMode {
					runner, ok := e.singletonRunners[id]
					if !ok {
						runner.in = make(chan uuid.UUID, 1000)
						runner.done = make(chan struct{})
						e.singletonRunners[id] = runner
						go e.singletonRunner(runner.in, runner.done)
					}
					runner.in <- id
				} else {
					wg.Add(1)
					go func(j job) {
						e.runJob(j)
						wg.Done()
					}(j)
				}
			}

		case <-e.schCtx.Done():
			e.cancel()
			c1 := make(chan struct{})
			c2 := make(chan struct{})
			go func() {
				wg.Wait()
				close(c1)
			}()
			go func() {
				for _, sr := range e.singletonRunners {
					<-sr.done
				}
				close(c2)
			}()

			var timedOut bool
			var count int
			for !timedOut && count < 2 {
				select {
				case <-time.After(e.shutdownTimeout):
					timedOut = true
				case <-c1:
					count++
				case <-c2:
					count++
				}
			}
			if timedOut {
				e.done <- fmt.Errorf("gocron: timed out waiting for jobs to finish")
			} else {
				e.done <- nil
			}
			return
		}
	}
}

func (e *executor) singletonRunner(in chan uuid.UUID, done chan struct{}) {
	for {
		select {
		case id := <-in:
			j := requestJob(id, e.jobOutRequest)
			e.runJob(j)
		case <-e.ctx.Done():
			done <- struct{}{}
			return
		}
	}
}

func (e *executor) limitModeRunner(in chan uuid.UUID, done chan struct{}) {
	for {
		select {
		case id := <-in:
			j := requestJob(id, e.jobOutRequest)
			e.runJob(j)
			// remove the limiter block to allow another job to be scheduled
			<-e.limitMode.rescheduleLimiter
		case <-e.ctx.Done():
			done <- struct{}{}
			return
		}
	}
}

func (e *executor) runJob(j job) {
	select {
	case <-j.ctx.Done():
	default:
		_ = callJobFuncWithParams(j.beforeJobRuns, j.id)
		e.jobIDsOut <- j.id
		err := callJobFuncWithParams(j.function, j.parameters...)
		if err != nil {
			_ = callJobFuncWithParams(j.afterJobRunsWithError, j.id, err)
		} else {
			_ = callJobFuncWithParams(j.afterJobRuns, j.id)
		}
	}
}
