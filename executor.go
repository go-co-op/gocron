package gocron

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
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
	//
	// Warning: do not use this mode if your jobs will continue to stack
	// up beyond the ability of the limit workers to keep up. An example of
	// what NOT to do:
	//
	//     s.Every("1s").Do(func() {
	//         // this will result in an ever-growing number of goroutines
	//    	   // blocked trying to send to the buffered channel
	//         time.Sleep(10 * time.Minute)
	//     })

	WaitMode
)

type executor struct {
	jobFunctions chan jobFunction   // the chan upon which the jobFunctions are passed in from the scheduler
	ctx          context.Context    // used to tell the executor to stop
	cancel       context.CancelFunc // used to tell the executor to stop
	wg           *sync.WaitGroup    // used by the scheduler to wait for the executor to stop
	jobsWg       *sync.WaitGroup    // used by the executor to wait for all jobs to finish
	singletonWgs *sync.Map          // used by the executor to wait for the singleton runners to complete

	limitMode               limitMode        // when SetMaxConcurrentJobs() is set upon the scheduler
	limitModeMaxRunningJobs int              // stores the maximum number of concurrently running jobs
	limitModeFuncsRunning   *atomic.Int64    // tracks the count of limited mode funcs running
	limitModeFuncWg         *sync.WaitGroup  // allow the executor to wait for limit mode functions to wrap up
	limitModeQueue          chan jobFunction // pass job functions to the limit mode workers
	limitModeRunningJobs    *atomic.Int64    // tracks the count of running jobs to check against the max
	stopped                 *atomic.Bool     // allow workers to drain the buffered limitModeQueue

	distributedLocker Locker // support running jobs across multiple instances
}

func newExecutor() executor {
	e := executor{
		jobFunctions:          make(chan jobFunction, 1),
		singletonWgs:          &sync.Map{},
		limitModeFuncsRunning: &atomic.Int64{},
		limitModeFuncWg:       &sync.WaitGroup{},
		limitModeRunningJobs:  &atomic.Int64{},
		limitModeQueue:        make(chan jobFunction, 1000),
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
			jf.singletonQueue = make(chan struct{}, 1000)
			jf.stopped.Store(false)
			return
		case <-jf.singletonQueue:
			if !jf.stopped.Load() {
				runJob(*jf)
			}
		}
	}
}

func (e *executor) limitModeRunner() {
	e.limitModeFuncWg.Add(1)
	for {
		select {
		case <-e.ctx.Done():
			e.limitModeFuncsRunning.Add(-1)
			e.limitModeFuncWg.Done()
			return
		case jf := <-e.limitModeQueue:
			if !e.stopped.Load() {
				runJob(jf)
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

	e.stopped = &atomic.Bool{}
	go e.run()
}

func (e *executor) run() {
	for {
		select {
		case f := <-e.jobFunctions:
			if e.stopped.Load() {
				continue
			}

			if e.limitModeMaxRunningJobs > 0 {
				countRunning := e.limitModeFuncsRunning.Load()
				if countRunning < int64(e.limitModeMaxRunningJobs) {
					diff := int64(e.limitModeMaxRunningJobs) - countRunning
					for i := int64(0); i < diff; i++ {
						go e.limitModeRunner()
						e.limitModeFuncsRunning.Add(1)
					}
				}
			}

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

				if e.limitModeMaxRunningJobs > 0 {
					switch e.limitMode {
					case RescheduleMode:
						if e.limitModeRunningJobs.Load() < int64(e.limitModeMaxRunningJobs) {
							e.limitModeQueue <- f
						}
					case WaitMode:
						e.limitModeQueue <- f
					}
					return
				}

				switch f.runConfig.mode {
				case defaultMode:
					if e.distributedLocker != nil {
						l, err := e.distributedLocker.Lock(f.ctx, f.name)
						if err != nil || l == nil {
							return
						}
						defer func() {
							durationToNextRun := time.Until(f.jobFuncNextRun)
							if durationToNextRun > time.Second*5 {
								durationToNextRun = time.Second * 5
							}
							if durationToNextRun > time.Millisecond*100 {
								timeToSleep := time.Duration(float64(durationToNextRun) * 0.9)
								time.Sleep(timeToSleep)
							}
							_ = l.Unlock(f.ctx)
						}()
					}
					runJob(f)
				case singletonMode:
					e.singletonWgs.Store(f.singletonWg, struct{}{})

					if !f.singletonRunnerOn.Load() {
						go f.singletonRunner()
					}
					f.singletonQueue <- struct{}{}
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
	e.stopped.Store(true)
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
	if e.limitModeMaxRunningJobs > 0 {
		e.limitModeFuncWg.Wait()
	}
}
