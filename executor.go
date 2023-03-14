package gocron

import (
	"context"
	"sync"

	"golang.org/x/sync/semaphore"
)

const (
	// RescheduleMode - the default is that if a limit on maximum
	// concurrent jobs is set and the limit is reached, a job will
	// skip it's run and try again on the next occurrence in the schedule
	RescheduleMode limitMode = iota

	// WaitMode - if a limit on maximum concurrent jobs is set
	// and the limit is reached, a job will wait to try and run
	// until a spot in the limit is freed up.
	//
	// Note: this mode can produce unpredictable results as
	// job execution order isn't guaranteed. For example, a job that
	// executes frequently may pile up in the wait queue and be executed
	// many times back to back when the queue opens.
	WaitMode
)

type executor struct {
	jobFunctions   chan jobFunction
	stopCh         chan bool
	stoppedCh      chan bool
	limitMode      limitMode
	maxRunningJobs *semaphore.Weighted
}

func newExecutor() executor {
	e := executor{
		jobFunctions: make(chan jobFunction, 1),
		stopCh:       make(chan bool, 1),
		stoppedCh:    make(chan bool, 1),
	}
	e.stopCh <- false
	e.stoppedCh <- false
	return e
}

func (e *executor) cleanupBeforeStart() {
	// if we have a .stop, .start scenario here
	// stoppedCh should should have true
	stopped, open := <-e.stoppedCh
	if stopped {
		// if the job was already stopped
		// recreate the stop channel
		// and the stopped channel
		e.stopCh = make(chan bool, 1)
		e.stopCh <- false
		e.stoppedCh <- false
	} else if open {
		e.stoppedCh <- stopped
	}
}

func (e *executor) start() {
	e.cleanupBeforeStart()
	stopCtx, cancel := context.WithCancel(context.Background())
	runningJobsWg := sync.WaitGroup{}

	for {
		select {
		case f := <-e.jobFunctions:
			runningJobsWg.Add(1)
			go func() {
				defer runningJobsWg.Done()

				panicHandlerMutex.RLock()
				defer panicHandlerMutex.RUnlock()

				if panicHandler != nil {
					defer func() {
						if r := recover(); r != any(nil) {
							panicHandler(f.name, r)
						}
					}()
				}

				if e.maxRunningJobs != nil {
					if !e.maxRunningJobs.TryAcquire(1) {

						switch e.limitMode {
						case RescheduleMode:
							return
						case WaitMode:
							select {
							case <-stopCtx.Done():
								return
							case <-f.ctx.Done():
								return
							default:
							}

							if err := e.maxRunningJobs.Acquire(f.ctx, 1); err != nil {
								break

							}
						}
					}

					defer e.maxRunningJobs.Release(1)
				}

				runJob := func() {
					f.incrementRunState()
					callJobFunc(f.eventListeners.onBeforeJobExecution)
					callJobFuncWithParams(f.function, f.parameters)
					callJobFunc(f.eventListeners.onAfterJobExecution)
					f.decrementRunState()
				}

				switch f.runConfig.mode {
				case defaultMode:
					runJob()
				case singletonMode:
					_, _, _ = f.limiter.Do("main", func() (any, error) {
						select {
						case <-stopCtx.Done():
							return nil, nil
						case <-f.ctx.Done():
							return nil, nil
						default:
						}
						runJob()
						return nil, nil
					})
				}
			}()
		// This logic runs in the case we have a stop signal
		// we use stopCh to determine whether the job was stopped
		// if the the channel is open then it means that the job
		// hasn't been stopped yet.
		// if the job should be stopped, cancel the job
		// close the stop channel and set the stopped channel
		// to true.
		case stop, open := <-e.stopCh:
			if stop {
				cancel()
				runningJobsWg.Wait()
				close(e.stopCh)
				<-e.stoppedCh
				e.stoppedCh <- true
			} else if open {
				e.stopCh <- false
			}
		}
	}
}

func (e *executor) stop() {
	// set the channel value to true
	// to make the job stop
	_, open := <-e.stopCh
	if open {
		e.stopCh <- true
	}
}
