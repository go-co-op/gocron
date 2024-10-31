package gocron

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// ci/cd produces a lot of false positive goroutine leaks for reasons
// I have not been able to pin down. All tests pass locally without leaks.
// Tests run in ci will use the TEST_ENV 'ci' to skip running leak detection.
const testEnvLocal = "local"

var testEnv = testEnvLocal

func init() {
	tmp := os.Getenv("TEST_ENV")
	if tmp != "" {
		testEnv = tmp
	}
}

var verifyNoGoroutineLeaks = func(t *testing.T) {
	if testEnv != testEnvLocal {
		return
	}
	goleak.VerifyNone(t)
}

func newTestScheduler(t *testing.T, options ...SchedulerOption) Scheduler {
	// default test options
	out := []SchedulerOption{
		WithLogger(NewLogger(LogLevelDebug)),
		WithStopTimeout(time.Second),
	}

	// append any additional options 2nd to override defaults if needed
	out = append(out, options...)
	s, err := NewScheduler(out...)
	require.NoError(t, err)
	return s
}

var _ Locker = new(errorLocker)

type errorLocker struct{}

func (e errorLocker) Lock(_ context.Context, _ string) (Lock, error) {
	return nil, errors.New("locked")
}

func TestScheduler_OneSecond_NoOptions(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	cronNoOptionsCh := make(chan struct{}, 10)
	durationNoOptionsCh := make(chan struct{}, 10)

	tests := []struct {
		name string
		ch   chan struct{}
		jd   JobDefinition
		tsk  Task
	}{
		{
			"cron",
			cronNoOptionsCh,
			CronJob(
				"* * * * * *",
				true,
			),
			NewTask(
				func() {
					cronNoOptionsCh <- struct{}{}
				},
			),
		},
		{
			"duration",
			durationNoOptionsCh,
			DurationJob(
				time.Second,
			),
			NewTask(
				func() {
					durationNoOptionsCh <- struct{}{}
				},
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t)

			_, err := s.NewJob(tt.jd, tt.tsk)
			require.NoError(t, err)

			s.Start()

			startTime := time.Now()
			var runCount int
			for runCount < 1 {
				<-tt.ch
				runCount++
			}
			require.NoError(t, s.Shutdown())
			stopTime := time.Now()

			select {
			case <-tt.ch:
				t.Fatal("job ran after scheduler was stopped")
			case <-time.After(time.Millisecond * 50):
			}

			runDuration := stopTime.Sub(startTime)
			assert.GreaterOrEqual(t, runDuration, time.Millisecond)
			assert.LessOrEqual(t, runDuration, 1500*time.Millisecond)
		})
	}
}

func TestScheduler_LongRunningJobs(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	if testEnv != testEnvLocal {
		// this test is flaky in ci, but always passes locally
		t.SkipNow()
	}

	durationCh := make(chan struct{}, 10)
	durationSingletonCh := make(chan struct{}, 10)

	tests := []struct {
		name         string
		ch           chan struct{}
		jd           JobDefinition
		tsk          Task
		opts         []JobOption
		options      []SchedulerOption
		expectedRuns int
	}{
		{
			"duration with stop time between executions",
			durationCh,
			DurationJob(
				time.Millisecond * 500,
			),
			NewTask(
				func() {
					time.Sleep(1 * time.Second)
					durationCh <- struct{}{}
				}),
			[]JobOption{WithStopAt(WithStopDateTime(time.Now().Add(time.Millisecond * 1100)))},
			[]SchedulerOption{WithStopTimeout(time.Second * 2)},
			2,
		},
		{
			"duration",
			durationCh,
			DurationJob(
				time.Millisecond * 500,
			),
			NewTask(
				func() {
					time.Sleep(1 * time.Second)
					durationCh <- struct{}{}
				},
			),
			nil,
			[]SchedulerOption{WithStopTimeout(time.Second * 2)},
			3,
		},
		{
			"duration singleton",
			durationSingletonCh,
			DurationJob(
				time.Millisecond * 500,
			),
			NewTask(
				func() {
					time.Sleep(1 * time.Second)
					durationSingletonCh <- struct{}{}
				},
			),
			[]JobOption{WithSingletonMode(LimitModeWait)},
			[]SchedulerOption{WithStopTimeout(time.Second * 5)},
			2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t, tt.options...)

			_, err := s.NewJob(tt.jd, tt.tsk, tt.opts...)
			require.NoError(t, err)

			s.Start()
			time.Sleep(1600 * time.Millisecond)
			require.NoError(t, s.Shutdown())

			var runCount int
			timeout := make(chan struct{})
			go func() {
				time.Sleep(2 * time.Second)
				close(timeout)
			}()
		Outer:
			for {
				select {
				case <-tt.ch:
					runCount++
				case <-timeout:
					break Outer
				}
			}

			assert.Equal(t, tt.expectedRuns, runCount)
		})
	}
}

func TestScheduler_Update(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	durationJobCh := make(chan struct{})

	tests := []struct {
		name               string
		initialJob         JobDefinition
		updateJob          JobDefinition
		tsk                Task
		ch                 chan struct{}
		runCount           int
		updateAfterCount   int
		expectedMinTime    time.Duration
		expectedMaxRunTime time.Duration
	}{
		{
			"duration, updated to another duration",
			DurationJob(
				time.Millisecond * 500,
			),
			DurationJob(
				time.Second,
			),
			NewTask(
				func() {
					durationJobCh <- struct{}{}
				},
			),
			durationJobCh,
			2,
			1,
			time.Second * 1,
			time.Second * 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t)

			j, err := s.NewJob(tt.initialJob, tt.tsk)
			require.NoError(t, err)

			startTime := time.Now()
			s.Start()

			var runCount int
			for runCount < tt.runCount {
				select {
				case <-tt.ch:
					runCount++
					if runCount == tt.updateAfterCount {
						_, err = s.Update(j.ID(), tt.updateJob, tt.tsk)
						require.NoError(t, err)
					}
				default:
				}
			}
			require.NoError(t, s.Shutdown())
			stopTime := time.Now()

			select {
			case <-tt.ch:
				t.Fatal("job ran after scheduler was stopped")
			case <-time.After(time.Millisecond * 50):
			}

			runDuration := stopTime.Sub(startTime)
			assert.GreaterOrEqual(t, runDuration, tt.expectedMinTime)
			assert.LessOrEqual(t, runDuration, tt.expectedMaxRunTime)
		})
	}
}

func TestScheduler_StopTimeout(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	tests := []struct {
		name string
		jd   JobDefinition
		f    any
		opts []JobOption
	}{
		{
			"duration",
			DurationJob(
				time.Millisecond * 100,
			),
			func(testDoneCtx context.Context) {
				select {
				case <-time.After(1 * time.Second):
				case <-testDoneCtx.Done():
				}
			},
			nil,
		},
		{
			"duration singleton",
			DurationJob(
				time.Millisecond * 100,
			),
			func(testDoneCtx context.Context) {
				select {
				case <-time.After(1 * time.Second):
				case <-testDoneCtx.Done():
				}
			},
			[]JobOption{WithSingletonMode(LimitModeWait)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDoneCtx, cancel := context.WithCancel(context.Background())
			s := newTestScheduler(t,
				WithStopTimeout(time.Millisecond*100),
			)

			_, err := s.NewJob(tt.jd, NewTask(tt.f, testDoneCtx), tt.opts...)
			require.NoError(t, err)

			s.Start()
			assert.ErrorIs(t, err, s.Shutdown())
			cancel()
			time.Sleep(2 * time.Second)
		})
	}
}

