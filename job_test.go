package gocron

import (
	"math/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurationJob_next(t *testing.T) {
	tests := []time.Duration{
		time.Millisecond,
		time.Second,
		100 * time.Second,
		1000 * time.Second,
		5 * time.Second,
		50 * time.Second,
		time.Minute,
		5 * time.Minute,
		100 * time.Minute,
		time.Hour,
		2 * time.Hour,
		100 * time.Hour,
		1000 * time.Hour,
	}

	lastRun := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, duration := range tests {
		t.Run(duration.String(), func(t *testing.T) {
			d := durationJob{duration: duration}
			next := d.next(lastRun)
			expected := lastRun.Add(duration)

			assert.Equal(t, expected, next)
		})
	}
}

func TestDailyJob_next(t *testing.T) {
	tests := []struct {
		name                      string
		interval                  uint
		atTimes                   []time.Time
		lastRun                   time.Time
		expectedNextRun           time.Time
		expectedDurationToNextRun time.Duration
	}{
		{
			"daily at midnight",
			1,
			[]time.Time{
				time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC),
			},
			time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC),
			24 * time.Hour,
		},
		{
			"daily multiple at times",
			1,
			[]time.Time{
				time.Date(0, 0, 0, 5, 30, 0, 0, time.UTC),
				time.Date(0, 0, 0, 12, 30, 0, 0, time.UTC),
			},
			time.Date(2000, 1, 1, 5, 30, 0, 0, time.UTC),
			time.Date(2000, 1, 1, 12, 30, 0, 0, time.UTC),
			7 * time.Hour,
		},
		{
			"every 2 days multiple at times",
			2,
			[]time.Time{
				time.Date(0, 0, 0, 5, 30, 0, 0, time.UTC),
				time.Date(0, 0, 0, 12, 30, 0, 0, time.UTC),
			},
			time.Date(2000, 1, 1, 12, 30, 0, 0, time.UTC),
			time.Date(2000, 1, 3, 5, 30, 0, 0, time.UTC),
			41 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := dailyJob{
				interval: tt.interval,
				atTimes:  tt.atTimes,
			}

			next := d.next(tt.lastRun)
			assert.Equal(t, tt.expectedNextRun, next)
			assert.Equal(t, tt.expectedDurationToNextRun, next.Sub(tt.lastRun))
		})
	}
}

func TestWeeklyJob_next(t *testing.T) {
	tests := []struct {
		name                      string
		interval                  uint
		daysOfWeek                []time.Weekday
		atTimes                   []time.Time
		lastRun                   time.Time
		expectedNextRun           time.Time
		expectedDurationToNextRun time.Duration
	}{
		{
			"last run Monday, next run is Thursday",
			1,
			[]time.Weekday{time.Monday, time.Thursday},
			[]time.Time{
				time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC),
			},
			time.Date(2000, 1, 3, 0, 0, 0, 0, time.UTC),
			time.Date(2000, 1, 6, 0, 0, 0, 0, time.UTC),
			3 * 24 * time.Hour,
		},
		{
			"last run Thursday, next run is Monday",
			1,
			[]time.Weekday{time.Monday, time.Thursday},
			[]time.Time{
				time.Date(0, 0, 0, 5, 30, 0, 0, time.UTC),
			},
			time.Date(2000, 1, 6, 5, 30, 0, 0, time.UTC),
			time.Date(2000, 1, 10, 5, 30, 0, 0, time.UTC),
			4 * 24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := weeklyJob{
				interval:   tt.interval,
				daysOfWeek: tt.daysOfWeek,
				atTimes:    tt.atTimes,
			}

			next := w.next(tt.lastRun)
			assert.Equal(t, tt.expectedNextRun, next)
			assert.Equal(t, tt.expectedDurationToNextRun, next.Sub(tt.lastRun))
		})
	}
}

