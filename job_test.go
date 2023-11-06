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
				time.Date(0, 0, 0, 5, 30, 0, 0, time.UTC),
			},
			time.Date(2000, 1, 3, 5, 30, 0, 0, time.UTC),
			time.Date(2000, 1, 6, 5, 30, 0, 0, time.UTC),
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
				time.Date(0, 0, 0, 5, 30, 0, 0, time.UTC),
			},
			time.Date(2000, 1, 1, 5, 30, 0, 0, time.UTC),
			time.Date(2000, 1, 10, 5, 30, 0, 0, time.UTC),
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

func TestJob_LastRun(t *testing.T) {
	testTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
	fakeClock := clockwork.NewFakeClockAt(testTime)

	s, err := newTestScheduler(
		WithClock(fakeClock),
	)
	require.NoError(t, err)

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
			"afterJobRuns",
			[]EventListener{
				AfterJobRuns(func(_ uuid.UUID) {}),
			},
			nil,
		},
		{
			"afterJobRunsWithError",
			[]EventListener{
				AfterJobRunsWithError(func(_ uuid.UUID, _ error) {}),
			},
			nil,
		},
		{
			"beforeJobRuns",
			[]EventListener{
				BeforeJobRuns(func(_ uuid.UUID) {}),
			},
			nil,
		},
		{
			"multiple event listeners",
			[]EventListener{
				AfterJobRuns(func(_ uuid.UUID) {}),
				AfterJobRunsWithError(func(_ uuid.UUID, _ error) {}),
				BeforeJobRuns(func(_ uuid.UUID) {}),
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ij internalJob
			err := WithEventListeners(tt.eventListeners...)(&ij)
			assert.Equal(t, tt.err, err)

			if err != nil {
				return
			}
			var count int
			if ij.afterJobRuns != nil {
				count++
			}
			if ij.afterJobRunsWithError != nil {
				count++
			}
			if ij.beforeJobRuns != nil {
				count++
			}
			assert.Equal(t, len(tt.eventListeners), count)
		})
	}
}