func TestScheduler_Shutdown(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	t.Run("start, stop, start, shutdown", func(t *testing.T) {
		s := newTestScheduler(t,
			WithStopTimeout(time.Second),
		)

		_, err := s.NewJob(
			DurationJob(
				50*time.Millisecond,
			),
			NewTask(
				func() {},
			),
			WithStartAt(
				WithStartImmediately(),
			),
		)
		require.NoError(t, err)

		s.Start()
		require.NoError(t, s.StopJobs())

		s.Start()

		require.NoError(t, s.Shutdown())
	})

	t.Run("calling Job methods after shutdown errors", func(t *testing.T) {
		s := newTestScheduler(t,
			WithStopTimeout(time.Second),
		)
		j, err := s.NewJob(
			DurationJob(
				100*time.Millisecond,
			),
			NewTask(
				func() {},
			),
			WithStartAt(
				WithStartImmediately(),
			),
		)
		require.NoError(t, err)

		s.Start()
		require.NoError(t, s.Shutdown())

		_, err = j.LastRun()
		assert.ErrorIs(t, err, ErrJobNotFound)

		_, err = j.NextRun()
		assert.ErrorIs(t, err, ErrJobNotFound)
	})
}

func TestScheduler_NewJob(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	tests := []struct {
		name string
		jd   JobDefinition
		tsk  Task
		opts []JobOption
	}{
		{
			"cron with timezone",
			CronJob(
				"CRON_TZ=America/Chicago * * * * * *",
				true,
			),
			NewTask(
				func() {},
			),
			nil,
		},
		{
			"cron with timezone, no seconds",
			CronJob(
				"CRON_TZ=America/Chicago * * * * *",
				false,
			),
			NewTask(
				func() {},
			),
			nil,
		},
		{
			"random duration",
			DurationRandomJob(
				time.Second,
				time.Second*5,
			),
			NewTask(
				func() {},
			),
			nil,
		},
		{
			"daily",
			DailyJob(
				1,
				NewAtTimes(
					NewAtTime(1, 0, 0),
				),
			),
			NewTask(
				func() {},
			),
			nil,
		},
		{
			"weekly",
			WeeklyJob(
				1,
				NewWeekdays(time.Monday),
				NewAtTimes(
					NewAtTime(1, 0, 0),
				),
			),
			NewTask(
				func() {},
			),
			nil,
		},
		{
			"monthly",
			MonthlyJob(
				1,
				NewDaysOfTheMonth(1, -1),
				NewAtTimes(
					NewAtTime(1, 0, 0),
				),
			),
			NewTask(
				func() {},
			),
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t)

			_, err := s.NewJob(tt.jd, tt.tsk, tt.opts...)
			require.NoError(t, err)

			s.Start()
			require.NoError(t, s.Shutdown())
		})
	}
}

func TestScheduler_NewJobErrors(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	tests := []struct {
		name string
		jd   JobDefinition
		opts []JobOption
		err  error
	}{
		{
			"cron with timezone",
			CronJob(
				"bad cron",
				true,
			),
			nil,
			ErrCronJobParse,
		},
		{
			"cron invalid date",
			CronJob(
				"* * * 31 FEB *",
				true,
			),
			nil,
			ErrCronJobInvalid,
		},
		{
			"duration job time interval is zero",
			DurationJob(0 * time.Second),
			nil,
			ErrDurationJobIntervalZero,
		},
		{
			"random with bad min/max",
			DurationRandomJob(
				time.Second*5,
				time.Second,
			),
			nil,
			ErrDurationRandomJobMinMax,
		},
		{
			"daily job at times nil",
			DailyJob(
				1,
				nil,
			),
			nil,
			ErrDailyJobAtTimesNil,
		},
		{
			"daily job at time nil",
			DailyJob(
				1,
				NewAtTimes(nil),
			),
			nil,
			ErrDailyJobAtTimeNil,
		},
		{
			"daily job hours out of range",
			DailyJob(
				1,
				NewAtTimes(
					NewAtTime(100, 0, 0),
				),
			),
			nil,
			ErrDailyJobHours,
		},
		{
			"daily job minutes out of range",
			DailyJob(
				1,
				NewAtTimes(
					NewAtTime(1, 100, 0),
				),
			),
			nil,
			ErrDailyJobMinutesSeconds,
		},
		{
			"daily job seconds out of range",
			DailyJob(
				1,
				NewAtTimes(
					NewAtTime(1, 0, 100),
				),
			),
			nil,
			ErrDailyJobMinutesSeconds,
		},
		{
			"daily job interval 0",
			DailyJob(
				0,
				NewAtTimes(
					NewAtTime(1, 0, 0),
				),
			),
			nil,
			ErrDailyJobZeroInterval,
		},
		{
			"weekly job at times nil",
			WeeklyJob(
				1,
				NewWeekdays(time.Monday),
				nil,
			),
			nil,
			ErrWeeklyJobAtTimesNil,
		},
		{
			"weekly job at time nil",
			WeeklyJob(
				1,
				NewWeekdays(time.Monday),
				NewAtTimes(nil),
			),
			nil,
			ErrWeeklyJobAtTimeNil,
		},
		{
			"weekly job weekdays nil",
			WeeklyJob(
				1,
				nil,
				NewAtTimes(
					NewAtTime(1, 0, 0),
				),
			),
			nil,
			ErrWeeklyJobDaysOfTheWeekNil,
		},
		{
			"weekly job hours out of range",
			WeeklyJob(
				1,
				NewWeekdays(time.Monday),
				NewAtTimes(
					NewAtTime(100, 0, 0),
				),
			),
			nil,
			ErrWeeklyJobHours,
		},
		{
			"weekly job minutes out of range",
			WeeklyJob(
				1,
				NewWeekdays(time.Monday),
				NewAtTimes(
					NewAtTime(1, 100, 0),
				),
			),
			nil,
			ErrWeeklyJobMinutesSeconds,
		},
		{
			"weekly job seconds out of range",
			WeeklyJob(
				1,
				NewWeekdays(time.Monday),
				NewAtTimes(
					NewAtTime(1, 0, 100),
				),
			),
			nil,
			ErrWeeklyJobMinutesSeconds,
		},
		{
			"weekly job interval zero",
			WeeklyJob(
				0,
				NewWeekdays(time.Monday),
				NewAtTimes(
					NewAtTime(1, 0, 0),
				),
			),
			nil,
			ErrWeeklyJobZeroInterval,
		},
		{
			"monthly job at times nil",
			MonthlyJob(
				1,
				NewDaysOfTheMonth(1),
				nil,
			),
			nil,
			ErrMonthlyJobAtTimesNil,
		},
		{
			"monthly job at time nil",
			MonthlyJob(
				1,
				NewDaysOfTheMonth(1),
				NewAtTimes(nil),
			),
			nil,
			ErrMonthlyJobAtTimeNil,
		},
		{
			"monthly job days out of range",
			MonthlyJob(
				1,
				NewDaysOfTheMonth(0),
				NewAtTimes(
					NewAtTime(1, 0, 0),
				),
			),
			nil,
			ErrMonthlyJobDays,
		},
		{
			"monthly job days out of range",
			MonthlyJob(
				1,
				nil,
				NewAtTimes(
					NewAtTime(1, 0, 0),
				),
			),
			nil,
			ErrMonthlyJobDaysNil,
		},
		{
			"monthly job hours out of range",
			MonthlyJob(
				1,
				NewDaysOfTheMonth(1),
				NewAtTimes(
					NewAtTime(100, 0, 0),
				),
			),
			nil,
			ErrMonthlyJobHours,
		},
		{
			"monthly job minutes out of range",
			MonthlyJob(
				1,
				NewDaysOfTheMonth(1),
				NewAtTimes(
					NewAtTime(1, 100, 0),
				),
			),
			nil,
			ErrMonthlyJobMinutesSeconds,
		},
		{
			"monthly job seconds out of range",
			MonthlyJob(
				1,
				NewDaysOfTheMonth(1),
				NewAtTimes(
					NewAtTime(1, 0, 100),
				),
			),
			nil,
			ErrMonthlyJobMinutesSeconds,
		},
		{
			"monthly job interval zero",
			MonthlyJob(
				0,
				NewDaysOfTheMonth(1),
				NewAtTimes(
					NewAtTime(1, 0, 0),
				),
			),
			nil,
			ErrMonthlyJobZeroInterval,
		},
		{
			"WithName no name",
			DurationJob(
				time.Second,
			),
			[]JobOption{WithName("")},
			ErrWithNameEmpty,
		},
		{
			"WithStartDateTime is zero",
			DurationJob(
				time.Second,
			),
			[]JobOption{WithStartAt(WithStartDateTime(time.Time{}))},
			ErrWithStartDateTimePast,
		},
		{
			"WithStartDateTime is in the past",
			DurationJob(
				time.Second,
			),
			[]JobOption{WithStartAt(WithStartDateTime(time.Now().Add(-time.Second)))},
			ErrWithStartDateTimePast,
		},
		{
			"WithStartDateTime is later than the end",
			DurationJob(
				time.Second,
			),
			[]JobOption{WithStopAt(WithStopDateTime(time.Now().Add(time.Second))), WithStartAt(WithStartDateTime(time.Now().Add(time.Hour)))},
			ErrStartTimeLaterThanEndTime,
		},
		{
			"WithStopDateTime is earlier than the start",
			DurationJob(
				time.Second,
			),
			[]JobOption{WithStartAt(WithStartDateTime(time.Now().Add(time.Hour))), WithStopAt(WithStopDateTime(time.Now().Add(time.Second)))},
			ErrStopTimeEarlierThanStartTime,
		},
		{
			"oneTimeJob start at is zero",
			OneTimeJob(OneTimeJobStartDateTime(time.Time{})),
			nil,
			ErrOneTimeJobStartDateTimePast,
		},
		{
			"oneTimeJob start at is in past",
			OneTimeJob(OneTimeJobStartDateTime(time.Now().Add(-time.Second))),
			nil,
			ErrOneTimeJobStartDateTimePast,
		},
		{
			"WithDistributedJobLocker is nil",
			DurationJob(
				time.Second,
			),
			[]JobOption{WithDistributedJobLocker(nil)},
			ErrWithDistributedJobLockerNil,
		},
		{
			"WithIdentifier is nil",
			DurationJob(
				time.Second,
			),
			[]JobOption{WithIdentifier(uuid.Nil)},
			ErrWithIdentifierNil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t,
				WithStopTimeout(time.Millisecond*50),
			)

			_, err := s.NewJob(tt.jd, NewTask(func() {}), tt.opts...)
			assert.ErrorIs(t, err, tt.err)
			require.NoError(t, s.Shutdown())
		})
		t.Run(tt.name+" global", func(t *testing.T) {
			s := newTestScheduler(t,
				WithStopTimeout(time.Millisecond*50),
				WithGlobalJobOptions(tt.opts...),
			)

			_, err := s.NewJob(tt.jd, NewTask(func() {}))
			assert.ErrorIs(t, err, tt.err)
			require.NoError(t, s.Shutdown())
		})
	}
}