func TestMonthlyJob_next(t *testing.T) {
	americaChicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)

	tests := []struct {
		name                      string
		interval                  uint
		days                      []int
		daysFromEnd               []int
		atTimes                   []time.Time
		lastRun                   time.Time
		expectedNextRun           time.Time
		expectedDurationToNextRun time.Duration
	}{
		{
			"same day - before at time",
			1,
			[]int{1},
			nil,
			[]time.Time{
				time.Date(0, 0, 0, 5, 30, 0, 0, time.UTC),
			},
			time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2000, 1, 1, 5, 30, 0, 0, time.UTC),
			5*time.Hour + 30*time.Minute,
		},
		{
			"same day - after at time, runs next available date",
			1,
			[]int{1, 10},
			nil,
			[]time.Time{
				time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC),
			},
			time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2000, 1, 10, 0, 0, 0, 0, time.UTC),
			9 * 24 * time.Hour,
		},
		{
			"same day - after at time, runs next available date, following interval month",
			2,
			[]int{1},
			nil,
			[]time.Time{
				time.Date(0, 0, 0, 5, 30, 0, 0, time.UTC),
			},
			time.Date(2000, 1, 1, 5, 30, 0, 0, time.UTC),
			time.Date(2000, 3, 1, 5, 30, 0, 0, time.UTC),
			60 * 24 * time.Hour,
		},
		{
			"daylight savings time",
			1,
			[]int{5},
			nil,
			[]time.Time{
				time.Date(0, 0, 0, 5, 30, 0, 0, americaChicago),
			},
			time.Date(2023, 11, 1, 0, 0, 0, 0, americaChicago),
			time.Date(2023, 11, 5, 5, 30, 0, 0, americaChicago),
			4*24*time.Hour + 6*time.Hour + 30*time.Minute,
		},
		{
			"negative days",
			1,
			nil,
			[]int{-1, -3, -5},
			[]time.Time{
				time.Date(0, 0, 0, 5, 30, 0, 0, time.UTC),
			},
			time.Date(2000, 1, 29, 5, 30, 0, 0, time.UTC),
			time.Date(2000, 1, 31, 5, 30, 0, 0, time.UTC),
			2 * 24 * time.Hour,
		},
		{
			"day not in current month, runs next month (leap year)",
			1,
			[]int{31},
			nil,
			[]time.Time{
				time.Date(0, 0, 0, 5, 30, 0, 0, time.UTC),
			},
			time.Date(2000, 1, 31, 5, 30, 0, 0, time.UTC),
			time.Date(2000, 3, 31, 5, 30, 0, 0, time.UTC),
			29*24*time.Hour + 31*24*time.Hour,
		},
		{
			"multiple days not in order",
			1,
			[]int{10, 7, 19, 2},
			nil,
			[]time.Time{
				time.Date(0, 0, 0, 5, 30, 0, 0, time.UTC),
			},
			time.Date(2000, 1, 2, 5, 30, 0, 0, time.UTC),
			time.Date(2000, 1, 7, 5, 30, 0, 0, time.UTC),
			5 * 24 * time.Hour,
		},
		{
			"day not in next interval month, selects next available option, skips Feb, April & June",
			2,
			[]int{31},
			nil,
			[]time.Time{
				time.Date(0, 0, 0, 5, 30, 0, 0, time.UTC),
			},
			time.Date(1999, 12, 31, 5, 30, 0, 0, time.UTC),
			time.Date(2000, 8, 31, 5, 30, 0, 0, time.UTC),
			244 * 24 * time.Hour,
		},
		{
			"handle -1 with differing month's day count",
			1,
			nil,
			[]int{-1},
			[]time.Time{
				time.Date(0, 0, 0, 5, 30, 0, 0, time.UTC),
			},
			time.Date(2024, 1, 31, 5, 30, 0, 0, time.UTC),
			time.Date(2024, 2, 29, 5, 30, 0, 0, time.UTC),
			29 * 24 * time.Hour,
		},
		{
			"handle -1 with another differing month's day count",
			1,
			nil,
			[]int{-1},
			[]time.Time{
				time.Date(0, 0, 0, 5, 30, 0, 0, time.UTC),
			},
			time.Date(2024, 2, 29, 5, 30, 0, 0, time.UTC),
			time.Date(2024, 3, 31, 5, 30, 0, 0, time.UTC),
			31 * 24 * time.Hour,
		},
		{
			"handle -1 every 3 months next run in February",
			3,
			nil,
			[]int{-1},
			[]time.Time{
				time.Date(0, 0, 0, 5, 30, 0, 0, time.UTC),
			},
			time.Date(2023, 11, 30, 5, 30, 0, 0, time.UTC),
			time.Date(2024, 2, 29, 5, 30, 0, 0, time.UTC),
			91 * 24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := monthlyJob{
				interval:    tt.interval,
				days:        tt.days,
				daysFromEnd: tt.daysFromEnd,
				atTimes:     tt.atTimes,
			}

			next := m.next(tt.lastRun)
			assert.Equal(t, tt.expectedNextRun, next)
			assert.Equal(t, tt.expectedDurationToNextRun, next.Sub(tt.lastRun))
		})
	}
}

