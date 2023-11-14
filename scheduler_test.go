package gocron

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func newTestScheduler(options ...SchedulerOption) (Scheduler, error) {
	// default test options
	out := []SchedulerOption{
		WithLogger(NewLogger(LogLevelDebug)),
		WithStopTimeout(time.Second),
	}

	// append any additional options 2nd to override defaults if needed
	out = append(out, options...)
	return NewScheduler(out...)
}

func TestScheduler_OneSecond_NoOptions(t *testing.T) {
	defer goleak.VerifyNone(t)
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
			s, err := newTestScheduler()
			require.NoError(t, err)

			_, err = s.NewJob(tt.jd, tt.tsk)
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
	defer goleak.VerifyNone(t)

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
			s, err := newTestScheduler(tt.options...)
			require.NoError(t, err)

			_, err = s.NewJob(tt.jd, tt.tsk, tt.opts...)
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
	defer goleak.VerifyNone(t)

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
			s, err := newTestScheduler()
			require.NoError(t, err)

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
	defer goleak.VerifyNone(t)

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
			s, err := newTestScheduler(
				WithStopTimeout(time.Millisecond * 100),
			)
			require.NoError(t, err)

			_, err = s.NewJob(tt.jd, NewTask(tt.f, testDoneCtx), tt.opts...)
			require.NoError(t, err)

			s.Start()
			time.Sleep(time.Millisecond * 200)
			err = s.Shutdown()
			assert.ErrorIs(t, err, ErrStopJobsTimedOut)
			cancel()
			time.Sleep(2 * time.Second)
		})
	}
}

func TestScheduler_Shutdown(t *testing.T) {
	goleak.VerifyNone(t)

	t.Run("start, stop, start, shutdown", func(t *testing.T) {
		s, err := newTestScheduler(
			WithStopTimeout(time.Second),
		)
		require.NoError(t, err)
		_, err = s.NewJob(
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
		time.Sleep(50 * time.Millisecond)
		require.NoError(t, s.StopJobs())

		time.Sleep(200 * time.Millisecond)
		s.Start()

		time.Sleep(50 * time.Millisecond)
		require.NoError(t, s.Shutdown())
		time.Sleep(200 * time.Millisecond)
	})

	t.Run("calling Job methods after shutdown errors", func(t *testing.T) {
		s, err := newTestScheduler(
			WithStopTimeout(time.Second),
		)
		require.NoError(t, err)
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
		time.Sleep(50 * time.Millisecond)
		require.NoError(t, s.Shutdown())

		_, err = j.LastRun()
		assert.ErrorIs(t, err, ErrJobNotFound)

		_, err = j.NextRun()
		assert.ErrorIs(t, err, ErrJobNotFound)
	})
}

func TestScheduler_NewJob(t *testing.T) {
	goleak.VerifyNone(t)
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
			s, err := newTestScheduler()
			require.NoError(t, err)

			_, err = s.NewJob(tt.jd, tt.tsk, tt.opts...)
			require.NoError(t, err)

			s.Start()
			require.NoError(t, s.Shutdown())
			time.Sleep(50 * time.Millisecond)
		})
	}
}

func TestScheduler_NewJobErrors(t *testing.T) {
	goleak.VerifyNone(t)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := newTestScheduler(
				WithStopTimeout(time.Millisecond * 50),
			)
			require.NoError(t, err)

			_, err = s.NewJob(tt.jd, NewTask(func() {}), tt.opts...)
			assert.ErrorIs(t, err, tt.err)
			require.NoError(t, s.Shutdown())
		})
		t.Run(tt.name+" global", func(t *testing.T) {
			s, err := newTestScheduler(
				WithStopTimeout(time.Millisecond*50),
				WithGlobalJobOptions(tt.opts...),
			)
			require.NoError(t, err)

			_, err = s.NewJob(tt.jd, NewTask(func() {}))
			assert.ErrorIs(t, err, tt.err)
			require.NoError(t, s.Shutdown())
		})
	}
}

func TestScheduler_NewJobTask(t *testing.T) {
	goleak.VerifyNone(t)

	testFuncPtr := func() {}

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := newTestScheduler()
			require.NoError(t, err)

			_, err = s.NewJob(DurationJob(time.Second), tt.tsk)
			assert.ErrorIs(t, err, tt.err)
			require.NoError(t, s.Shutdown())
		})
	}
}