func TestScheduler_NewJobTask(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	testFuncPtr := func() {}
	testFuncWithParams := func(_, _ string) {}
	testStruct := struct{}{}

	tests := []struct {
		name string
		tsk  Task
		err  error
	}{
		{
			"task nil",
			nil,
			ErrNewJobTaskNil,
		},
		{
			"task not func - nil",
			NewTask(nil),
			ErrNewJobTaskNotFunc,
		},
		{
			"task not func - string",
			NewTask("not a func"),
			ErrNewJobTaskNotFunc,
		},
		{
			"task func is pointer",
			NewTask(&testFuncPtr),
			nil,
		},
		{
			"parameter number does not match",
			NewTask(testFuncWithParams, "one"),
			ErrNewJobWrongNumberOfParameters,
		},
		{
			"parameter type does not match",
			NewTask(testFuncWithParams, "one", 2),
			ErrNewJobWrongTypeOfParameters,
		},
		{
			"parameter number does not match - ptr",
			NewTask(&testFuncWithParams, "one"),
			ErrNewJobWrongNumberOfParameters,
		},
		{
			"parameter type does not match - ptr",
			NewTask(&testFuncWithParams, "one", 2),
			ErrNewJobWrongTypeOfParameters,
		},
		{
			"all good struct",
			NewTask(func(_ struct{}) {}, struct{}{}),
			nil,
		},
		{
			"all good interface",
			NewTask(func(_ interface{}) {}, struct{}{}),
			nil,
		},
		{
			"all good any",
			NewTask(func(_ any) {}, struct{}{}),
			nil,
		},
		{
			"all good slice",
			NewTask(func(_ []struct{}) {}, []struct{}{}),
			nil,
		},
		{
			"all good chan",
			NewTask(func(_ chan struct{}) {}, make(chan struct{})),
			nil,
		},
		{
			"all good pointer",
			NewTask(func(_ *struct{}) {}, &testStruct),
			nil,
		},
		{
			"all good map",
			NewTask(func(_ map[string]struct{}) {}, make(map[string]struct{})),
			nil,
		},
		{
			"all good",
			NewTask(&testFuncWithParams, "one", "two"),
			nil,
		},
		{
			"parameter type does not match - different argument types against variadic parameters",
			NewTask(func(_ ...string) {}, "one", 2),
			ErrNewJobWrongTypeOfParameters,
		},
		{
			"all good string - variadic",
			NewTask(func(_ ...string) {}, "one", "two"),
			nil,
		},
		{
			"all good mixed variadic",
			NewTask(func(_ int, _ ...string) {}, 1, "one", "two"),
			nil,
		},
		{
			"all good struct - variadic",
			NewTask(func(_ ...interface{}) {}, struct{}{}),
			nil,
		},
		{
			"all good no arguments passed in - variadic",
			NewTask(func(_ ...interface{}) {}),
			nil,
		},
		{
			"all good - interface variadic, int, string",
			NewTask(func(_ ...interface{}) {}, 1, "2", 3.0),
			nil,
		},
		{
			"parameter type does not match - different argument types against interface variadic parameters",
			NewTask(func(_ ...io.Reader) {}, os.Stdout, any(3.0)),
			ErrNewJobWrongTypeOfParameters,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t)

			_, err := s.NewJob(DurationJob(time.Second), tt.tsk)
			assert.ErrorIs(t, err, tt.err)
			require.NoError(t, s.Shutdown())
		})
	}
}

func TestScheduler_WithOptionsErrors(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	tests := []struct {
		name string
		opt  SchedulerOption
		err  error
	}{
		{
			"WithClock nil",
			WithClock(nil),
			ErrWithClockNil,
		},
		{
			"WithDistributedElector nil",
			WithDistributedElector(nil),
			ErrWithDistributedElectorNil,
		},
		{
			"WithDistributedLocker nil",
			WithDistributedLocker(nil),
			ErrWithDistributedLockerNil,
		},
		{
			"WithLimitConcurrentJobs limit 0",
			WithLimitConcurrentJobs(0, LimitModeWait),
			ErrWithLimitConcurrentJobsZero,
		},
		{
			"WithLocation nil",
			WithLocation(nil),
			ErrWithLocationNil,
		},
		{
			"WithLogger nil",
			WithLogger(nil),
			ErrWithLoggerNil,
		},
		{
			"WithStopTimeout 0",
			WithStopTimeout(0),
			ErrWithStopTimeoutZeroOrNegative,
		},
		{
			"WithStopTimeout -1",
			WithStopTimeout(-1),
			ErrWithStopTimeoutZeroOrNegative,
		},
		{
			"WithMonitorer nil",
			WithMonitor(nil),
			ErrWithMonitorNil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewScheduler(tt.opt)
			assert.ErrorIs(t, err, tt.err)
		})
	}
}

