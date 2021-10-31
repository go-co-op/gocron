package gocron

import (
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
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
