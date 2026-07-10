package gocron

import (
	"context"
	"errors"
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

func TestScheduler_StopLongRunningJobs(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	t.Run("start, run job, stop jobs before job is completed", func(t *testing.T) {
		s := newTestScheduler(t,
			WithStopTimeout(50*time.Millisecond),
		)

		_, err := s.NewJob(
			DurationJob(
				50*time.Millisecond,
			),
			NewTask(
				func(ctx context.Context) {
					select {
					case <-ctx.Done():
					case <-time.After(100 * time.Millisecond):
						t.Fatal("job can not been canceled")
					}
				},
			),
			WithStartAt(
				WithStartImmediately(),
			),
			WithSingletonMode(LimitModeReschedule),
		)
		require.NoError(t, err)

		s.Start()

		time.Sleep(20 * time.Millisecond)
		// the running job is canceled, no unexpected timeout error
		require.NoError(t, s.StopJobs())
		time.Sleep(100 * time.Millisecond)

		require.NoError(t, s.Shutdown())
	})
	t.Run("start, run job, stop jobs before job is completed - manual context cancel", func(t *testing.T) {
		s := newTestScheduler(t,
			WithStopTimeout(50*time.Millisecond),
		)

		ctx, cancel := context.WithCancel(context.Background())

		_, err := s.NewJob(
			DurationJob(
				50*time.Millisecond,
			),
			NewTask(
				func(ctx context.Context) {
					select {
					case <-ctx.Done():
					case <-time.After(100 * time.Millisecond):
						t.Fatal("job can not been canceled")
					}
				}, ctx,
			),
			WithStartAt(
				WithStartImmediately(),
			),
			WithSingletonMode(LimitModeReschedule),
		)
		require.NoError(t, err)

		s.Start()

		time.Sleep(20 * time.Millisecond)
		// the running job is canceled, no unexpected timeout error
		cancel()
		require.NoError(t, s.StopJobs())
		time.Sleep(100 * time.Millisecond)

		require.NoError(t, s.Shutdown())
	})
	t.Run("start, run job, stop jobs before job is completed - manual context cancel WithContext", func(t *testing.T) {
		s := newTestScheduler(t,
			WithStopTimeout(50*time.Millisecond),
		)

		ctx, cancel := context.WithCancel(context.Background())

		_, err := s.NewJob(
			DurationJob(
				50*time.Millisecond,
			),
			NewTask(
				func(ctx context.Context) {
					select {
					case <-ctx.Done():
					case <-time.After(100 * time.Millisecond):
						t.Fatal("job can not been canceled")
					}
				},
			),
			WithStartAt(
				WithStartImmediately(),
			),
			WithSingletonMode(LimitModeReschedule),
			WithContext(ctx),
		)
		require.NoError(t, err)

		s.Start()

		time.Sleep(20 * time.Millisecond)
		// the running job is canceled, no unexpected timeout error
		cancel()
		require.NoError(t, s.StopJobs())
		time.Sleep(100 * time.Millisecond)

		require.NoError(t, s.Shutdown())
	})
}

func TestScheduler_StopAndStartLongRunningJobs(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	t.Run("start, run job, stop jobs before job is completed", func(t *testing.T) {
		s := newTestScheduler(t,
			WithStopTimeout(50*time.Millisecond),
		)

		_, err := s.NewJob(
			DurationJob(
				50*time.Millisecond,
			),
			NewTask(
				func(ctx context.Context) {
					select {
					case <-ctx.Done():
					case <-time.After(100 * time.Millisecond):
					}
				},
			),
			WithStartAt(
				WithStartImmediately(),
			),
			WithSingletonMode(LimitModeReschedule),
		)
		require.NoError(t, err)

		s.Start()

		time.Sleep(20 * time.Millisecond)
		// the running job is canceled, no unexpected timeout error
		require.NoError(t, s.StopJobs())

		s.Start()

		time.Sleep(200 * time.Millisecond)
		require.NoError(t, s.Shutdown())
	})
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

		// After shutdown, Job accessors can no longer reach the
		// scheduler goroutine, so they surface ErrSchedulerBusy
		// (rather than the historically-misleading ErrJobNotFound;
		// see M4 in CODE_REVIEW.md). Callers who want a single
		// "job not usable" error should combine both with errors.Is.
		_, err = j.LastRun()
		assert.ErrorIs(t, err, ErrSchedulerBusy)

		_, err = j.NextRun()
		assert.ErrorIs(t, err, ErrSchedulerBusy)
	})

	t.Run("calling shutdown multiple times is a no-op", func(t *testing.T) {
		s := newTestScheduler(t)

		s.Start()

		assert.NoError(t, s.Shutdown())
		assert.NoError(t, s.Shutdown())
	})
}

func TestScheduler_ShutdownWithContext(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	t.Run("clean shutdown completes before context deadline", func(t *testing.T) {
		s := newTestScheduler(t, WithStopTimeout(time.Second))

		s.Start()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		require.NoError(t, s.ShutdownWithContext(ctx))
	})

	t.Run("shutdown times out if jobs block longer than context deadline", func(t *testing.T) {
		s := newTestScheduler(t, WithStopTimeout(time.Second))

		testCtx, testCancel := context.WithCancel(context.Background())
		defer testCancel()

		_, err := s.NewJob(
			DurationJob(10*time.Millisecond),
			NewTask(func() {
				<-testCtx.Done() // Block job intentionally
			}),
			WithStartAt(WithStartImmediately()),
		)
		require.NoError(t, err)

		s.Start()
		time.Sleep(50 * time.Millisecond) // Let job start

		// We give 50ms for shutdown context, this should timeout because the job is blocking until testCtx is done
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err = s.ShutdownWithContext(ctx)
		assert.ErrorIs(t, err, context.DeadlineExceeded)

		testCancel() // Unblock job so test can clean up gracefully
		time.Sleep(50 * time.Millisecond)
	})
}

func TestScheduler_StopJobsWithContext(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	t.Run("clean stop completes before context deadline", func(t *testing.T) {
		s := newTestScheduler(t, WithStopTimeout(time.Second))

		s.Start()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		require.NoError(t, s.StopJobsWithContext(ctx))
		require.NoError(t, s.Shutdown()) // clean up
	})
}