func TestScheduler_Singleton(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	tests := []struct {
		name        string
		duration    time.Duration
		limitMode   LimitMode
		runCount    int
		expectedMin time.Duration
		expectedMax time.Duration
	}{
		{
			"singleton mode reschedule",
			time.Millisecond * 100,
			LimitModeReschedule,
			3,
			time.Millisecond * 600,
			time.Millisecond * 1100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobRanCh := make(chan struct{}, 10)

			s := newTestScheduler(t,
				WithStopTimeout(1*time.Second),
				WithLocation(time.Local),
			)

			_, err := s.NewJob(
				DurationJob(
					tt.duration,
				),
				NewTask(func() {
					time.Sleep(tt.duration * 2)
					jobRanCh <- struct{}{}
				}),
				WithSingletonMode(tt.limitMode),
			)
			require.NoError(t, err)

			start := time.Now()
			s.Start()

			var runCount int
			for runCount < tt.runCount {
				select {
				case <-jobRanCh:
					runCount++
				case <-time.After(time.Second):
					t.Fatalf("timed out waiting for jobs to run")
				}
			}

			stop := time.Now()
			require.NoError(t, s.Shutdown())

			assert.GreaterOrEqual(t, stop.Sub(start), tt.expectedMin)
			assert.LessOrEqual(t, stop.Sub(start), tt.expectedMax)
		})
	}
}

func TestScheduler_LimitMode(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	tests := []struct {
		name        string
		numJobs     int
		limit       uint
		limitMode   LimitMode
		duration    time.Duration
		expectedMin time.Duration
		expectedMax time.Duration
	}{
		{
			"limit mode reschedule",
			10,
			2,
			LimitModeReschedule,
			time.Millisecond * 100,
			time.Millisecond * 400,
			time.Millisecond * 700,
		},
		{
			"limit mode wait",
			10,
			2,
			LimitModeWait,
			time.Millisecond * 100,
			time.Millisecond * 200,
			time.Millisecond * 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t,
				WithLimitConcurrentJobs(tt.limit, tt.limitMode),
				WithStopTimeout(2*time.Second),
			)

			jobRanCh := make(chan struct{}, 20)

			for i := 0; i < tt.numJobs; i++ {
				_, err := s.NewJob(
					DurationJob(tt.duration),
					NewTask(func() {
						time.Sleep(tt.duration / 2)
						jobRanCh <- struct{}{}
					}),
				)
				require.NoError(t, err)
			}

			start := time.Now()
			s.Start()

			var runCount int
			for runCount < tt.numJobs {
				select {
				case <-jobRanCh:
					runCount++
				case <-time.After(time.Second):
					t.Fatalf("timed out waiting for jobs to run")
				}
			}
			stop := time.Now()
			require.NoError(t, s.Shutdown())

			assert.GreaterOrEqual(t, stop.Sub(start), tt.expectedMin)
			assert.LessOrEqual(t, stop.Sub(start), tt.expectedMax)
		})
	}
}

func TestScheduler_LimitModeAndSingleton(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	tests := []struct {
		name          string
		numJobs       int
		limit         uint
		limitMode     LimitMode
		singletonMode LimitMode
		duration      time.Duration
		expectedMin   time.Duration
		expectedMax   time.Duration
	}{
		{
			"limit mode reschedule",
			10,
			2,
			LimitModeReschedule,
			LimitModeReschedule,
			time.Millisecond * 100,
			time.Millisecond * 400,
			time.Millisecond * 700,
		},
		{
			"limit mode wait",
			10,
			2,
			LimitModeWait,
			LimitModeWait,
			time.Millisecond * 100,
			time.Millisecond * 200,
			time.Millisecond * 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t,
				WithLimitConcurrentJobs(tt.limit, tt.limitMode),
				WithStopTimeout(2*time.Second),
			)

			jobRanCh := make(chan int, 20)

			for i := 0; i < tt.numJobs; i++ {
				jobNum := i
				_, err := s.NewJob(
					DurationJob(tt.duration),
					NewTask(func() {
						time.Sleep(tt.duration / 2)
						jobRanCh <- jobNum
					}),
					WithSingletonMode(tt.singletonMode),
				)
				require.NoError(t, err)
			}

			start := time.Now()
			s.Start()

			jobsRan := make(map[int]int)
			var runCount int
			for runCount < tt.numJobs {
				select {
				case jobNum := <-jobRanCh:
					runCount++
					jobsRan[jobNum]++
				case <-time.After(time.Second):
					t.Fatalf("timed out waiting for jobs to run")
				}
			}
			stop := time.Now()
			require.NoError(t, s.Shutdown())

			assert.GreaterOrEqual(t, stop.Sub(start), tt.expectedMin)
			assert.LessOrEqual(t, stop.Sub(start), tt.expectedMax)
			for _, count := range jobsRan {
				if tt.singletonMode == LimitModeWait {
					assert.Equal(t, 1, count)
				} else {
					assert.LessOrEqual(t, count, 5)
				}
			}
		})
	}
}

func TestScheduler_OneTimeJob_DoesNotCleanupNext(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	schedulerStartTime := time.Date(2024, time.April, 3, 4, 5, 0, 0, time.UTC)

	tests := []struct {
		name      string
		runAt     time.Time
		fakeClock clockwork.FakeClock
		assertErr require.ErrorAssertionFunc
		// asserts things about schedules, advance time and perform new assertions
		advanceAndAsserts []func(
			t *testing.T,
			j Job,
			clock clockwork.FakeClock,
			runs *atomic.Uint32,
		)
	}{
		{
			name:      "exhausted run do does not cleanup next item",
			runAt:     time.Date(2024, time.April, 22, 4, 5, 0, 0, time.UTC),
			fakeClock: clockwork.NewFakeClockAt(schedulerStartTime),
			advanceAndAsserts: []func(t *testing.T, j Job, clock clockwork.FakeClock, runs *atomic.Uint32){
				func(t *testing.T, j Job, clock clockwork.FakeClock, runs *atomic.Uint32) {
					require.Equal(t, uint32(0), runs.Load())

					// last not initialized
					lastRunAt, err := j.LastRun()
					require.NoError(t, err)
					require.Equal(t, time.Time{}, lastRunAt)

					// next is now
					expected := time.Date(2024, time.April, 22, 4, 5, 0, 0, time.UTC)
					nextRunAt, err := j.NextRun()
					require.NoError(t, err)
					require.Equal(t, expected, nextRunAt.UTC())

					// advance and eventually run
					oneSecondAfterNextRun := expected.Add(1 * time.Second)

					clock.Advance(oneSecondAfterNextRun.Sub(schedulerStartTime))
					require.Eventually(t, func() bool {
						return assert.Equal(t, uint32(1), runs.Load())
					}, 3*time.Second, 100*time.Millisecond)

					// last was run
					lastRunAt, err = j.LastRun()
					require.NoError(t, err)
					require.WithinDuration(t, expected, lastRunAt, 1*time.Second)

					nextRunAt, err = j.NextRun()
					require.NoError(t, err)
					require.Equal(t, time.Time{}, nextRunAt)
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t, WithClock(tt.fakeClock), WithLocation(time.UTC))
			t.Cleanup(func() {
				require.NoError(t, s.Shutdown())
			})

			runs := atomic.Uint32{}
			j, err := s.NewJob(
				OneTimeJob(OneTimeJobStartDateTime(tt.runAt)),
				NewTask(func() {
					runs.Add(1)
				}),
			)
			if tt.assertErr != nil {
				tt.assertErr(t, err)
			} else {
				require.NoError(t, err)
				s.Start()

				for _, advanceAndAssert := range tt.advanceAndAsserts {
					advanceAndAssert(t, j, tt.fakeClock, &runs)
				}
			}
		})
	}
}

