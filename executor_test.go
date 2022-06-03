package gocron

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ExecutorExecute(t *testing.T) {
	e := newExecutor()

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go e.start()

	var runState = int64(0)
	e.jobFunctions <- jobFunction{
		name: "test_fn",
		function: func(arg string) {
			assert.Equal(t, arg, "test")
			wg.Done()
		},
		parameters: []interface{}{"test"},
		runState:   &runState,
	}

	e.stop()
	wg.Wait()
}

func Test_ExecutorPanicHandling(t *testing.T) {
	panicHandled := false
	PanicHandler = func(jobName string, recoverData interface{}) {
		fmt.Println("PanicHandler calld:")
		fmt.Println("panic in " + jobName)
		fmt.Println(recoverData)
		panicHandled = true
	}

	e := newExecutor()

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go e.start()

	var runState = int64(0)
	e.jobFunctions <- jobFunction{
		name: "test_fn",
		function: func() {
			defer wg.Done()
			a := make([]string, 0)
			a[0] = "This will panic"
		},
		parameters: nil,
		runState:   &runState,
	}

	e.stop()
	wg.Wait()

	assert.Equal(t, panicHandled, true)
}
