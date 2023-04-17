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
	jobFunctions   chan jobFunction   // the chan upon which the jobFunctions are passed in from the scheduler
	ctx            context.Context    // used to tell the executor to stop
	cancel         context.CancelFunc // used to tell the executor to stop
	wg             *sync.WaitGroup    // used by the scheduler to wait for the executor to stop
	jobsWg         *sync.WaitGroup    // used by the executor to wait for all jobs to finish
	singletonWgs   *sync.Map          // used by the executor to wait for the singleton runners to complete
	limitMode      limitMode          // when SetMaxConcurrentJobs() is set upon the scheduler
	maxRunningJobs *semaphore.Weighted
}

func newExecutor() executor {
	e := executor{
		jobFunctions: make(chan jobFunction, 1),
		singletonWgs: &sync.Map{},
		wg:           &sync.WaitGroup{},
	}
	e.wg.Add(1)
	return e
}

func runJob(f jobFunction) {
	f.runStartCount.Add(1)
	f.isRunning.Store(true)
	callJobFunc(f.eventListeners.onBeforeJobExecution)
	callJobFuncWithParams(f.function, f.parameters)
	callJobFunc(f.eventListeners.onAfterJobExecution)
	f.isRunning.Store(false)
	f.runFinishCount.Add(1)
}

func (jf *jobFunction) singletonRunner() {
	jf.singletonRunnerOn.Store(true)
	jf.singletonWg.Add(1)
	for {
		select {
		case <-jf.ctx.Done():
			jf.singletonWg.Done()
			jf.singletonRunnerOn.Store(false)
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
	for {
		select {
		case f := <-e.jobFunctions:
			e.jobsWg.Add(1)
			go func() {
				defer e.jobsWg.Done()

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
							case <-e.ctx.Done():
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
					e.singletonWgs.Store(f.singletonWg, struct{}{})

					if !f.singletonRunnerOn.Load() {
						go f.singletonRunner()
					}

					f.singletonQueue.Add(1)
				}
			}()
		case <-e.ctx.Done():
			e.jobsWg.Wait()
			e.wg.Done()
			return
		}
	}
}

func (e *executor) stop() {
	e.cancel()
	e.wg.Wait()
	if e.singletonWgs != nil {
		e.singletonWgs.Range(func(key, value any) bool {
			if wg, ok := key.(*sync.WaitGroup); ok {
				wg.Wait()
			}
			return true
		})
	}
}