func TestDurationRandomJob_next(t *testing.T) {
	tests := []struct {
		name        string
		min         time.Duration
		max         time.Duration
		lastRun     time.Time
		expectedMin time.Time
		expectedMax time.Time
	}{
		{
			"min 1s, max 5s",
			time.Second,
			5 * time.Second,
			time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2000, 1, 1, 0, 0, 1, 0, time.UTC),
			time.Date(2000, 1, 1, 0, 0, 5, 0, time.UTC),
		},
		{
			"min 100ms, max 1s",
			100 * time.Millisecond,
			1 * time.Second,
			time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2000, 1, 1, 0, 0, 0, 100000000, time.UTC),
			time.Date(2000, 1, 1, 0, 0, 1, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rj := durationRandomJob{
				min:  tt.min,
				max:  tt.max,
				rand: rand.New(rand.NewSource(time.Now().UnixNano())), // nolint:gosec
			}

			for i := 0; i < 100; i++ {
				next := rj.next(tt.lastRun)
				assert.GreaterOrEqual(t, next, tt.expectedMin)
				assert.LessOrEqual(t, next, tt.expectedMax)
			}
		})
	}
}

func TestOneTimeJob_next(t *testing.T) {
	otj := oneTimeJob{}
	assert.Zero(t, otj.next(time.Time{}))
}

func TestJob_RunNow_Error(t *testing.T) {
	s := newTestScheduler(t)

	j, err := s.NewJob(
		DurationJob(time.Second),
		NewTask(func() {}),
	)
	require.NoError(t, err)

	require.NoError(t, s.Shutdown())

	assert.EqualError(t, j.RunNow(), ErrJobRunNowFailed.Error())
}

func TestJob_LastRun(t *testing.T) {
	testTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
	fakeClock := clockwork.NewFakeClockAt(testTime)

	s := newTestScheduler(t,
		WithClock(fakeClock),
	)

	j, err := s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
		WithStartAt(WithStartImmediately()),
	)
	require.NoError(t, err)

	s.Start()
	time.Sleep(10 * time.Millisecond)

	lastRun, err := j.LastRun()
	assert.NoError(t, err)

	err = s.Shutdown()
	require.NoError(t, err)

	assert.Equal(t, testTime, lastRun)
}

