package gocron

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
)

func Test_ExecutorExecute(t *testing.T) {
	e := newExecutor()

	wg := &sync.WaitGroup{}
	wg.Add(1)
	e.start()

	e.jobFunctions <- jobFunction{
		funcName: "test_fn",
		function: func(arg string) {
			assert.Equal(t, arg, "test")
			wg.Done()
		},
		parameters:     []interface{}{"test"},
		isRunning:      atomic.NewBool(false),
		runStartCount:  atomic.NewInt64(0),
		runFinishCount: atomic.NewInt64(0),
	}

	wg.Wait()
	e.stop()
}

func Test_ExecutorPanicHandling(t *testing.T) {
	panicHandled := make(chan bool, 1)

	handler := func(jobName string, recoverData interface{}) {
		panicHandled <- true
	}

	SetPanicHandler(handler)

	e := newExecutor()

	wg := &sync.WaitGroup{}
	wg.Add(1)
	e.start()

	e.jobFunctions <- jobFunction{
		funcName: "test_fn",
		function: func() {
			defer wg.Done()
			a := make([]string, 0)
			a[0] = "This will panic"
		},
		parameters:     nil,
		isRunning:      atomic.NewBool(false),
		runStartCount:  atomic.NewInt64(0),
		runFinishCount: atomic.NewInt64(0),
	}

	wg.Wait()
	e.stop()

	state := <-panicHandled
	assert.Equal(t, state, true)
}
