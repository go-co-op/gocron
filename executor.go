package gocron

import (
	"context"
	"sync"
	"sync/atomic"
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
	jobFunctions chan jobFunction   // the chan upon which the jobFunctions are passed in from the scheduler
	ctx          context.Context    // used to tell the executor to stop
	cancel       context.CancelFunc // used to tell the executor to stop
	wg           *sync.WaitGroup    // used by the scheduler to wait for the executor to stop
	jobsWg       *sync.WaitGroup    // used by the executor to wait for all jobs to finish
	singletonWgs *sync.Map          // used by the executor to wait for the singleton runners to complete

	limitMode               limitMode     // when SetMaxConcurrentJobs() is set upon the scheduler
	limitModeMaxRunningJobs int           // stores the maximum number of concurrently running jobs
	limitModeFuncRunning    *atomic.Bool  // tracks whether the function for handling limited run jobs is running
	limitModeQueue          []jobFunction // queues the limited jobs for running when able per limit mode
	limitModeQueueMu        *sync.Mutex   // mutex for the queue
	limitModeRunningJobs    *atomic.Int64 // tracks the count of running jobs to check against the max
}

func newExecutor() executor {
	e := executor{
		jobFunctions:         make(chan jobFunction, 1),
		singletonWgs:         &sync.Map{},
		limitModeFuncRunning: &atomic.Bool{},
		limitModeQueueMu:     &sync.Mutex{},
		limitModeRunningJobs: &atomic.Int64{},
	}
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

func (e *executor) limitModeRunner() {
	for {
		select {
		case <-e.ctx.Done():
			e.limitModeQueueMu.Lock()
			e.limitModeQueue = nil
			e.limitModeQueueMu.Unlock()
			e.limitModeFuncRunning.Store(false)
			return
		default:
			e.limitModeQueueMu.Lock()
			if e.limitModeQueue != nil && len(e.limitModeQueue) > 0 && e.limitModeRunningJobs.Load() < int64(e.limitModeMaxRunningJobs) {
				jf := e.limitModeQueue[0]
				e.limitModeQueue = e.limitModeQueue[1:]
				e.limitModeQueueMu.Unlock()

				e.limitModeRunningJobs.Store(e.limitModeRunningJobs.Load() + 1)

				select {
				case <-jf.ctx.Done():
					continue
				default:
					e.jobsWg.Add(1)
					go func() {
						runJob(jf)
						e.jobsWg.Done()
						e.limitModeRunningJobs.Store(e.limitModeRunningJobs.Load() - 1)
					}()
				}
			} else {
				e.limitModeQueueMu.Unlock()
			}
		}
	}
}

func (e *executor) start() {
	e.wg = &sync.WaitGroup{}
	e.wg.Add(1)

	stopCtx, cancel := context.WithCancel(context.Background())
	e.ctx = stopCtx
	e.cancel = cancel

	e.jobsWg = &sync.WaitGroup{}

	go e.run()
}

func (e *executor) run() {
	for {
		select {
		case f := <-e.jobFunctions:

			e.jobsWg.Add(1)
			go func() {
				defer e.jobsWg.Done()

				if e.limitModeMaxRunningJobs > 0 {
					if !e.limitModeFuncRunning.Load() {
						go e.limitModeRunner()
						e.limitModeFuncRunning.Store(true)
					}
				}

				panicHandlerMutex.RLock()
				defer panicHandlerMutex.RUnlock()

				if panicHandler != nil {
					defer func() {
						if r := recover(); r != any(nil) {
							panicHandler(f.name, r)
						}
					}()
				}

				if e.limitModeMaxRunningJobs > 0 {
					switch e.limitMode {
					case RescheduleMode:
						e.limitModeQueueMu.Lock()
						if e.limitModeQueue == nil || len(e.limitModeQueue) < e.limitModeMaxRunningJobs {
							e.limitModeQueue = append(e.limitModeQueue, f)
						}
						e.limitModeQueueMu.Unlock()
					case WaitMode:
						e.limitModeQueueMu.Lock()
						e.limitModeQueue = append(e.limitModeQueue, f)
						e.limitModeQueueMu.Unlock()
					}
					return
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