func TestWithEventListeners(t *testing.T) {
	tests := []struct {
		name           string
		eventListeners []EventListener
		err            error
	}{
		{
			"no event listeners",
			nil,
			nil,
		},
		{
			"beforeJobRuns",
			[]EventListener{
				BeforeJobRuns(func(_ uuid.UUID, _ string) {}),
			},
			nil,
		},
		{
			"afterJobRuns",
			[]EventListener{
				AfterJobRuns(func(_ uuid.UUID, _ string) {}),
			},
			nil,
		},
		{
			"afterJobRunsWithError",
			[]EventListener{
				AfterJobRunsWithError(func(_ uuid.UUID, _ string, _ error) {}),
			},
			nil,
		},
		{
			"afterJobRunsWithPanic",
			[]EventListener{
				AfterJobRunsWithPanic(func(_ uuid.UUID, _ string, _ any) {}),
			},
			nil,
		},
		{
			"afterLockError",
			[]EventListener{
				AfterLockError(func(_ uuid.UUID, _ string, _ error) {}),
			},
			nil,
		},
		{
			"multiple event listeners",
			[]EventListener{
				AfterJobRuns(func(_ uuid.UUID, _ string) {}),
				AfterJobRunsWithError(func(_ uuid.UUID, _ string, _ error) {}),
				BeforeJobRuns(func(_ uuid.UUID, _ string) {}),
				AfterLockError(func(_ uuid.UUID, _ string, _ error) {}),
			},
			nil,
		},
		{
			"nil after job runs listener",
			[]EventListener{
				AfterJobRuns(nil),
			},
			ErrEventListenerFuncNil,
		},
		{
			"nil after job runs with error listener",
			[]EventListener{
				AfterJobRunsWithError(nil),
			},
			ErrEventListenerFuncNil,
		},
		{
			"nil before job runs listener",
			[]EventListener{
				BeforeJobRuns(nil),
			},
			ErrEventListenerFuncNil,
		},
		{
			"nil before job runs error listener",
			[]EventListener{
				BeforeJobRunsSkipIfBeforeFuncErrors(nil),
			},
			ErrEventListenerFuncNil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ij internalJob
			err := WithEventListeners(tt.eventListeners...)(&ij, time.Now())
			assert.Equal(t, tt.err, err)

			if err != nil {
				return
			}
			var count int
			if ij.beforeJobRuns != nil {
				count++
			}
			if ij.afterJobRuns != nil {
				count++
			}
			if ij.afterJobRunsWithError != nil {
				count++
			}
			if ij.afterJobRunsWithPanic != nil {
				count++
			}
			if ij.afterLockError != nil {
				count++
			}
			assert.Equal(t, len(tt.eventListeners), count)
		})
	}
}

func TestJob_NextRun(t *testing.T) {
	tests := []struct {
		name string
		f    func()
	}{
		{
			"simple",
			func() {},
		},
		{
			"sleep 3 seconds",
			func() {
				time.Sleep(300 * time.Millisecond)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testTime := time.Now()

			s := newTestScheduler(t)

			j, err := s.NewJob(
				DurationJob(
					100*time.Millisecond,
				),
				NewTask(
					func() {},
				),
				WithStartAt(WithStartDateTime(testTime.Add(100*time.Millisecond))),
				WithSingletonMode(LimitModeReschedule),
			)
			require.NoError(t, err)

			s.Start()
			nextRun, err := j.NextRun()
			require.NoError(t, err)

			assert.Equal(t, testTime.Add(100*time.Millisecond), nextRun)

			time.Sleep(150 * time.Millisecond)

			nextRun, err = j.NextRun()
			assert.NoError(t, err)

			assert.Equal(t, testTime.Add(200*time.Millisecond), nextRun)
			assert.Equal(t, 200*time.Millisecond, nextRun.Sub(testTime))

			err = s.Shutdown()
			require.NoError(t, err)
		})
	}
}

func TestJob_NextRuns(t *testing.T) {
	tests := []struct {
		name      string
		jd        JobDefinition
		assertion func(t *testing.T, iteration int, previousRun, nextRun time.Time)
	}{
		{
			"simple - milliseconds",
			DurationJob(
				100 * time.Millisecond,
			),
			func(t *testing.T, _ int, previousRun, nextRun time.Time) {
				assert.Equal(t, previousRun.UnixMilli()+100, nextRun.UnixMilli())
			},
		},
		{
			"weekly",
			WeeklyJob(
				2,
				NewWeekdays(time.Tuesday),
				NewAtTimes(
					NewAtTime(0, 0, 0),
				),
			),
			func(t *testing.T, iteration int, previousRun, nextRun time.Time) {
				diff := time.Hour * 14 * 24
				if iteration == 1 {
					// because the job is run immediately, the first run is on
					// Saturday 1/1/2000. The following run is then on Tuesday 1/11/2000
					diff = time.Hour * 10 * 24
				}
				assert.Equal(t, previousRun.Add(diff).Day(), nextRun.Day())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
			fakeClock := clockwork.NewFakeClockAt(testTime)

			s := newTestScheduler(t,
				WithClock(fakeClock),
			)

			j, err := s.NewJob(
				tt.jd,
				NewTask(
					func() {},
				),
				WithStartAt(WithStartImmediately()),
			)
			require.NoError(t, err)

			s.Start()
			time.Sleep(10 * time.Millisecond)

			nextRuns, err := j.NextRuns(5)
			require.NoError(t, err)

			assert.Len(t, nextRuns, 5)

			for i := range nextRuns {
				if i == 0 {
					// skipping because there is no previous run
					continue
				}
				tt.assertion(t, i, nextRuns[i-1], nextRuns[i])
			}

			assert.NoError(t, s.Shutdown())
		})
	}
}

