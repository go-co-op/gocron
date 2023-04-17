package gocron

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ExecutorExecute(t *testing.T) {
	e := newExecutor()
	stopCtx, cancel := context.WithCancel(context.Background())
	e.ctx = stopCtx
	e.cancel = cancel
	e.jobsWg = &sync.WaitGroup{}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go e.start()

	e.jobFunctions <- jobFunction{
		name: "test_fn",
		function: func(arg string) {
			assert.Equal(t, arg, "test")
			wg.Done()
		},
		parameters:     []any{"test"},
		isRunning:      &atomic.Bool{},
		runStartCount:  &atomic.Int64{},
		runFinishCount: &atomic.Int64{},
	}

	wg.Wait()
	e.stop()
}

func Test_ExecutorPanicHandling(t *testing.T) {
	panicHandled := make(chan bool, 1)

	handler := func(jobName string, recoverData any) {
		panicHandled <- true
	}

	SetPanicHandler(handler)

	e := newExecutor()
	stopCtx, cancel := context.WithCancel(context.Background())
	e.ctx = stopCtx
	e.cancel = cancel
	e.jobsWg = &sync.WaitGroup{}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go e.start()

	e.jobFunctions <- jobFunction{
		name: "test_fn",
		function: func() {
			defer wg.Done()
			a := make([]string, 0)
			a[0] = "This will panic"
		},
		parameters:     nil,
		isRunning:      &atomic.Bool{},
		runStartCount:  &atomic.Int64{},
		runFinishCount: &atomic.Int64{},
	}

	wg.Wait()
	e.stop()

	state := <-panicHandled
	assert.Equal(t, state, true)

}
