package gocron

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type executor struct {
	ctx              context.Context
	cancel           context.CancelFunc
	jobsIDsIn        chan uuid.UUID
	jobIDsOut        chan uuid.UUID
	jobOutRequest    chan jobOutRequest
	stopTimeout      time.Duration
	done             chan error
	singletonRunners map[uuid.UUID]singletonRunner
	limitMode        *limitMode
	elector          Elector
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
				j := requestJob(e.ctx, id, e.jobOutRequest)
				if j == nil {
					continue
				}
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
					go func(j internalJob) {
						e.runJob(j)
						wg.Done()
					}(*j)
				}
			}

		case <-e.ctx.Done():
			waitForJobs := make(chan struct{})
			waitForSingletons := make(chan struct{})

			waiterCtx, waiterCancel := context.WithCancel(context.Background())

			go func() {
				wg.Wait()
				select {
				case <-waiterCtx.Done():
					return
				default:
				}
				waitForJobs <- struct{}{}
			}()
			go func() {
				for _, sr := range e.singletonRunners {
					<-sr.done
				}
				select {
				case <-waiterCtx.Done():
					return
				default:
				}
				waitForSingletons <- struct{}{}
			}()

			var count int
			timeout := time.Now().Add(e.stopTimeout)
			for time.Now().Before(timeout) && count < 2 {
				select {
				case <-waitForJobs:
					count++
				case <-waitForSingletons:
					count++
				default:
				}
			}
			if count < 2 {
				e.done <- ErrStopTimedOut
			} else {
				e.done <- nil
			}
			waiterCancel()
			return
		}
	}
}

func (e *executor) singletonRunner(in chan uuid.UUID, done chan struct{}) {
	for {
		select {
		case id := <-in:
			j := requestJob(e.ctx, id, e.jobOutRequest)
			if j != nil {
				e.runJob(*j)
			}
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
			j := requestJob(e.ctx, id, e.jobOutRequest)
			if j != nil {
				e.runJob(*j)
			}

			// remove the limiter block to allow another job to be scheduled
			if e.limitMode.mode == LimitModeReschedule {
				<-e.limitMode.rescheduleLimiter
			}
		case <-e.ctx.Done():
			done <- struct{}{}
			return
		}
	}
}

func (e *executor) runJob(j internalJob) {
	select {
	case <-e.ctx.Done():
	default:
		if e.elector != nil {
			if err := e.elector.IsLeader(j.ctx); err != nil {
				return
			}
		}
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