func TestScheduler_Start(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	t.Run("calling start multiple times is a no-op", func(t *testing.T) {
		s := newTestScheduler(t)

		var counter int
		var mu sync.Mutex

		_, err := s.NewJob(
			DurationJob(
				100*time.Millisecond,
			),
			NewTask(
				func() {
					mu.Lock()
					counter++
					mu.Unlock()
				},
			),
		)
		require.NoError(t, err)

		s.Start()
		s.Start()
		s.Start()

		time.Sleep(1000 * time.Millisecond)

		require.NoError(t, s.Shutdown())

		assert.Contains(t, []int{9, 10}, counter)
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
			"context nil",
			DurationJob(time.Second),
			[]JobOption{WithContext(nil)}, //nolint:staticcheck
			ErrWithContextNil,
		},
		{
			"duration job time interval is zero",
			DurationJob(0 * time.Second),
			nil,
			ErrDurationJobIntervalZero,
		},
		{
			"duration job time interval is negative",
			DurationJob(-1 * time.Second),
			nil,
			ErrDurationJobIntervalNegative,
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
			"random with negative min",
			DurationRandomJob(
				-time.Second,
				time.Second,
			),
			nil,
			ErrDurationRandomJobPositive,
		},
		{
			"random with negative max",
			DurationRandomJob(
				-2*time.Second,
				-time.Second,
			),
			nil,
			ErrDurationRandomJobPositive,
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
			"WithStartDateTimePast is zero",
			DurationJob(
				time.Second,
			),
			[]JobOption{WithStartAt(WithStartDateTimePast(time.Time{}))},
			ErrWithStartDateTimePastZero,
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
		{
			"WithLimitedRuns is zero",
			DurationJob(
				time.Second,
			),
			[]JobOption{WithLimitedRuns(0)},
			ErrWithLimitedRunsZero,
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
			NewTask(func(_ any) {}, struct{}{}),
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
			NewTask(func(_ ...any) {}, struct{}{}),
			nil,
		},
		{
			"all good no arguments passed in - variadic",
			NewTask(func(_ ...any) {}),
			nil,
		},
		{
			"all good - interface variadic, int, string",
			NewTask(func(_ ...any) {}, 1, "2", 3.0),
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
		fakeClock *clockwork.FakeClock
		assertErr require.ErrorAssertionFunc
		// asserts things about schedules, advance time and perform new assertions
		advanceAndAsserts []func(
			t *testing.T,
			j Job,
			clock *clockwork.FakeClock,
			runs *atomic.Uint32,
		)
	}{
		{
			name:      "exhausted run do does not cleanup next item",
			runAt:     time.Date(2024, time.April, 22, 4, 5, 0, 0, time.UTC),
			fakeClock: clockwork.NewFakeClockAt(schedulerStartTime),
			advanceAndAsserts: []func(t *testing.T, j Job, clock *clockwork.FakeClock, runs *atomic.Uint32){
				func(t *testing.T, j Job, clock *clockwork.FakeClock, runs *atomic.Uint32) {
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
						return uint32(1) == runs.Load()
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
		return errors.New("done")
	default:
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.leaderElected {
		t.notLeader <- struct{}{}
		return errors.New("already elected leader")
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
		return nil, errors.New("job already locked")
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
		runCount      int
		schedulerOpts []SchedulerOption
		jobOpts       []JobOption
		assertions    func(*testing.T)
	}{
		{
			"3 schedulers with elector",
			3,
			1,
			[]SchedulerOption{
				WithDistributedElector(&testElector{notLeader: notLeader}),
			},
			nil,
			func(t *testing.T) {
				timeout := time.Now().Add(1 * time.Second)
				var notLeaderCount int
				for !time.Now().After(timeout) {
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
			1,
			[]SchedulerOption{
				WithDistributedLocker(&testLocker{notLocked: notLocked}),
			},
			nil,
			func(_ *testing.T) {
				timeout := time.Now().Add(1 * time.Second)
				var notLockedCount int
				for !time.Now().After(timeout) {
					select {
					case <-notLocked:
						notLockedCount++
					default:
					}
				}

				assert.Equal(t, 2, notLockedCount)
			},
		},
		{
			"3 schedulers and job with Distributed locker",
			3,
			1,
			nil,
			[]JobOption{
				WithDistributedJobLocker(&testLocker{notLocked: notLocked}),
			},
			func(_ *testing.T) {
				timeout := time.Now().Add(1 * time.Second)
				var notLockedCount int
				for !time.Now().After(timeout) {
					select {
					case <-notLocked:
						notLockedCount++
					default:
					}
				}

				assert.Equal(t, 2, notLockedCount)
			},
		},
		{
			"3 schedulers and job with disabled Distributed locker",
			3,
			3,
			[]SchedulerOption{
				WithDistributedLocker(&testLocker{notLocked: notLocked}),
			},
			[]JobOption{
				WithDisabledDistributedJobLocker(true),
			},
			func(_ *testing.T) {
				timeout := time.Now().Add(1 * time.Second)
				var notLockedCount int
				for !time.Now().After(timeout) {
					select {
					case <-notLocked:
						notLockedCount++
					default:
					}
				}

				assert.Equal(t, 0, notLockedCount)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobsRan := make(chan struct{}, 20)
			ctx, cancel := context.WithCancel(context.Background())
			schedulersDone := make(chan struct{}, tt.count)

			var (
				runCount  int
				doneCount int
			)

			for i := tt.count; i > 0; i-- {
				s := newTestScheduler(t,
					tt.schedulerOpts...,
				)
				jobOpts := []JobOption{
					WithStartAt(
						WithStartImmediately(),
					),
					WithLimitedRuns(1),
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

		RunCountLoop:
			for {
				select {
				case <-jobsRan:
					runCount++
					if runCount >= tt.runCount {
						break RunCountLoop
					}
				case <-time.After(time.Second):
					t.Error("timed out waiting for job to run")
					break RunCountLoop
				}
			}

			cancel()
			assert.Equal(t, tt.runCount, runCount)

		DoneCountLoop:
			for {
				select {
				case <-schedulersDone:
					doneCount++
					if doneCount >= tt.count {
						break DoneCountLoop
					}
				case <-time.After(3 * time.Second):
					t.Error("timed out waiting for schedulers to shutdown")
					break DoneCountLoop
				}
			}

			assert.Equal(t, tt.count, doneCount)

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
			assert.ErrorIs(t, err, tt.err)
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

	require.Eventually(t, func() bool {
		return len(s.Jobs()) == 0
	}, time.Second, 10*time.Millisecond)
	assert.NoError(t, s.Shutdown())
}

func TestScheduler_WithEventListeners(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	listenerRunCh := make(chan error, 1)
	testErr := errors.New("test error")
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
		fakeClock *clockwork.FakeClock
		assertErr require.ErrorAssertionFunc
		// asserts things about schedules, advance time and perform new assertions
		advanceAndAsserts []func(
			t *testing.T,
			j Job,
			clock *clockwork.FakeClock,
			runs *atomic.Uint32,
		)
	}{
		{
			name:      "no at times",
			atTimes:   []time.Time{},
			fakeClock: clockwork.NewFakeClock(),
			assertErr: func(t require.TestingT, err error, _ ...any) {
				require.ErrorIs(t, err, ErrOneTimeJobStartDateTimePast)
			},
		},
		{
			name:      "all in the past",
			atTimes:   []time.Time{n.Add(-1 * time.Second)},
			fakeClock: clockwork.NewFakeClockAt(n),
			assertErr: func(t require.TestingT, err error, _ ...any) {
				require.ErrorIs(t, err, ErrOneTimeJobStartDateTimePast)
			},
		},
		{
			name:      "one run 1 millisecond in the future",
			atTimes:   []time.Time{n.Add(1 * time.Millisecond)},
			fakeClock: clockwork.NewFakeClockAt(n),
			advanceAndAsserts: []func(t *testing.T, j Job, clock *clockwork.FakeClock, runs *atomic.Uint32){
				func(t *testing.T, j Job, clock *clockwork.FakeClock, runs *atomic.Uint32) {
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
						return uint32(1) == runs.Load()
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
			advanceAndAsserts: []func(t *testing.T, j Job, clock *clockwork.FakeClock, runs *atomic.Uint32){
				func(t *testing.T, j Job, clock *clockwork.FakeClock, runs *atomic.Uint32) {
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
						return uint32(1) == runs.Load()
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
			advanceAndAsserts: []func(t *testing.T, j Job, clock *clockwork.FakeClock, runs *atomic.Uint32){
				func(t *testing.T, j Job, clock *clockwork.FakeClock, runs *atomic.Uint32) {
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
						return uint32(1) == runs.Load()
					}, 3*time.Second, 100*time.Millisecond)

					// last was run
					lastRunAt, err = j.LastRun()
					require.NoError(t, err)
					require.WithinDuration(t, n.Add(1*time.Millisecond), lastRunAt, 1*time.Millisecond)

					nextRunAt, err = j.NextRun()
					require.NoError(t, err)
					require.Equal(t, n.Add(3*time.Millisecond), nextRunAt)
				},

				func(t *testing.T, j Job, clock *clockwork.FakeClock, runs *atomic.Uint32) {
					// advance and eventually run
					clock.Advance(2 * time.Millisecond)
					require.Eventually(t, func() bool {
						return uint32(2) == runs.Load()
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
			advanceAndAsserts: []func(t *testing.T, j Job, clock *clockwork.FakeClock, runs *atomic.Uint32){
				func(t *testing.T, j Job, clock *clockwork.FakeClock, runs *atomic.Uint32) {
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
						return uint32(1) == runs.Load()
					}, 3*time.Second, 100*time.Millisecond)

					// last was run
					lastRunAt, err = j.LastRun()
					require.NoError(t, err)
					require.WithinDuration(t, n.Add(1*time.Millisecond), lastRunAt, 1*time.Millisecond)

					nextRunAt, err = j.NextRun()
					require.NoError(t, err)
					require.Equal(t, n.Add(3*time.Millisecond), nextRunAt)
				},

				func(t *testing.T, j Job, clock *clockwork.FakeClock, runs *atomic.Uint32) {
					// advance and eventually run
					clock.Advance(2 * time.Millisecond)
					require.Eventually(t, func() bool {
						return uint32(2) == runs.Load()
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

func TestScheduler_WithStartAtDateTimePast(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	// Monday
	testTime := time.Date(2024, time.January, 1, 9, 0, 0, 0, time.UTC)

	fakeClock := clockwork.NewFakeClockAt(testTime)

	s := newTestScheduler(t, WithClock(fakeClock))
	j, err := s.NewJob(
		WeeklyJob(2, NewWeekdays(time.Sunday), NewAtTimes(NewAtTime(10, 0, 0))),
		NewTask(func() {}),
		WithStartAt(
			// The start time is in the past (Dec 30, 2023 9am) which is a Saturday
			WithStartDateTimePast(testTime.Add(-time.Hour*24*2)),
		),
	)
	require.NoError(t, err)

	s.Start()

	nextRun, err := j.NextRun()
	require.NoError(t, err)

	require.NoError(t, s.Shutdown())

	// Because the start time was in the past - we expect it to schedule 2 intervals ahead, pasing the first available Sunday
	// which was in the past Dec 31, 2023, so the next is Jan 7, 2024
	assert.Equal(t, time.Date(2024, time.January, 7, 10, 0, 0, 0, time.UTC), nextRun)
}

func BenchmarkSchedulerJobs(b *testing.B) {
	cases := []struct {
		name string
		n    int
	}{
		{"10", 10},
		{"100", 100},
		{"500", 500},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			s, err := NewScheduler(WithLogger(NewLogger(LogLevelError)))
			if err != nil {
				b.Fatal(err)
			}
			for i := 0; i < tc.n; i++ {
				_, err := s.NewJob(DurationJob(time.Hour), NewTask(func() {}))
				if err != nil {
					b.Fatal(err)
				}
			}
			s.Start()
			b.Cleanup(func() { _ = s.Shutdown() })
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = s.Jobs()
			}
		})
	}
}

func TestScheduler_JobSchedule(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	tests := []struct {
		name       string
		jd         JobDefinition
		assertFunc func(t *testing.T, schedule JobSchedule)
	}{
		{
			"cron",
			CronJob(
				"*/5 * * * *",
				false,
			),
			func(t *testing.T, schedule JobSchedule) {
				require.NotNil(t, schedule)
				assert.Equal(t, CronJobType, schedule.JobType())
				cronSchedule, ok := schedule.(CronJobSchedule)
				require.True(t, ok)
				assert.Equal(t, "*/5 * * * *", cronSchedule.Crontab)
			},
		},
		{
			"cron with seconds",
			CronJob(
				"*/5 * * * * *",
				true,
			),
			func(t *testing.T, schedule JobSchedule) {
				require.NotNil(t, schedule)
				assert.Equal(t, CronJobType, schedule.JobType())
				cronSchedule, ok := schedule.(CronJobSchedule)
				require.True(t, ok)
				assert.Equal(t, "*/5 * * * * *", cronSchedule.Crontab)
			},
		},
		{
			"duration",
			DurationJob(
				5 * time.Second,
			),
			func(t *testing.T, schedule JobSchedule) {
				require.NotNil(t, schedule)
				assert.Equal(t, DurationJobType, schedule.JobType())
				durationSchedule, ok := schedule.(DurationJobSchedule)
				require.True(t, ok)
				assert.Equal(t, 5*time.Second, durationSchedule.Duration)
			},
		},
		{
			"duration random",
			DurationRandomJob(
				time.Second,
				5*time.Second,
			),
			func(t *testing.T, schedule JobSchedule) {
				require.NotNil(t, schedule)
				assert.Equal(t, DurationRandomJobType, schedule.JobType())
				durationRandomSchedule, ok := schedule.(DurationRandomJobSchedule)
				require.True(t, ok)
				assert.Equal(t, time.Second, durationRandomSchedule.Min)
				assert.Equal(t, 5*time.Second, durationRandomSchedule.Max)
			},
		},
		{
			"daily",
			DailyJob(
				2,
				NewAtTimes(
					NewAtTime(1, 30, 0),
					NewAtTime(12, 0, 0),
				),
			),
			func(t *testing.T, schedule JobSchedule) {
				require.NotNil(t, schedule)
				assert.Equal(t, DailyJobType, schedule.JobType())
				dailySchedule, ok := schedule.(DailyJobSchedule)
				require.True(t, ok)
				assert.Equal(t, uint(2), dailySchedule.Interval)
				assert.Len(t, dailySchedule.AtTimes, 2)
			},
		},
		{
			"weekly",
			WeeklyJob(
				1,
				NewWeekdays(time.Monday, time.Wednesday, time.Friday),
				NewAtTimes(
					NewAtTime(9, 0, 0),
				),
			),
			func(t *testing.T, schedule JobSchedule) {
				require.NotNil(t, schedule)
				assert.Equal(t, WeeklyJobType, schedule.JobType())
				weeklySchedule, ok := schedule.(WeeklyJobSchedule)
				require.True(t, ok)
				assert.Equal(t, uint(1), weeklySchedule.Interval)
				assert.Len(t, weeklySchedule.DaysOfWeek, 3)
				assert.Contains(t, weeklySchedule.DaysOfWeek, time.Monday)
				assert.Contains(t, weeklySchedule.DaysOfWeek, time.Wednesday)
				assert.Contains(t, weeklySchedule.DaysOfWeek, time.Friday)
				assert.Len(t, weeklySchedule.AtTimes, 1)
			},
		},
		{
			"monthly",
			MonthlyJob(
				1,
				NewDaysOfTheMonth(1, 15, -1),
				NewAtTimes(
					NewAtTime(8, 0, 0),
				),
			),
			func(t *testing.T, schedule JobSchedule) {
				require.NotNil(t, schedule)
				assert.Equal(t, MonthlyJobType, schedule.JobType())
				monthlySchedule, ok := schedule.(MonthlyJobSchedule)
				require.True(t, ok)
				assert.Equal(t, uint(1), monthlySchedule.Interval)
				assert.Contains(t, monthlySchedule.Days, 1)
				assert.Contains(t, monthlySchedule.Days, 15)
				assert.Contains(t, monthlySchedule.DaysFromEnd, -1)
				assert.Len(t, monthlySchedule.AtTimes, 1)
			},
		},
		{
			"one time",
			OneTimeJob(
				OneTimeJobStartDateTime(time.Now().Add(time.Hour)),
			),
			func(t *testing.T, schedule JobSchedule) {
				require.NotNil(t, schedule)
				assert.Equal(t, OneTimeJobType, schedule.JobType())
				oneTimeSchedule, ok := schedule.(OneTimeJobSchedule)
				require.True(t, ok)
				assert.Len(t, oneTimeSchedule.StartAt, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t)

			j, err := s.NewJob(tt.jd, NewTask(func() {}))
			require.NoError(t, err)

			tt.assertFunc(t, j.Schedule())

			require.NoError(t, s.Shutdown())
		})
	}
}

func TestScheduler_WithStopDateTime_JobRemovedAfterStopTime(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	t.Run("job is removed from scheduler after stop time elapses", func(t *testing.T) {
		monitor := newTestSchedulerMonitor()
		s := newTestScheduler(t, WithSchedulerMonitor(monitor))

		_, err := s.NewJob(
			DurationJob(50*time.Millisecond),
			NewTask(func() {}),
			WithStopAt(WithStopDateTime(time.Now().Add(200*time.Millisecond))),
			WithStartAt(WithStartImmediately()),
		)
		require.NoError(t, err)

		s.Start()

		require.Eventually(t, func() bool {
			return len(s.Jobs()) == 0
		}, time.Second, 10*time.Millisecond, "job should be removed after stop time")

		assert.GreaterOrEqual(t, monitor.getJobUnregCount(), int64(1), "monitor should receive JobUnregistered notification")

		require.NoError(t, s.Shutdown())
	})

	t.Run("job added before start is removed on start when stop time already elapsed", func(t *testing.T) {
		monitor := newTestSchedulerMonitor()
		s := newTestScheduler(t, WithSchedulerMonitor(monitor))

		_, err := s.NewJob(
			DurationJob(time.Hour),
			NewTask(func() {}),
			WithStopAt(WithStopDateTime(time.Now().Add(100*time.Millisecond))),
		)
		require.NoError(t, err)

		// wait until stop time has passed, then start the scheduler
		time.Sleep(150 * time.Millisecond)
		s.Start()

		require.Eventually(t, func() bool {
			return len(s.Jobs()) == 0
		}, time.Second, 10*time.Millisecond, "job should be removed when scheduler starts after stop time")

		require.NoError(t, s.Shutdown())
	})

	t.Run("RemoveJob on already auto-removed job returns ErrJobNotFound", func(t *testing.T) {
		s := newTestScheduler(t)

		j, err := s.NewJob(
			DurationJob(50*time.Millisecond),
			NewTask(func() {}),
			WithStopAt(WithStopDateTime(time.Now().Add(150*time.Millisecond))),
			WithStartAt(WithStartImmediately()),
		)
		require.NoError(t, err)

		s.Start()

		require.Eventually(t, func() bool {
			return len(s.Jobs()) == 0
		}, time.Second, 10*time.Millisecond, "job should be auto-removed after stop time")

		// Explicitly removing an already auto-removed job should return ErrJobNotFound
		err = s.RemoveJob(j.ID())
		assert.ErrorIs(t, err, ErrJobNotFound)

		require.NoError(t, s.Shutdown())
	})
}

// TestScheduler_WithLimitedRuns_ContextNotCanceledDuringTask asserts that the
// context passed to a task running under WithLimitedRuns is not canceled
// while the task function is still executing. Regression test for #925.
func TestScheduler_WithLimitedRuns_ContextNotCanceledDuringTask(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	s := newTestScheduler(t)

	ctxErrCh := make(chan error, 1)
	_, err := s.NewJob(
		DurationJob(time.Hour),
		NewTask(func(ctx context.Context) {
			// Hold the task open briefly so the scheduler has time to
			// process the after-rescheduling signal and (incorrectly)
			// cancel the context before this function returns.
			time.Sleep(100 * time.Millisecond)
			ctxErrCh <- ctx.Err()
		}),
		WithStartAt(WithStartImmediately()),
		WithLimitedRuns(1),
	)
	require.NoError(t, err)

	s.Start()

	select {
	case ctxErr := <-ctxErrCh:
		require.NoError(t, ctxErr, "task context must not be canceled while task is still running")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task to run")
	}

	require.NoError(t, s.Shutdown())
}

// zeroNextCron is a pathological Cron implementation whose Next always
// returns the zero time. Used to verify the scheduler does not hang when
// a custom Cron implementation fails to produce a forward-progressing time.
// Regression test for the infinite loop in selectExecJobsOutForRescheduling /
// selectNewJob / selectStart documented as C2 in CODE_REVIEW.md.
type zeroNextCron struct{}

func (zeroNextCron) IsValid(_ string, _ *time.Location, _ time.Time) error {
	return nil
}

func (zeroNextCron) Next(_ time.Time) time.Time {
	return time.Time{}
}

func TestScheduler_CronWithZeroNext_DoesNotHang(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	s := newTestScheduler(t)

	// Use a fail-safe context timeout so a regression hangs the test
	// rather than the entire test binary.
	done := make(chan struct{})
	go func() {
		defer close(done)

		_, err := s.NewJob(
			CronJob("* * * * *", false),
			NewTask(func() {}),
			WithCronImplementation(zeroNextCron{}),
		)
		require.NoError(t, err)

		s.Start()

		// Give the scheduler a moment to attempt rescheduling.
		time.Sleep(100 * time.Millisecond)

		// Jobs() must respond promptly even though the cron impl
		// produces a zero-time next run. The job should have been
		// removed because no forward-progressing time can be found.
		jobs := s.Jobs()
		require.Empty(t, jobs, "job with zero-next cron should be removed, not retained in a spin loop")

		require.NoError(t, s.Shutdown())
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("scheduler hung when custom Cron.Next returned zero time")
	}
}

// TestScheduler_WithGlobalJobOptions_MultipleCallsAppend asserts that
// passing WithGlobalJobOptions multiple times to NewScheduler results
// in all option lists being applied to each job (in order), rather
// than the last call silently overwriting earlier calls. See H6 in
// CODE_REVIEW.md.
func TestScheduler_WithGlobalJobOptions_MultipleCallsAppend(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	s := newTestScheduler(t,
		WithGlobalJobOptions(WithTags("shared-tag")),
		WithGlobalJobOptions(WithName("shared-name")),
	)

	j, err := s.NewJob(
		DurationJob(time.Hour),
		NewTask(func() {}),
	)
	require.NoError(t, err)

	// Both option lists must have applied. If WithGlobalJobOptions
	// had overwritten instead of appended, only the "shared-name"
	// option (from the second call) would be present and Tags()
	// would be empty.
	require.Equal(t, []string{"shared-tag"}, j.Tags())
	require.Equal(t, "shared-name", j.Name())

	require.NoError(t, s.Shutdown())
}

// TestScheduler_WithLimitedRuns_SkippedRunsDoNotConsumeBudget asserts
// that runs aborted before the task function executes (for example,
// by BeforeJobRunsSkipIfBeforeFuncErrors returning an error) do not
// count against the WithLimitedRuns budget. See C3 in CODE_REVIEW.md.
func TestScheduler_WithLimitedRuns_SkippedRunsDoNotConsumeBudget(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	s := newTestScheduler(t)

	var skipCalls atomic.Int32
	var runs atomic.Int32
	const forcedSkips = 3

	_, err := s.NewJob(
		DurationJob(20*time.Millisecond),
		NewTask(func() { runs.Add(1) }),
		WithStartAt(WithStartImmediately()),
		WithLimitedRuns(2),
		WithEventListeners(
			BeforeJobRunsSkipIfBeforeFuncErrors(func(_ uuid.UUID, _ string) error {
				// Force the first `forcedSkips` invocations to skip;
				// afterward let the task run.
				if skipCalls.Add(1) <= forcedSkips {
					return errors.New("forced skip")
				}
				return nil
			}),
		),
	)
	require.NoError(t, err)

	s.Start()
	// Wait long enough for forcedSkips skips + 2 real runs at 20ms
	// intervals to play out with margin for CI noise.
	time.Sleep(400 * time.Millisecond)
	require.NoError(t, s.Shutdown())

	require.GreaterOrEqual(t, skipCalls.Load(), int32(forcedSkips),
		"beforeFunc should have fired at least forcedSkips times")
	require.Equal(t, int32(2), runs.Load(),
		"task should have executed exactly WithLimitedRuns(2) times, "+
			"regardless of how many prior invocations were skipped")
}

// TestScheduler_NextRuns_ReturnsAscendingAfterRescheduleCycles asserts
// that after many reschedule cycles, Job.NextRuns returns strictly
// ascending times. This locks in the internalJob.nextScheduled
// sort invariant that NextRun/NextRuns rely on (job.go:1614/1627/1633).
// See H4 in CODE_REVIEW.md.
func TestScheduler_NextRuns_ReturnsAscendingAfterRescheduleCycles(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	s := newTestScheduler(t)
	j, err := s.NewJob(
		DurationJob(15*time.Millisecond),
		NewTask(func() {}),
		WithStartAt(WithStartImmediately()),
	)
	require.NoError(t, err)

	s.Start()

	// Poll NextRuns while the scheduler cycles the job.
	deadline := time.Now().Add(300 * time.Millisecond)
	checks := 0
	for time.Now().Before(deadline) {
		runs, err := j.NextRuns(5)
		require.NoError(t, err)
		for i := 1; i < len(runs); i++ {
			require.False(t, runs[i].Before(runs[i-1]),
				"NextRuns must return strictly ascending times: "+
					"idx %d (%v) < idx %d (%v); full result: %v",
				i, runs[i], i-1, runs[i-1], runs)
		}
		checks++
		time.Sleep(5 * time.Millisecond)
	}
	require.Greater(t, checks, 10, "sanity: expected many polling iterations")

	require.NoError(t, s.Shutdown())
}

// TestScheduler_CronJob_DefinitionReuseDoesNotAliasCronImpl asserts
// that reusing a JobDefinition across multiple NewJob calls yields
// jobs with independent Cron implementations. Previously, all jobs
// derived from the same definition shared the same *defaultCron
// pointer, which was mutated by IsValid during setup — creating a
// latent data race when Update ran concurrently with Job.NextRuns.
func TestScheduler_CronJob_DefinitionReuseDoesNotAliasCronImpl(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	def := CronJob("*/5 * * * *", false)
	s := newTestScheduler(t)

	j1, err := s.NewJob(def, NewTask(func() {}))
	require.NoError(t, err)
	j2, err := s.NewJob(def, NewTask(func() {}))
	require.NoError(t, err)

	// Reach into the scheduler's job map (test-only access via the
	// concrete type) to confirm that each cronJob holds its own
	// Cron instance rather than aliasing the definition's inner
	// pointer.
	sched := s.(*scheduler)
	ij1 := sched.jobs[j1.ID()]
	ij2 := sched.jobs[j2.ID()]
	cj1, ok := ij1.jobSchedule.(*cronJob)
	require.True(t, ok, "expected *cronJob jobSchedule for j1")
	cj2, ok := ij2.jobSchedule.(*cronJob)
	require.True(t, ok, "expected *cronJob jobSchedule for j2")
	require.NotSame(t, cj1.cronSchedule, cj2.cronSchedule,
		"each job derived from the same JobDefinition must hold its own Cron instance")

	require.NoError(t, s.Shutdown())
}

// TestScheduler_DurationRandomJob_NextRunsIsRaceFree hammers
// Job.NextRuns from many goroutines while the scheduler is running,
// which previously tripped `-race` because durationRandomJob used a
// non-concurrent *rand.Rand shared between the scheduler goroutine
// and user callers of NextRuns.
func TestScheduler_DurationRandomJob_NextRunsIsRaceFree(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	s := newTestScheduler(t)
	j, err := s.NewJob(
		DurationRandomJob(10*time.Millisecond, 20*time.Millisecond),
		NewTask(func() {}),
	)
	require.NoError(t, err)
	s.Start()

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				runs, err := j.NextRuns(5)
				require.NoError(t, err)
				require.NotNil(t, runs)
			}
		}()
	}
	wg.Wait()
	require.NoError(t, s.Shutdown())
}

// TestScheduler_OneTimeJob_PastStartTime_DoesNotSpin is a regression test
// for issue #943 (100% CPU spin under LimitModeWait + OneTimeJobStartDateTime).
//
// The v2.21.2 spin worked as follows: selectStart (and selectNewJob) had
//
//	if next.Before(s.now()) {
//	    for next.Before(s.now()) {
//	        next = j.next(next)
//	    }
//	}
//
// For a oneTimeJob whose sortedTimes has been exhausted, next() returns
// time.Time{} — which is also Before(s.now()). The subsequent call
// next(time.Time{}) binary-searches to idx=0 and returns sortedTimes[0]
// (the original past time). The loop oscillates between the past time
// and the zero time forever, pegging one CPU core inside time.Now().
// The reporter's pprof profile showed 94% of CPU accounted for by
// exactly these two lines (see issue #943 attachment).
//
// The fix (PR #930) is advancePastNow, which detects both the zero-time
// return and any non-monotonic step and removes the job cleanly.
//
// To force the exhausted-schedule condition deterministically we use a
// fake clock: register the job with a startAt one minute in the future
// (so setup validation passes), then advance the clock past startAt
// before starting the scheduler. selectStart then observes
// next.Before(s.now()) and would spin on v2.21.2. With the fix, the job
// is removed and Start() returns promptly.
func TestScheduler_OneTimeJob_PastStartTime_DoesNotSpin(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	schedulerStart := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(schedulerStart)

	s := newTestScheduler(t, WithClock(fakeClock))

	// One minute in the future relative to the fake clock — passes
	// oneTimeJobDefinition.setup's !at.After(now) validation.
	startAt := schedulerStart.Add(time.Minute)
	j, err := s.NewJob(
		OneTimeJob(OneTimeJobStartDateTime(startAt)),
		NewTask(func() {}),
	)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, j.ID())

	// Advance past startAt so selectStart sees a job whose only
	// schedule time is in the past.
	fakeClock.Advance(5 * time.Minute)

	// Start() blocks on <-s.startedCh until the scheduler goroutine
	// signals ready after selectStart completes. If selectStart spins
	// (v2.21.2 behavior), this signal never arrives and Start() hangs.
	done := make(chan struct{})
	go func() {
		s.Start()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler.Start() did not return within 2s — regression: selectStart is spinning on a past OneTimeJob (issue #943)")
	}

	// Past OneTimeJob should be silently removed at Start(), not retained
	// in a poison state.
	require.Empty(t, s.Jobs(), "exhausted OneTimeJob should be removed by selectStart, not retained")

	require.NoError(t, s.Shutdown())
}

// TestScheduler_WithStartAtGrace covers the WithStartAtGrace JobOption,
// which lets callers opt into a bounded tolerance for late first-run
// dispatch. Without grace, a job whose scheduled start time has already
// elapsed by dispatch is silently removed (strict semantics, preserved
// as the default). With grace, if the elapsed drift is within the
// configured window the run fires immediately; if it exceeds the window
// the strict behavior kicks in.
//
// See WithStartAtGrace docstring and issue #943 for the workload that
// motivated this option.
func TestScheduler_WithStartAtGrace(t *testing.T) {
	t.Run("within grace fires the missed one-time run", func(t *testing.T) {
		defer verifyNoGoroutineLeaks(t)

		schedulerStart := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC)
		fakeClock := clockwork.NewFakeClockAt(schedulerStart)

		s := newTestScheduler(t, WithClock(fakeClock))

		var runs atomic.Uint32
		done := make(chan struct{}, 1)
		startAt := schedulerStart.Add(time.Minute)
		_, err := s.NewJob(
			OneTimeJob(OneTimeJobStartDateTime(startAt)),
			NewTask(func() {
				runs.Add(1)
				select {
				case done <- struct{}{}:
				default:
				}
			}),
			WithStartAtGrace(10*time.Minute),
		)
		require.NoError(t, err)

		// Advance past startAt but still within the 10-minute grace window.
		fakeClock.Advance(5 * time.Minute)
		s.Start()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("expected grace-triggered fire within 2s")
		}
		require.Equal(t, uint32(1), runs.Load())

		require.NoError(t, s.Shutdown())
	})

	t.Run("outside grace drops the job", func(t *testing.T) {
		defer verifyNoGoroutineLeaks(t)

		schedulerStart := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC)
		fakeClock := clockwork.NewFakeClockAt(schedulerStart)

		s := newTestScheduler(t, WithClock(fakeClock))

		var runs atomic.Uint32
		startAt := schedulerStart.Add(time.Minute)
		_, err := s.NewJob(
			OneTimeJob(OneTimeJobStartDateTime(startAt)),
			NewTask(func() { runs.Add(1) }),
			WithStartAtGrace(2*time.Minute),
		)
		require.NoError(t, err)

		// Advance past startAt by more than the grace window (10m > 2m).
		fakeClock.Advance(10 * time.Minute)
		s.Start()

		// Give the scheduler time to process the queue and remove the job.
		time.Sleep(100 * time.Millisecond)
		require.Empty(t, s.Jobs(), "job past its grace window should be removed")
		require.Equal(t, uint32(0), runs.Load(), "no run should happen for a dropped job")

		require.NoError(t, s.Shutdown())
	})

	t.Run("default (no option) preserves strict drop behavior", func(t *testing.T) {
		defer verifyNoGoroutineLeaks(t)

		schedulerStart := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC)
		fakeClock := clockwork.NewFakeClockAt(schedulerStart)

		s := newTestScheduler(t, WithClock(fakeClock))

		var runs atomic.Uint32
		startAt := schedulerStart.Add(time.Minute)
		_, err := s.NewJob(
			OneTimeJob(OneTimeJobStartDateTime(startAt)),
			NewTask(func() { runs.Add(1) }),
			// No WithStartAtGrace — must behave identically to pre-feature.
		)
		require.NoError(t, err)

		fakeClock.Advance(5 * time.Minute)
		s.Start()

		time.Sleep(100 * time.Millisecond)
		require.Empty(t, s.Jobs(), "default (grace=0) must drop past OneTimeJob")
		require.Equal(t, uint32(0), runs.Load())

		require.NoError(t, s.Shutdown())
	})

	t.Run("grace of zero is equivalent to no option", func(t *testing.T) {
		defer verifyNoGoroutineLeaks(t)

		schedulerStart := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC)
		fakeClock := clockwork.NewFakeClockAt(schedulerStart)

		s := newTestScheduler(t, WithClock(fakeClock))

		var runs atomic.Uint32
		startAt := schedulerStart.Add(time.Minute)
		_, err := s.NewJob(
			OneTimeJob(OneTimeJobStartDateTime(startAt)),
			NewTask(func() { runs.Add(1) }),
			WithStartAtGrace(0),
		)
		require.NoError(t, err)

		fakeClock.Advance(5 * time.Minute)
		s.Start()

		time.Sleep(100 * time.Millisecond)
		require.Empty(t, s.Jobs())
		require.Equal(t, uint32(0), runs.Load())

		require.NoError(t, s.Shutdown())
	})

	t.Run("negative grace returns an error at NewJob", func(t *testing.T) {
		defer verifyNoGoroutineLeaks(t)

		s := newTestScheduler(t)
		_, err := s.NewJob(
			OneTimeJob(OneTimeJobStartImmediately()),
			NewTask(func() {}),
			WithStartAtGrace(-1*time.Second),
		)
		require.ErrorIs(t, err, ErrWithStartAtGraceNegative)

		require.NoError(t, s.Shutdown())
	})

	t.Run("grace-triggered fire respects stopTime", func(t *testing.T) {
		defer verifyNoGoroutineLeaks(t)

		schedulerStart := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC)
		fakeClock := clockwork.NewFakeClockAt(schedulerStart)

		s := newTestScheduler(t, WithClock(fakeClock))

		var runs atomic.Uint32
		startAt := schedulerStart.Add(time.Minute)
		stopAt := schedulerStart.Add(2 * time.Minute)
		_, err := s.NewJob(
			OneTimeJob(OneTimeJobStartDateTime(startAt)),
			NewTask(func() { runs.Add(1) }),
			WithStartAtGrace(1*time.Hour),
			WithStopAt(WithStopDateTime(stopAt)),
		)
		require.NoError(t, err)

		// Advance past BOTH startAt and stopAt — within grace but past stop.
		fakeClock.Advance(10 * time.Minute)
		s.Start()

		time.Sleep(100 * time.Millisecond)
		require.Empty(t, s.Jobs(), "grace must not fire a job past its stopTime")
		require.Equal(t, uint32(0), runs.Load())

		require.NoError(t, s.Shutdown())
	})

	t.Run("applies to first run of recurring schedules and does not catch up", func(t *testing.T) {
		defer verifyNoGoroutineLeaks(t)

		schedulerStart := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC)
		fakeClock := clockwork.NewFakeClockAt(schedulerStart)

		s := newTestScheduler(t, WithClock(fakeClock))

		var runs atomic.Uint32
		firstFired := make(chan struct{}, 1)
		startAt := schedulerStart.Add(time.Minute)
		_, err := s.NewJob(
			DurationJob(30*time.Second),
			NewTask(func() {
				runs.Add(1)
				select {
				case firstFired <- struct{}{}:
				default:
				}
			}),
			WithStartAt(WithStartDateTime(startAt)),
			WithStartAtGrace(30*time.Minute),
		)
		require.NoError(t, err)

		// Advance five minutes past startAt — well within grace, well past
		// several 30-second intervals. We expect exactly ONE grace-triggered
		// fire on Start(), NOT catch-up fires for every missed 30s tick.
		fakeClock.Advance(5 * time.Minute)
		s.Start()

		select {
		case <-firstFired:
		case <-time.After(2 * time.Second):
			t.Fatal("expected first grace-triggered fire within 2s")
		}

		// Give the scheduler a moment to (incorrectly) queue catch-ups if
		// it were going to. It shouldn't.
		time.Sleep(100 * time.Millisecond)
		require.Equal(t, uint32(1), runs.Load(), "exactly one grace-triggered fire, no catch-up")

		require.NoError(t, s.Shutdown())
	})
}

// stubSchedule is a test jobSchedule whose next() behavior is fully
// controlled by nextFunc, letting tests simulate exhausted or
// misbehaving schedules (zero / non-advancing next values).
type stubSchedule struct {
	nextFunc func(lastRun time.Time) time.Time
}

func (s stubSchedule) next(lastRun time.Time) time.Time {
	return s.nextFunc(lastRun)
}

// TestScheduler_advanceNextPastDuplicates covers the guard that prevents
// selectExecJobsOutForRescheduling from looping forever (or arming a
// zero-time timer that busy-loops) when a schedule's next() returns the
// zero time or fails to advance while skipping duplicate nextScheduled
// entries. See issue #943 for the failure class.
func TestScheduler_advanceNextPastDuplicates(t *testing.T) {
	base := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC)
	s := &scheduler{location: time.UTC}

	t.Run("no duplicate returns next unchanged", func(t *testing.T) {
		j := internalJob{
			jobSchedule:   stubSchedule{nextFunc: func(_ time.Time) time.Time { t.Fatal("next must not be called"); return time.Time{} }},
			nextScheduled: []time.Time{base.Add(time.Hour)},
		}
		got, ok := s.advanceNextPastDuplicates(j, base)
		require.True(t, ok)
		require.Equal(t, base, got)
	})

	t.Run("advances past duplicate to a future non-duplicate", func(t *testing.T) {
		dup := base.Add(time.Minute)
		want := base.Add(2 * time.Minute)
		j := internalJob{
			jobSchedule:   stubSchedule{nextFunc: func(_ time.Time) time.Time { return want }},
			nextScheduled: []time.Time{dup},
		}
		got, ok := s.advanceNextPastDuplicates(j, dup)
		require.True(t, ok)
		require.Equal(t, want, got)
	})

	t.Run("non-advancing next returns ok=false instead of spinning", func(t *testing.T) {
		dup := base.Add(time.Minute)
		j := internalJob{
			// next() keeps returning the same duplicate value -> no forward progress.
			jobSchedule:   stubSchedule{nextFunc: func(_ time.Time) time.Time { return dup }},
			nextScheduled: []time.Time{dup},
		}
		done := make(chan struct{})
		var ok bool
		go func() {
			_, ok = s.advanceNextPastDuplicates(j, dup)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("advanceNextPastDuplicates spun on a non-advancing next() (regression)")
		}
		require.False(t, ok, "non-advancing schedule must be reported as exhausted")
	})

	t.Run("zero next returns ok=false", func(t *testing.T) {
		dup := base.Add(time.Minute)
		j := internalJob{
			jobSchedule:   stubSchedule{nextFunc: func(_ time.Time) time.Time { return time.Time{} }},
			nextScheduled: []time.Time{dup},
		}
		_, ok := s.advanceNextPastDuplicates(j, dup)
		require.False(t, ok, "zero next must be reported as exhausted")
	})
}

// TestScheduler_advancePastNow exercises the past-time guard directly,
// complementing the integration-level regression test
// TestScheduler_OneTimeJob_PastStartTime_DoesNotSpin.
func TestScheduler_advancePastNow(t *testing.T) {
	now := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC)
	s := &scheduler{location: time.UTC}
	s.exec.clock = clockwork.NewFakeClockAt(now)

	t.Run("already-future next returned unchanged", func(t *testing.T) {
		future := now.Add(time.Hour)
		j := internalJob{jobSchedule: stubSchedule{nextFunc: func(_ time.Time) time.Time { t.Fatal("next must not be called"); return time.Time{} }}}
		got, ok := s.advancePastNow(j, future)
		require.True(t, ok)
		require.Equal(t, future, got)
	})

	t.Run("non-advancing next returns ok=false instead of spinning", func(t *testing.T) {
		past := now.Add(-time.Hour)
		j := internalJob{jobSchedule: stubSchedule{nextFunc: func(lastRun time.Time) time.Time { return lastRun }}}
		done := make(chan struct{})
		var ok bool
		go func() {
			_, ok = s.advancePastNow(j, past)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("advancePastNow spun on a non-advancing next() (regression)")
		}
		require.False(t, ok)
	})
}

// TestScheduler_ExecutionTimeUsesInjectedClock verifies that job timing
// metrics are measured with the scheduler's injected clock rather than the
// wall clock. With a fake clock, a job that advances the clock by a known
// duration during execution must report that same duration as its execution
// time. Prior to the fix, runJob captured start/end via time.Now(), so the
// reported duration reflected wall time (~0) instead of the injected clock.
func TestScheduler_ExecutionTimeUsesInjectedClock(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	fakeClock := clockwork.NewFakeClockAt(time.Date(2050, time.January, 1, 0, 0, 0, 0, time.UTC))
	monitor := newTestSchedulerMonitor()

	s := newTestScheduler(t,
		WithClock(fakeClock),
		WithSchedulerMonitor(monitor),
	)

	const jobDuration = 5 * time.Second
	ran := make(chan struct{}, 1)
	_, err := s.NewJob(
		DurationJob(time.Hour),
		NewTask(func() {
			// Simulate work that takes jobDuration on the injected clock.
			fakeClock.Advance(jobDuration)
			select {
			case ran <- struct{}{}:
			default:
			}
		}),
		WithStartAt(WithStartImmediately()),
	)
	require.NoError(t, err)

	s.Start()

	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not run within 2s")
	}

	require.NoError(t, s.Shutdown())

	monitor.mu.RLock()
	execTimes := append([]time.Duration(nil), monitor.jobExecutionTimes...)
	monitor.mu.RUnlock()

	require.NotEmpty(t, execTimes, "expected at least one JobExecutionTime notification")
	// With the injected clock, execution time equals the advanced duration.
	// With the wall-clock bug it would be sub-millisecond.
	require.GreaterOrEqual(t, execTimes[0], jobDuration,
		"execution time must be measured on the injected clock (got %s, want >= %s)", execTimes[0], jobDuration)
}