func TestScheduler_WithOptionsErrors(t *testing.T) {
	goleak.VerifyNone(t)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newTestScheduler(tt.opt)
			assert.ErrorIs(t, err, tt.err)
		})
	}
}

func TestScheduler_Singleton(t *testing.T) {
	goleak.VerifyNone(t)
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

			s, err := newTestScheduler(
				WithStopTimeout(1*time.Second),
				WithLocation(time.Local),
			)
			require.NoError(t, err)

			_, err = s.NewJob(
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
	goleak.VerifyNone(t)
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
			s, err := newTestScheduler(
				WithLimitConcurrentJobs(tt.limit, tt.limitMode),
				WithStopTimeout(2*time.Second),
			)
			require.NoError(t, err)

			jobRanCh := make(chan struct{}, 20)

			for i := 0; i < tt.numJobs; i++ {
				_, err = s.NewJob(
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

var _ Elector = (*testElector)(nil)

type testElector struct {
	mu            sync.Mutex
	leaderElected bool
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
		return fmt.Errorf("already elected leader")
	}
	t.leaderElected = true
	return nil
}

var _ Locker = (*testLocker)(nil)

type testLocker struct {
	mu        sync.Mutex
	jobLocked bool
}

func (t *testLocker) Lock(_ context.Context, _ string) (Lock, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.jobLocked {
		return nil, fmt.Errorf("job already locked")
	}
	return &testLock{}, nil
}

var _ Lock = (*testLock)(nil)

type testLock struct{}

func (t testLock) Unlock(_ context.Context) error {
	return nil
}

func TestScheduler_WithDistributed(t *testing.T) {
	goleak.VerifyNone(t)
	tests := []struct {
		name  string
		count int
		opt   SchedulerOption
	}{
		{
			"3 schedulers with elector",
			3,
			WithDistributedElector(&testElector{}),
		},
		{
			"3 schedulers with locker",
			3,
			WithDistributedLocker(&testLocker{}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elector := &testElector{}
			jobsRan := make(chan struct{}, 20)
			ctx, cancel := context.WithCancel(context.Background())
			schedulersDone := make(chan struct{}, tt.count)

			for i := tt.count; i > 0; i-- {
				go func() {
					s, err := newTestScheduler(
						WithDistributedElector(elector),
					)
					require.NoError(t, err)

					s.Start()

					_, err = s.NewJob(
						DurationJob(
							time.Second,
						),
						NewTask(
							func() {
								jobsRan <- struct{}{}
							},
						),
						WithStartAt(
							WithStartImmediately(),
						),
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
		})
	}
}

func TestScheduler_RemoveJob(t *testing.T) {
	goleak.VerifyNone(t)
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
			s, err := newTestScheduler()
			require.NoError(t, err)

			var id uuid.UUID
			if tt.addJob {
				j, err := s.NewJob(DurationJob(time.Second), NewTask(func() {}))
				require.NoError(t, err)
				id = j.ID()
			} else {
				id = uuid.New()
			}

			time.Sleep(50 * time.Millisecond)
			err = s.RemoveJob(id)
			assert.ErrorIs(t, err, err)
			require.NoError(t, s.Shutdown())
		})
	}
}

func TestScheduler_WithEventListeners(t *testing.T) {
	goleak.VerifyNone(t)

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
			AfterJobRuns(func(_ uuid.UUID) {
				listenerRunCh <- nil
			}),
			true,
			nil,
		},
		{
			"AfterJobRunsWithError - error",
			NewTask(func() error { return testErr }),
			AfterJobRunsWithError(func(_ uuid.UUID, err error) {
				listenerRunCh <- err
			}),
			true,
			testErr,
		},
		{
			"AfterJobRunsWithError - no error",
			NewTask(func() error { return nil }),
			AfterJobRunsWithError(func(_ uuid.UUID, err error) {
				listenerRunCh <- err
			}),
			false,
			nil,
		},
		{
			"BeforeJobRuns",
			NewTask(func() {}),
			BeforeJobRuns(func(_ uuid.UUID) {
				listenerRunCh <- nil
			}),
			true,
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := newTestScheduler()
			require.NoError(t, err)
			_, err = s.NewJob(
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