func TestJob_PanicOccurred(t *testing.T) {
	gotCh := make(chan any)
	errCh := make(chan error)
	s := newTestScheduler(t)
	_, err := s.NewJob(
		DurationJob(10*time.Millisecond),
		NewTask(func() {
			a := 0
			_ = 1 / a
		}),
		WithEventListeners(
			AfterJobRunsWithPanic(func(_ uuid.UUID, _ string, recoverData any) {
				gotCh <- recoverData
			}), AfterJobRunsWithError(func(_ uuid.UUID, _ string, err error) {
				errCh <- err
			}),
		),
	)
	require.NoError(t, err)

	s.Start()
	got := <-gotCh
	require.EqualError(t, got.(error), "runtime error: integer divide by zero")

	err = <-errCh
	require.ErrorIs(t, err, ErrPanicRecovered)
	require.EqualError(t, err, "gocron: panic recovered from runtime error: integer divide by zero")

	require.NoError(t, s.Shutdown())
	close(gotCh)
	close(errCh)
}

func TestTimeFromAtTime(t *testing.T) {
	testTimeUTC := time.Date(0, 0, 0, 1, 1, 1, 0, time.UTC)
	cst, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)
	testTimeCST := time.Date(0, 0, 0, 1, 1, 1, 0, cst)

	tests := []struct {
		name         string
		at           AtTime
		loc          *time.Location
		expectedTime time.Time
		expectedStr  string
	}{
		{
			"UTC",
			NewAtTime(
				uint(testTimeUTC.Hour()),
				uint(testTimeUTC.Minute()),
				uint(testTimeUTC.Second()),
			),
			time.UTC,
			testTimeUTC,
			"01:01:01",
		},
		{
			"CST",
			NewAtTime(
				uint(testTimeCST.Hour()),
				uint(testTimeCST.Minute()),
				uint(testTimeCST.Second()),
			),
			cst,
			testTimeCST,
			"01:01:01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TimeFromAtTime(tt.at, tt.loc)
			assert.Equal(t, tt.expectedTime, result)

			resultFmt := result.Format("15:04:05")
			assert.Equal(t, tt.expectedStr, resultFmt)
		})
	}
}

func TestNewAtTimes(t *testing.T) {
	at := NewAtTimes(
		NewAtTime(1, 1, 1),
		NewAtTime(2, 2, 2),
	)

	var times []string
	for _, att := range at() {
		timeStr := TimeFromAtTime(att, time.UTC).Format("15:04")
		times = append(times, timeStr)
	}

	var timesAgain []string
	for _, att := range at() {
		timeStr := TimeFromAtTime(att, time.UTC).Format("15:04")
		timesAgain = append(timesAgain, timeStr)
	}

	assert.Equal(t, times, timesAgain)
}

func TestNewWeekdays(t *testing.T) {
	wd := NewWeekdays(
		time.Monday,
		time.Tuesday,
	)

	var dayStrings []string
	for _, w := range wd() {
		dayStrings = append(dayStrings, w.String())
	}

	var dayStringsAgain []string
	for _, w := range wd() {
		dayStringsAgain = append(dayStringsAgain, w.String())
	}

	assert.Equal(t, dayStrings, dayStringsAgain)
}

func TestNewDaysOfTheMonth(t *testing.T) {
	dom := NewDaysOfTheMonth(1, 2, 3)

	var domInts []int
	for _, d := range dom() {
		domInts = append(domInts, d)
	}

	var domIntsAgain []int
	for _, d := range dom() {
		domIntsAgain = append(domIntsAgain, d)
	}

	assert.Equal(t, domInts, domIntsAgain)
}
