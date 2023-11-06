package gocron

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

type executor struct {
	ctx              context.Context
	cancel           context.CancelFunc
	logger           Logger
	stopCh           chan struct{}
	jobsIDsIn        chan uuid.UUID
	jobIDsOut        chan uuid.UUID
	jobOutRequest    chan jobOutRequest
	stopTimeout      time.Duration
	done             chan error
	singletonRunners map[uuid.UUID]singletonRunner
	limitMode        *limitModeConfig
	elector          Elector
}

type singletonRunner struct {
	in                chan uuid.UUID
	done              chan struct{}
	rescheduleLimiter chan struct{}
}

type limitModeConfig struct {
	started           bool
	mode              LimitMode
	limit             uint
	rescheduleLimiter chan struct{}
	in                chan uuid.UUID
	done              chan struct{}
}

func (e *executor) start() {
	e.logger.Debug("executor started")

	// creating the executor's context here as the executor
	// is the only goroutine that should access this context
	// any other uses within the executor should create a context
	// using the executor context as parent.
	e.ctx, e.cancel = context.WithCancel(context.Background())

	// the standardJobsWg tracks
	standardJobsWg := waitGroupWithMutex{
		wg: sync.WaitGroup{},
		mu: sync.Mutex{},
	}

	// start the for leap that is the executor
	// selecting on channels for work to do
	for {
		select {
		// job ids in are sent from 1 of 2 places:
		// 1. the scheduler sends directly when jobs
		//    are run immediately.
		// 2. sent from time.AfterFuncs in which job schedules
		// 	  are spun up by the scheduler
		case id := <-e.jobsIDsIn:
			// this context is used to handle cancellation of the executor
			// on requests for a job to the scheduler via requestJobCtx
			ctx, cancel := context.WithCancel(e.ctx)

			if e.limitMode != nil && !e.limitMode.started {
				// check if we are already running the limit mode runners
				// if not, spin up the required number i.e. limit!
				e.limitMode.started = true
				for i := e.limitMode.limit; i > 0; i-- {
					go e.limitModeRunner("limitMode-"+strconv.Itoa(int(i)), e.limitMode.in, e.limitMode.done, e.limitMode.mode, e.limitMode.rescheduleLimiter)
				}
			}

			// spin off into a goroutine to unblock the executor and
			// allow for processing for more work
			go func() {
				// make sure to cancel the above context per the docs
				// // Canceling this context releases resources associated with it, so code should
				// // call cancel as soon as the operations running in this Context complete.
				defer cancel()

				// check for limit mode - this spins up a separate runner which handles
				// limiting the total number of concurrently running jobs
				if e.limitMode != nil {
					if e.limitMode.mode == LimitModeReschedule {
						select {
						// rescheduleLimiter is a channel the size of the limit
						// this blocks publishing to the channel and keeps
						// the executor from building up a waiting queue
						// and forces rescheduling
						case e.limitMode.rescheduleLimiter <- struct{}{}:
							e.limitMode.in <- id
						default:
							// all runners are busy, reschedule the work for later
							// which means we just skip it here and do nothing
							// TODO when metrics are added, this should increment a rescheduled metric
							e.jobIDsOut <- id
						}
					} else {
						// since we're not using LimitModeReschedule, but instead using LimitModeWait
						// we do want to queue up the work to the limit mode runners and allow them
						// to work through the channel backlog. A hard limit of 1000 is in place
						// at which point this call would block.
						// TODO when metrics are added, this should increment a wait metric
						e.limitMode.in <- id
					}
				} else {
					// no limit mode, so we're either running a regular job or
					// a job with a singleton mode
					//
					// get the job, so we can figure out what kind it is and how
					// to execute it
					j := requestJobCtx(ctx, id, e.jobOutRequest)
					if j == nil {
						// safety check as it'd be strange bug if this occurred
						// TODO add a log line here
						return
					}
					if j.singletonMode {
						// for singleton mode, get the existing runner for the job
						// or spin up a new one
						runner, ok := e.singletonRunners[id]
						if !ok {
							runner.in = make(chan uuid.UUID, 1000)
							runner.done = make(chan struct{}, 1)
							if j.singletonLimitMode == LimitModeReschedule {
								runner.rescheduleLimiter = make(chan struct{}, 1)
							}
							e.singletonRunners[id] = runner
							go e.limitModeRunner("singleton-"+id.String(), runner.in, runner.done, j.singletonLimitMode, runner.rescheduleLimiter)
						}

						if j.singletonLimitMode == LimitModeReschedule {
							// reschedule mode uses the limiter channel to check
							// for a running job and reschedules if the channel is full.
							select {
							case runner.rescheduleLimiter <- struct{}{}:
								runner.in <- id
							default:
								// runner is busy, reschedule the work for later
								// which means we just skip it here and do nothing
								// TODO when metrics are added, this should increment a rescheduled metric
								e.jobIDsOut <- id
							}
						} else {
							// wait mode, fill up that queue
							runner.in <- id
						}
					} else {
						// we've gotten to the basic / standard jobs --
						// the ones without anything special that just want
						// to be run. Add to the WaitGroup so that
						// stopping or shutting down can wait for the jobs to
						// complete.
						standardJobsWg.Add(1)
						go func(j internalJob) {
							e.runJob(j)
							standardJobsWg.Done()
						}(*j)
					}
				}
			}()
		case <-e.stopCh:
			e.logger.Debug("stopping executor")
			// we've been asked to stop. This is either because the scheduler has been told
			// to stop all jobs or the scheduler has been asked to completely shutdown.
			//
			// cancel tells all the functions to stop their work and send in a done response
			e.cancel()

			// the wait for job channels are used to report back whether we successfully waited
			// for all jobs to complete or if we hit the configured timeout.
			waitForJobs := make(chan struct{}, 1)
			waitForSingletons := make(chan struct{}, 1)
			waitForLimitMode := make(chan struct{}, 1)

			// the waiter context is used to cancel the functions waiting on jobs.
			// this is done to avoid goroutine leaks.
			waiterCtx, waiterCancel := context.WithCancel(context.Background())

			// wait for standard jobs to complete
			go func() {
				e.logger.Debug("waiting for standard jobs to complete")
				go func() {
					// this is done in a separate goroutine, so we aren't
					// blocked by the WaitGroup's Wait call in the event
					// that the waiter context is cancelled.
					// This particular goroutine could leak in the event that
					// some long-running standard job doesn't complete.
					standardJobsWg.Wait()
					e.logger.Debug("standard jobs completed")
					waitForJobs <- struct{}{}
				}()
				<-waiterCtx.Done()
			}()

			// wait for per job singleton limit mode runner jobs to complete
			go func() {
				e.logger.Debug("waiting for singleton jobs to complete")
			For:
				for _, sr := range e.singletonRunners {
					select {
					case <-waiterCtx.Done():
						break For
					case <-sr.done:
					}
				}
				select {
				case <-waiterCtx.Done():
					return
				default:
					e.logger.Debug("singleton jobs completed")
					waitForSingletons <- struct{}{}
				}
			}()

			// wait for limit mode runners to complete
			go func() {
				e.logger.Debug("waiting for limit mode jobs to complete")
				if e.limitMode != nil {
				For:
					for i := e.limitMode.limit; i > 0; i-- {
						select {
						case <-waiterCtx.Done():
							break For
						case <-e.limitMode.done:
						}
					}
				}
				select {
				case <-waiterCtx.Done():
					return
				default:
					e.logger.Debug("limit mode jobs completed")
					waitForLimitMode <- struct{}{}
				}
			}()

			// now either wait for all the jobs to complete,
			// or hit the timeout.
			var count int
			timeout := time.Now().Add(e.stopTimeout)
			for time.Now().Before(timeout) && count < 3 {
				select {
				case <-waitForJobs:
					count++
				case <-waitForSingletons:
					count++
					// emptying the singleton runner map
					// as we'll need to spin up new runners
					// in the event that the scheduler is started again
					e.singletonRunners = make(map[uuid.UUID]singletonRunner)
				case <-waitForLimitMode:
					count++
				default:
				}
			}
			if count < 3 {
				e.done <- ErrStopTimedOut
				e.logger.Debug("executor stopped - timed out")
			} else {
				e.done <- nil
				e.logger.Debug("executor stopped")
			}
			waiterCancel()
			return
		}
	}
}

func (e *executor) limitModeRunner(name string, in chan uuid.UUID, done chan struct{}, limitMode LimitMode, rescheduleLimiter chan struct{}) {
	e.logger.Debug("limitModeRunner starting", "name", name)
	for {
		select {
		case id := <-in:
			ctx, cancel := context.WithCancel(e.ctx)
			j := requestJobCtx(ctx, id, e.jobOutRequest)
			if j != nil {
				e.runJob(*j)
			}
			cancel()

			// remove the limiter block to allow another job to be scheduled
			if limitMode == LimitModeReschedule {
				select {
				case <-rescheduleLimiter:
				default:
				}
			}
		case <-e.ctx.Done():
			e.logger.Debug("limitModeRunner shutting down", "name", name)
			select {
			case done <- struct{}{}:
			default:
			}
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