var _ Elector = (*testElector)(nil)

type testElector struct {
	mu            sync.Mutex
	leaderElected bool
	notLeader     chan struct{}
}

func (t *testElector) IsLeader(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("done")
	default:
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.leaderElected {
		t.notLeader <- struct{}{}
		return fmt.Errorf("already elected leader")
	}
	t.leaderElected = true
	return nil
}

var _ Locker = (*testLocker)(nil)

type testLocker struct {
	mu        sync.Mutex
	jobLocked bool
	notLocked chan struct{}
}

func (t *testLocker) Lock(_ context.Context, _ string) (Lock, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.jobLocked {
		t.notLocked <- struct{}{}
		return nil, fmt.Errorf("job already locked")
	}
	t.jobLocked = true
	return &testLock{}, nil
}

var _ Lock = (*testLock)(nil)

type testLock struct{}

func (t testLock) Unlock(_ context.Context) error {
	return nil
}

func TestScheduler_WithDistributed(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	notLocked := make(chan struct{}, 10)
	notLeader := make(chan struct{}, 10)

	tests := []struct {
		name          string
		count         int
		schedulerOpts []SchedulerOption
		jobOpts       []JobOption
		assertions    func(*testing.T)
	}{
		{
			"3 schedulers with elector",
			3,
			[]SchedulerOption{
				WithDistributedElector(&testElector{notLeader: notLeader}),
			},
			nil,
			func(t *testing.T) {
				timeout := time.Now().Add(1 * time.Second)
				var notLeaderCount int
				for {
					if time.Now().After(timeout) {
						break
					}
					select {
					case <-notLeader:
						notLeaderCount++
					default:
					}
				}
				assert.Equal(t, 2, notLeaderCount)
			},
		},
		{
			"3 schedulers with locker",
			3,
			[]SchedulerOption{
				WithDistributedLocker(&testLocker{notLocked: notLocked}),
			},
			nil,
			func(_ *testing.T) {
				timeout := time.Now().Add(1 * time.Second)
				var notLockedCount int
				for {
					if time.Now().After(timeout) {
						break
					}
					select {
					case <-notLocked:
						notLockedCount++
					default:
					}
				}
			},
		},
		{
			"3 schedulers and job with Distributed locker",
			3,
			nil,
			[]JobOption{
				WithDistributedJobLocker(&testLocker{notLocked: notLocked}),
			},
			func(_ *testing.T) {
				timeout := time.Now().Add(1 * time.Second)
				var notLockedCount int
				for {
					if time.Now().After(timeout) {
						break
					}
					select {
					case <-notLocked:
						notLockedCount++
					default:
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobsRan := make(chan struct{}, 20)
			ctx, cancel := context.WithCancel(context.Background())
			schedulersDone := make(chan struct{}, tt.count)

			for i := tt.count; i > 0; i-- {
				s := newTestScheduler(t,
					tt.schedulerOpts...,
				)
				jobOpts := []JobOption{
					WithStartAt(
						WithStartImmediately(),
					),
				}
				jobOpts = append(jobOpts, tt.jobOpts...)

				go func() {
					s.Start()
					_, err := s.NewJob(
						DurationJob(
							time.Second,
						),
						NewTask(
							func() {
								time.Sleep(100 * time.Millisecond)
								jobsRan <- struct{}{}
							},
						),
						jobOpts...,
					)
					require.NoError(t, err)

					<-ctx.Done()
					err = s.Shutdown()
					require.NoError(t, err)
					schedulersDone <- struct{}{}
				}()
			}

			var runCount int
			select {
			case <-jobsRan:
				cancel()
				runCount++
			case <-time.After(time.Second):
				cancel()
				t.Error("timed out waiting for job to run")
			}

			var doneCount int
			timeout := time.Now().Add(3 * time.Second)
			for doneCount < tt.count && time.Now().After(timeout) {
				select {
				case <-schedulersDone:
					doneCount++
				default:
				}
			}
			close(jobsRan)
			for range jobsRan {
				runCount++
			}

			assert.Equal(t, 1, runCount)
			time.Sleep(time.Second)
			tt.assertions(t)
		})
	}
}

func TestScheduler_RemoveJob(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	tests := []struct {
		name   string
		addJob bool
		err    error
	}{
		{
			"success",
			true,
			nil,
		},
		{
			"job not found",
			false,
			ErrJobNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t)

			var id uuid.UUID
			if tt.addJob {
				j, err := s.NewJob(DurationJob(time.Second), NewTask(func() {}))
				require.NoError(t, err)
				id = j.ID()
			} else {
				id = uuid.New()
			}

			err := s.RemoveJob(id)
			assert.ErrorIs(t, err, err)
			require.NoError(t, s.Shutdown())
		})
	}
}

func TestScheduler_JobsWaitingInQueue(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	tests := []struct {
		name            string
		limit           uint
		mode            LimitMode
		startAt         func() OneTimeJobStartAtOption
		expectedInQueue int
	}{
		{
			"with mode wait limit 1",
			1,
			LimitModeWait,
			func() OneTimeJobStartAtOption {
				return OneTimeJobStartDateTime(time.Now().Add(10 * time.Millisecond))
			},
			4,
		},
		{
			"with mode wait limit 10",
			10,
			LimitModeWait,
			func() OneTimeJobStartAtOption {
				return OneTimeJobStartDateTime(time.Now().Add(10 * time.Millisecond))
			},
			0,
		},
		{
			"with mode Reschedule",
			1,
			LimitModeReschedule,
			func() OneTimeJobStartAtOption {
				return OneTimeJobStartDateTime(time.Now().Add(10 * time.Millisecond))
			},
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t, WithLimitConcurrentJobs(tt.limit, tt.mode))
			for i := 0; i <= 4; i++ {
				_, err := s.NewJob(OneTimeJob(tt.startAt()), NewTask(func() { time.Sleep(500 * time.Millisecond) }))
				require.NoError(t, err)
			}
			s.Start()
			time.Sleep(20 * time.Millisecond)
			assert.Equal(t, tt.expectedInQueue, s.JobsWaitingInQueue())
			require.NoError(t, s.Shutdown())
		})
	}
}

func TestScheduler_RemoveLotsOfJobs(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	tests := []struct {
		name    string
		numJobs int
	}{
		{
			"10 successes",
			10,
		},
		{
			"100 successes",
			100,
		},
		{
			"1000 successes",
			1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t)

			var ids []uuid.UUID
			for i := 0; i < tt.numJobs; i++ {
				j, err := s.NewJob(DurationJob(time.Second), NewTask(func() { time.Sleep(20 * time.Second) }))
				require.NoError(t, err)
				ids = append(ids, j.ID())
			}

			for _, id := range ids {
				err := s.RemoveJob(id)
				require.NoError(t, err)
			}

			assert.Len(t, s.Jobs(), 0)
			require.NoError(t, s.Shutdown())
		})
	}
}

func TestScheduler_RemoveJob_RemoveSelf(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	s := newTestScheduler(t)
	s.Start()

	_, err := s.NewJob(
		DurationJob(100*time.Millisecond),
		NewTask(func() {}),
		WithEventListeners(
			BeforeJobRuns(
				func(_ uuid.UUID, _ string) {
					s.RemoveByTags("tag1")
				},
			),
		),
		WithTags("tag1"),
	)
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 400)
	assert.NoError(t, s.Shutdown())
}

