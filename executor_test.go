package gocron

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
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

type testReporter struct {
	lock    sync.Mutex
	results []JobResult
}

func (r *testReporter) ReportJobResult(result JobResult) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.results = append(r.results, result)
}

func Test_ExecutorJobReport(t *testing.T) {
	reporter := &testReporter{lock: sync.Mutex{}, results: make([]JobResult, 0)}
	e := newExecutor()
	e.resultReporter = reporter

	wg := &sync.WaitGroup{}
	wg.Add(2)
	e.start()

	uuids := []uuid.UUID{uuid.New(), uuid.New()}

	e.jobFunctions <- jobFunction{
		id:       uuids[0],
		jobName:  "test_fn",
		funcName: "test_fn",
		function: func() {
			wg.Done()
		},
		parameters:     nil,
		isRunning:      atomic.NewBool(false),
		runStartCount:  atomic.NewInt64(0),
		runFinishCount: atomic.NewInt64(0),
		jobRunTimes:    &jobRunTimes{nextRun: time.Now()},
	}

	mockedErr := errors.New("mocked error")
	e.jobFunctions <- jobFunction{
		id:       uuids[1],
		jobName:  "test_fn",
		funcName: "test_fn",
		function: func() error {
			wg.Done()
			return mockedErr
		},
		parameters:     nil,
		isRunning:      atomic.NewBool(false),
		runStartCount:  atomic.NewInt64(0),
		runFinishCount: atomic.NewInt64(0),
		jobRunTimes:    &jobRunTimes{nextRun: time.Now()},
	}

	wg.Wait()
	e.stop()

	assert.Len(t, reporter.results, 2)
	var result1 JobResult
	var result2 JobResult
	if reporter.results[0].UUID == uuids[0] {
		result1 = reporter.results[0]
		result2 = reporter.results[1]
	} else {
		result1 = reporter.results[1]
		result2 = reporter.results[0]
	}
	assert.NoError(t, result1.Err)
	assert.Error(t, result2.Err)
}
