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
	stopCh         chan struct{}
	stoppedCh      chan struct{}
	limitMode      limitMode
	maxRunningJobs *semaphore.Weighted
}

func newExecutor() executor {
	return executor{
		jobFunctions: make(chan jobFunction, 1),
		stopCh:       make(chan struct{}),
		stoppedCh:    make(chan struct{}),
	}
}

func runJob(f jobFunction) {
	f.runCount.Add(1)
	f.isRunning.Store(true)
	callJobFunc(f.eventListeners.onBeforeJobExecution)
	callJobFuncWithParams(f.function, f.parameters)
	callJobFunc(f.eventListeners.onAfterJobExecution)
	f.isRunning.Store(false)
}

func (jf *jobFunction) singletonRunner() {
	for {
		select {
		case <-jf.ctx.Done():
			return
		default:
			if jf.singletonQueue.Load() != 0 {
				runJob(*jf)
				jf.singletonQueue.Add(-1)
			}
		}
	}
}

func (e *executor) start() {
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

				switch f.runConfig.mode {
				case defaultMode:
					runJob(f)
				case singletonMode:
					f.singletonQueue.Add(1)
				}
			}()
		case <-e.stopCh:
			cancel()
			runningJobsWg.Wait()
			close(e.stoppedCh)
			return
		}
	}
}

func (e *executor) stop() {
	close(e.stopCh)
	<-e.stoppedCh
}