func TestScheduler_WithEventListeners(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	listenerRunCh := make(chan error, 1)
	testErr := fmt.Errorf("test error")
	tests := []struct {
		name      string
		tsk       Task
		el        EventListener
		expectRun bool
		expectErr error
	}{
		{
			"AfterJobRuns",
			NewTask(func() {}),
			AfterJobRuns(func(_ uuid.UUID, _ string) {
				listenerRunCh <- nil
			}),
			true,
			nil,
		},
		{
			"AfterJobRunsWithError - error",
			NewTask(func() error { return testErr }),
			AfterJobRunsWithError(func(_ uuid.UUID, _ string, err error) {
				listenerRunCh <- err
			}),
			true,
			testErr,
		},
		{
			"AfterJobRunsWithError - multiple return values, including error",
			NewTask(func() (bool, error) { return false, testErr }),
			AfterJobRunsWithError(func(_ uuid.UUID, _ string, err error) {
				listenerRunCh <- err
			}),
			true,
			testErr,
		},
		{
			"AfterJobRunsWithError - no error",
			NewTask(func() error { return nil }),
			AfterJobRunsWithError(func(_ uuid.UUID, _ string, err error) {
				listenerRunCh <- err
			}),
			false,
			nil,
		},
		{
			"BeforeJobRuns",
			NewTask(func() {}),
			BeforeJobRuns(func(_ uuid.UUID, _ string) {
				listenerRunCh <- nil
			}),
			true,
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t)
			_, err := s.NewJob(
				DurationJob(time.Minute*10),
				tt.tsk,
				WithStartAt(
					WithStartImmediately(),
				),
				WithEventListeners(tt.el),
				WithLimitedRuns(1),
			)
			require.NoError(t, err)

			s.Start()
			if tt.expectRun {
				select {
				case err = <-listenerRunCh:
					assert.ErrorIs(t, err, tt.expectErr)
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for listener to run")
				}
			} else {
				select {
				case <-listenerRunCh:
					t.Fatal("listener ran when it shouldn't have")
				case <-time.After(time.Millisecond * 100):
				}
			}

			require.NoError(t, s.Shutdown())
		})
	}
}

func TestScheduler_WithLocker_WithEventListeners(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	listenerRunCh := make(chan error, 1)
	tests := []struct {
		name      string
		locker    Locker
		tsk       Task
		el        EventListener
		expectRun bool
		expectErr error
	}{
		{
			"AfterLockError",
			errorLocker{},
			NewTask(func() {}),
			AfterLockError(func(_ uuid.UUID, _ string, _ error) {
				listenerRunCh <- nil
			}),
			true,
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t)
			_, err := s.NewJob(
				DurationJob(time.Minute*10),
				tt.tsk,
				WithStartAt(
					WithStartImmediately(),
				),
				WithDistributedJobLocker(tt.locker),
				WithEventListeners(tt.el),
				WithLimitedRuns(1),
			)
			require.NoError(t, err)

			s.Start()
			if tt.expectRun {
				select {
				case err = <-listenerRunCh:
					assert.ErrorIs(t, err, tt.expectErr)
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for listener to run")
				}
			} else {
				select {
				case <-listenerRunCh:
					t.Fatal("listener ran when it shouldn't have")
				case <-time.After(time.Millisecond * 100):
				}
			}

			require.NoError(t, s.Shutdown())
		})
	}
}

func TestScheduler_ManyJobs(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	s := newTestScheduler(t)
	jobsRan := make(chan struct{}, 20000)

	for i := 1; i <= 1000; i++ {
		_, err := s.NewJob(
			DurationJob(
				time.Millisecond*100,
			),
			NewTask(
				func() {
					jobsRan <- struct{}{}
				},
			),
			WithStartAt(WithStartImmediately()),
		)
		require.NoError(t, err)
	}

	s.Start()
	time.Sleep(1 * time.Second)
	require.NoError(t, s.Shutdown())
	close(jobsRan)

	var count int
	for range jobsRan {
		count++
	}

	assert.GreaterOrEqual(t, count, 9900)
	assert.LessOrEqual(t, count, 11000)
}

func TestScheduler_RunJobNow(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	chDuration := make(chan struct{}, 10)
	chMonthly := make(chan struct{}, 10)
	chDurationImmediate := make(chan struct{}, 10)
	chDurationSingleton := make(chan struct{}, 10)
	chOneTime := make(chan struct{}, 10)

	tests := []struct {
		name         string
		ch           chan struct{}
		j            JobDefinition
		fun          any
		opts         []JobOption
		expectedDiff func() time.Duration
		expectedRuns int
	}{
		{
			"duration job",
			chDuration,
			DurationJob(time.Second * 10),
			func() {
				chDuration <- struct{}{}
			},
			nil,
			func() time.Duration {
				return 0
			},
			1,
		},
		{
			"monthly job",
			chMonthly,
			MonthlyJob(1, NewDaysOfTheMonth(1), NewAtTimes(NewAtTime(0, 0, 0))),
			func() {
				chMonthly <- struct{}{}
			},
			nil,
			func() time.Duration {
				return 0
			},
			1,
		},
		{
			"duration job - start immediately",
			chDurationImmediate,
			DurationJob(time.Second * 5),
			func() {
				chDurationImmediate <- struct{}{}
			},
			[]JobOption{
				WithStartAt(
					WithStartImmediately(),
				),
			},
			func() time.Duration {
				return 5 * time.Second
			},
			2,
		},
		{
			"duration job - singleton",
			chDurationSingleton,
			DurationJob(time.Second * 10),
			func() {
				chDurationSingleton <- struct{}{}
				time.Sleep(200 * time.Millisecond)
			},
			[]JobOption{
				WithStartAt(
					WithStartImmediately(),
				),
				WithSingletonMode(LimitModeReschedule),
			},
			func() time.Duration {
				return 10 * time.Second
			},
			1,
		},
		{
			"one time job",
			chOneTime,
			OneTimeJob(OneTimeJobStartImmediately()),
			func() {
				chOneTime <- struct{}{}
			},
			nil,
			nil,
			2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t)

			_, err := s.NewJob(tt.j, NewTask(tt.fun), tt.opts...)
			require.NoError(t, err)

			j := s.Jobs()[0]
			s.Start()

			var nextRunBefore time.Time
			if tt.expectedDiff != nil {
				for ; nextRunBefore.IsZero() || err != nil; nextRunBefore, err = j.NextRun() { //nolint:revive
				}
			}

			assert.NoError(t, err)

			time.Sleep(100 * time.Millisecond)
			require.NoError(t, j.RunNow())
			var runCount int

			select {
			case <-tt.ch:
				runCount++
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for job to run")
			}

			timeout := time.Now().Add(time.Second)
			for time.Now().Before(timeout) {
				select {
				case <-tt.ch:
					runCount++
				default:
				}
			}

			assert.Equal(t, tt.expectedRuns, runCount)

			nextRunAfter, err := j.NextRun()
			if tt.expectedDiff != nil && tt.expectedDiff() > 0 {
				for ; nextRunBefore.IsZero() || nextRunAfter.Equal(nextRunBefore); nextRunAfter, err = j.NextRun() { //nolint:revive
					time.Sleep(100 * time.Millisecond)
				}
			}

			assert.NoError(t, err)
			assert.NoError(t, s.Shutdown())

			if tt.expectedDiff != nil {
				assert.Equal(t, tt.expectedDiff(), nextRunAfter.Sub(nextRunBefore))
			}
		})
	}
}

