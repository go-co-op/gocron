package gocron

import (
	"sync"
)

type executor struct {
	jobFunctions chan jobFunction
	stop         chan struct{}
}

func newExecutor() executor {
	return executor{
		jobFunctions: make(chan jobFunction, 1),
		stop:         make(chan struct{}, 1),
	}
}

func (e *executor) start() {
	wg := sync.WaitGroup{}
	for {
		select {
		case f := <-e.jobFunctions:
			wg.Add(1)
			go func() {
				defer wg.Done()

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
			wg.Wait()
			return
		}
	}
}
