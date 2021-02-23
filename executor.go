package gocron

import (
	"context"
	"sync"

	"golang.org/x/sync/semaphore"
)

const (
	// default is that if a limit on maximum concurrent jobs is set
	// and the limit is reached, a job will skip it's run and try
	// again on the next occurrence in the schedule
	RescheduleMode limitMode = iota

	// in wait mode if a limit on maximum concurrent jobs is set
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
	stop           chan struct{}
	limitMode      limitMode
	maxRunningJobs *semaphore.Weighted
}

func newExecutor() executor {
	return executor{
		jobFunctions: make(chan jobFunction, 1),
		stop:         make(chan struct{}, 1),
	}
}

func (e *executor) start() {
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())

	for {
		select {
		case f := <-e.jobFunctions:
			wg.Add(1)
			go func() {
				defer wg.Done()

				if e.maxRunningJobs != nil {
					if !e.maxRunningJobs.TryAcquire(1) {

						switch e.limitMode {
						case RescheduleMode:
							return
						case WaitMode:
							for {
								select {
								case <-ctx.Done():
									return
								default:
								}

								if e.maxRunningJobs.TryAcquire(1) {
									break
								}
							}
						}
					}

					defer e.maxRunningJobs.Release(1)
				}

				switch f.runConfig.mode {
				case defaultMode:
					callJobFuncWithParams(f.functions[f.name], f.params[f.name])
				case singletonMode:
					_, _, _ = f.limiter.Do("main", func() (interface{}, error) {
						callJobFuncWithParams(f.functions[f.name], f.params[f.name])
						return nil, nil
					})
				}
			}()
		case <-e.stop:
			cancel()
			wg.Wait()
			return
		}
	}
}