func TestScheduler_LastRunSingleton(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	if testEnv != testEnvLocal {
		// this test is flaky in ci, but always passes locally
		t.SkipNow()
	}

	tests := []struct {
		name string
		f    func(t *testing.T, j Job, jobRan chan struct{})
	}{
		{
			"simple",
			func(_ *testing.T, _ Job, _ chan struct{}) {},
		},
		{
			"with runNow",
			func(t *testing.T, j Job, jobRan chan struct{}) {
				runTime := time.Now()
				assert.NoError(t, j.RunNow())

				// because we're using wait mode we need to wait here
				// to make sure the job queued with RunNow has finished running
				<-jobRan
				lastRun, err := j.LastRun()
				assert.NoError(t, err)
				assert.LessOrEqual(t, lastRun.Sub(runTime), time.Millisecond*225)
				assert.GreaterOrEqual(t, lastRun.Sub(runTime), time.Millisecond*175)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobRan := make(chan struct{}, 2)
			s := newTestScheduler(t)
			j, err := s.NewJob(
				DurationJob(time.Millisecond*100),
				NewTask(func() {
					jobRan <- struct{}{}
					time.Sleep(time.Millisecond * 200)
				}),
				WithSingletonMode(LimitModeWait),
			)
			require.NoError(t, err)

			startTime := time.Now()
			s.Start()

			lastRun, err := j.LastRun()
			assert.NoError(t, err)
			assert.True(t, lastRun.IsZero())

			<-jobRan

			lastRun, err = j.LastRun()
			assert.NoError(t, err)
			assert.LessOrEqual(t, lastRun.Sub(startTime), time.Millisecond*125)
			assert.GreaterOrEqual(t, lastRun.Sub(startTime), time.Millisecond*75)

			tt.f(t, j, jobRan)

			assert.NoError(t, s.Shutdown())
		})
	}
}

func TestScheduler_OneTimeJob(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	tests := []struct {
		name    string
		startAt func() OneTimeJobStartAtOption
	}{
		{
			"start now",
			func() OneTimeJobStartAtOption {
				return OneTimeJobStartImmediately()
			},
		},
		{
			"start in 100 ms",
			func() OneTimeJobStartAtOption {
				return OneTimeJobStartDateTime(time.Now().Add(100 * time.Millisecond))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobRan := make(chan struct{}, 2)

			s := newTestScheduler(t)

			_, err := s.NewJob(
				OneTimeJob(tt.startAt()),
				NewTask(func() {
					jobRan <- struct{}{}
				}),
			)
			require.NoError(t, err)

			s.Start()

			select {
			case <-jobRan:
			case <-time.After(500 * time.Millisecond):
				t.Fatal("timed out waiting for job to run")
			}

			assert.NoError(t, s.Shutdown())
		})
	}
}

func TestScheduler_AtTimesJob(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	n := time.Now().UTC()

	tests := []struct {
		name      string
		atTimes   []time.Time
		fakeClock clockwork.FakeClock
		assertErr require.ErrorAssertionFunc
		// asserts things about schedules, advance time and perform new assertions
		advanceAndAsserts []func(
			t *testing.T,
			j Job,
			clock clockwork.FakeClock,
			runs *atomic.Uint32,
		)
	}{
		{
			name:      "no at times",
			atTimes:   []time.Time{},
			fakeClock: clockwork.NewFakeClock(),
			assertErr: func(t require.TestingT, err error, _ ...interface{}) {
				require.ErrorIs(t, err, ErrOneTimeJobStartDateTimePast)
			},
		},
		{
			name:      "all in the past",
			atTimes:   []time.Time{n.Add(-1 * time.Second)},
			fakeClock: clockwork.NewFakeClockAt(n),
			assertErr: func(t require.TestingT, err error, _ ...interface{}) {
				require.ErrorIs(t, err, ErrOneTimeJobStartDateTimePast)
			},
		},
		{
			name:      "one run 1 millisecond in the future",
			atTimes:   []time.Time{n.Add(1 * time.Millisecond)},
			fakeClock: clockwork.NewFakeClockAt(n),
			advanceAndAsserts: []func(t *testing.T, j Job, clock clockwork.FakeClock, runs *atomic.Uint32){
				func(t *testing.T, j Job, clock clockwork.FakeClock, runs *atomic.Uint32) {
					require.Equal(t, uint32(0), runs.Load())

					// last not initialized
					lastRunAt, err := j.LastRun()
					require.NoError(t, err)
					require.Equal(t, time.Time{}, lastRunAt)

					// next is now
					nextRunAt, err := j.NextRun()
					require.NoError(t, err)
					require.Equal(t, n.Add(1*time.Millisecond), nextRunAt)

					// advance and eventually run
					clock.Advance(2 * time.Millisecond)
					require.Eventually(t, func() bool {
						return assert.Equal(t, uint32(1), runs.Load())
					}, 3*time.Second, 100*time.Millisecond)

					// last was run
					lastRunAt, err = j.LastRun()
					require.NoError(t, err)
					require.WithinDuration(t, n.Add(1*time.Millisecond), lastRunAt, 1*time.Millisecond)

					nextRunAt, err = j.NextRun()
					require.NoError(t, err)
					require.Equal(t, time.Time{}, nextRunAt)
				},
			},
		},
		{
			name:      "one run in the past and one in the future",
			atTimes:   []time.Time{n.Add(-1 * time.Millisecond), n.Add(1 * time.Millisecond)},
			fakeClock: clockwork.NewFakeClockAt(n),
			advanceAndAsserts: []func(t *testing.T, j Job, clock clockwork.FakeClock, runs *atomic.Uint32){
				func(t *testing.T, j Job, clock clockwork.FakeClock, runs *atomic.Uint32) {
					require.Equal(t, uint32(0), runs.Load())

					// last not initialized
					lastRunAt, err := j.LastRun()
					require.NoError(t, err)
					require.Equal(t, time.Time{}, lastRunAt)

					// next is now
					nextRunAt, err := j.NextRun()
					require.NoError(t, err)
					require.Equal(t, n.Add(1*time.Millisecond), nextRunAt)

					// advance and eventually run
					clock.Advance(2 * time.Millisecond)
					require.Eventually(t, func() bool {
						return assert.Equal(t, uint32(1), runs.Load())
					}, 3*time.Second, 100*time.Millisecond)

					// last was run
					lastRunAt, err = j.LastRun()
					require.NoError(t, err)
					require.WithinDuration(t, n.Add(1*time.Millisecond), lastRunAt, 1*time.Millisecond)
				},
			},
		},
		{
			name:      "two runs in the future - order is maintained even if times are provided out of order",
			atTimes:   []time.Time{n.Add(3 * time.Millisecond), n.Add(1 * time.Millisecond)},
			fakeClock: clockwork.NewFakeClockAt(n),
			advanceAndAsserts: []func(t *testing.T, j Job, clock clockwork.FakeClock, runs *atomic.Uint32){
				func(t *testing.T, j Job, clock clockwork.FakeClock, runs *atomic.Uint32) {
					require.Equal(t, uint32(0), runs.Load())

					// last not initialized
					lastRunAt, err := j.LastRun()
					require.NoError(t, err)
					require.Equal(t, time.Time{}, lastRunAt)

					// next is now
					nextRunAt, err := j.NextRun()
					require.NoError(t, err)
					require.Equal(t, n.Add(1*time.Millisecond), nextRunAt)

					// advance and eventually run
					clock.Advance(2 * time.Millisecond)
					require.Eventually(t, func() bool {
						return assert.Equal(t, uint32(1), runs.Load())
					}, 3*time.Second, 100*time.Millisecond)

					// last was run
					lastRunAt, err = j.LastRun()
					require.NoError(t, err)
					require.WithinDuration(t, n.Add(1*time.Millisecond), lastRunAt, 1*time.Millisecond)

					nextRunAt, err = j.NextRun()
					require.NoError(t, err)
					require.Equal(t, n.Add(3*time.Millisecond), nextRunAt)
				},

				func(t *testing.T, j Job, clock clockwork.FakeClock, runs *atomic.Uint32) {
					// advance and eventually run
					clock.Advance(2 * time.Millisecond)
					require.Eventually(t, func() bool {
						return assert.Equal(t, uint32(2), runs.Load())
					}, 3*time.Second, 100*time.Millisecond)

					// last was run
					lastRunAt, err := j.LastRun()
					require.NoError(t, err)
					require.WithinDuration(t, n.Add(3*time.Millisecond), lastRunAt, 1*time.Millisecond)

					nextRunAt, err := j.NextRun()
					require.NoError(t, err)
					require.Equal(t, time.Time{}, nextRunAt)
				},
			},
		},

		{
			name:      "two runs in the future - order is maintained even if times are provided out of order - deduplication",
			atTimes:   []time.Time{n.Add(3 * time.Millisecond), n.Add(1 * time.Millisecond), n.Add(1 * time.Millisecond), n.Add(3 * time.Millisecond)},
			fakeClock: clockwork.NewFakeClockAt(n),
			advanceAndAsserts: []func(t *testing.T, j Job, clock clockwork.FakeClock, runs *atomic.Uint32){
				func(t *testing.T, j Job, clock clockwork.FakeClock, runs *atomic.Uint32) {
					require.Equal(t, uint32(0), runs.Load())

					// last not initialized
					lastRunAt, err := j.LastRun()
					require.NoError(t, err)
					require.Equal(t, time.Time{}, lastRunAt)

					// next is now
					nextRunAt, err := j.NextRun()
					require.NoError(t, err)
					require.Equal(t, n.Add(1*time.Millisecond), nextRunAt)

					// advance and eventually run
					clock.Advance(2 * time.Millisecond)
					require.Eventually(t, func() bool {
						return assert.Equal(t, uint32(1), runs.Load())
					}, 3*time.Second, 100*time.Millisecond)

					// last was run
					lastRunAt, err = j.LastRun()
					require.NoError(t, err)
					require.WithinDuration(t, n.Add(1*time.Millisecond), lastRunAt, 1*time.Millisecond)

					nextRunAt, err = j.NextRun()
					require.NoError(t, err)
					require.Equal(t, n.Add(3*time.Millisecond), nextRunAt)
				},

				func(t *testing.T, j Job, clock clockwork.FakeClock, runs *atomic.Uint32) {
					// advance and eventually run
					clock.Advance(2 * time.Millisecond)
					require.Eventually(t, func() bool {
						return assert.Equal(t, uint32(2), runs.Load())
					}, 3*time.Second, 100*time.Millisecond)

					// last was run
					lastRunAt, err := j.LastRun()
					require.NoError(t, err)
					require.WithinDuration(t, n.Add(3*time.Millisecond), lastRunAt, 1*time.Millisecond)

					nextRunAt, err := j.NextRun()
					require.NoError(t, err)
					require.Equal(t, time.Time{}, nextRunAt)
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t, WithClock(tt.fakeClock))
			t.Cleanup(func() {
				require.NoError(t, s.Shutdown())
			})

			runs := atomic.Uint32{}
			j, err := s.NewJob(
				OneTimeJob(OneTimeJobStartDateTimes(tt.atTimes...)),
				NewTask(func() {
					runs.Add(1)
				}),
			)
			if tt.assertErr != nil {
				tt.assertErr(t, err)
			} else {
				require.NoError(t, err)
				s.Start()

				for _, advanceAndAssert := range tt.advanceAndAsserts {
					advanceAndAssert(t, j, tt.fakeClock, &runs)
				}
			}
		})
	}
}

func TestScheduler_WithLimitedRuns(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	tests := []struct {
		name          string
		schedulerOpts []SchedulerOption
		job           JobDefinition
		jobOpts       []JobOption
		runLimit      uint
		expectedRuns  int
	}{
		{
			"simple",
			nil,
			DurationJob(time.Millisecond * 100),
			nil,
			1,
			1,
		},
		{
			"OneTimeJob, WithLimitConcurrentJobs",
			[]SchedulerOption{
				WithLimitConcurrentJobs(1, LimitModeWait),
			},
			OneTimeJob(OneTimeJobStartImmediately()),
			nil,
			1,
			1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t, tt.schedulerOpts...)

			jobRan := make(chan struct{}, 10)

			jobOpts := []JobOption{
				WithLimitedRuns(tt.runLimit),
			}
			jobOpts = append(jobOpts, tt.jobOpts...)

			_, err := s.NewJob(
				tt.job,
				NewTask(func() {
					jobRan <- struct{}{}
				}),
				jobOpts...,
			)
			require.NoError(t, err)

			s.Start()
			time.Sleep(time.Millisecond * 150)

			assert.NoError(t, s.Shutdown())

			var runCount int
			for runCount < tt.expectedRuns {
				select {
				case <-jobRan:
					runCount++
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for job to run")
				}
			}
			select {
			case <-jobRan:
				t.Fatal("job ran more than expected")
			default:
			}
			assert.Equal(t, tt.expectedRuns, runCount)
		})
	}
}

func TestScheduler_Jobs(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	tests := []struct {
		name string
	}{
		{
			"order is equal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t)

			for i := 0; i <= 20; i++ {
				_, err := s.NewJob(
					DurationJob(time.Second),
					NewTask(func() {}),
				)
				require.NoError(t, err)
			}

			jobsFirst := s.Jobs()
			jobsSecond := s.Jobs()

			assert.Equal(t, jobsFirst, jobsSecond)
			assert.NoError(t, s.Shutdown())
		})
	}
}

type testMonitor struct {
	mu      sync.Mutex
	counter map[string]int
	time    map[string][]time.Duration
}

func newTestMonitor() *testMonitor {
	return &testMonitor{
		counter: make(map[string]int),
		time:    make(map[string][]time.Duration),
	}
}

func (t *testMonitor) IncrementJob(_ uuid.UUID, name string, _ []string, _ JobStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.counter[name]
	if !ok {
		t.counter[name] = 0
	}
	t.counter[name]++
}

func (t *testMonitor) RecordJobTiming(startTime, endTime time.Time, _ uuid.UUID, name string, _ []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.time[name]
	if !ok {
		t.time[name] = make([]time.Duration, 0)
	}
	t.time[name] = append(t.time[name], endTime.Sub(startTime))
}

func TestScheduler_WithMonitor(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	tests := []struct {
		name    string
		jd      JobDefinition
		jobName string
	}{
		{
			"scheduler with monitor",
			DurationJob(time.Millisecond * 50),
			"job",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan struct{}, 20)
			monitor := newTestMonitor()
			s := newTestScheduler(t, WithMonitor(monitor))

			opt := []JobOption{
				WithName(tt.jobName),
				WithStartAt(
					WithStartImmediately(),
				),
			}
			_, err := s.NewJob(
				tt.jd,
				NewTask(func() {
					ch <- struct{}{}
				}),
				opt...,
			)
			require.NoError(t, err)
			s.Start()
			time.Sleep(150 * time.Millisecond)
			require.NoError(t, s.Shutdown())
			close(ch)
			expectedCount := 0
			for range ch {
				expectedCount++
			}

			got := monitor.counter[tt.jobName]
			if got != expectedCount {
				t.Fatalf("job %q counter expected %d, got %d", tt.jobName, expectedCount, got)
			}
		})
	}
}
