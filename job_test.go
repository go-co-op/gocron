package gocron

import (
	"sync"
	"sync/atomic"
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

func TestCronJob_next_DaylightSavingsTime(t *testing.T) {
	americaNewYork, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	tests := []struct {
		name            string
		crontab         string
		lastRun         time.Time
		expectedNextRun time.Time
	}{
		{
			// Daylight Saving Time fall-back: Nov 1 2026, 02:00 EDT → 01:00 EST.
			// A job scheduled at "30 1 * * *" runs at 01:30 EDT;
			// next must be the following day, not 01:30 EST (duplicate).
			"fall back - cron job should not run twice",
			"30 1 * * *",
			time.Date(2026, 11, 1, 1, 30, 0, 0, americaNewYork), // 01:30 EDT
			time.Date(2026, 11, 2, 1, 30, 0, 0, americaNewYork), // 01:30 EST next day
		},
		{
			// Daylight Saving Time spring-forward: March 8 2026, 02:00 EST → 03:00 EDT.
			// 2:30 AM does not exist on this day.  The underlying cron
			// library correctly skips the non-existent time and schedules
			// the next occurrence on March 9.  This matches standard Unix
			// cron behavior — no "make-up" run should happen at a different
			// time on the same day.
			"spring forward - cron job correctly skips non-existent time",
			"30 2 * * *",
			time.Date(2026, 3, 7, 2, 30, 0, 0, americaNewYork), // March 7 02:30 EST
			time.Date(2026, 3, 9, 2, 30, 0, 0, americaNewYork), // March 9 02:30 EDT (skips March 8)
		},
		{
			// Normal day - should schedule for the next day at the same time.
			"normal day - cron job next day",
			"30 1 * * *",
			time.Date(2026, 6, 15, 1, 30, 0, 0, americaNewYork),
			time.Date(2026, 6, 16, 1, 30, 0, 0, americaNewYork),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cronImpl := newDefaultCronImplementation(false)
			// IsValid needs a "now" before the first expected match so the
			// parsed schedule can find at least one future occurrence.
			err := cronImpl.IsValid(tt.crontab, americaNewYork, tt.lastRun.Add(-time.Hour))
			require.NoError(t, err)

			j := cronJob{crontab: tt.crontab, cronSchedule: cronImpl}
			next := j.next(tt.lastRun)
			assert.Equal(t, tt.expectedNextRun, next)
		})
	}
}

func TestDaylightSavingsTimePolicy_CronJob(t *testing.T) {
	americaNewYork, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	tests := []struct {
		name                      string
		crontab                   string
		daylightSavingsTimePolicy DaylightSavingsTimePolicy
		lastRun                   time.Time
		expectedNextRun           time.Time
	}{
		{
			// Default behavior: cron library skips March 8 (2:30 doesn't exist)
			"default - spring forward skips to next day",
			"30 2 * * *",
			DaylightSavingsTimeDefault,
			time.Date(2026, 3, 7, 2, 30, 0, 0, americaNewYork),
			time.Date(2026, 3, 9, 2, 30, 0, 0, americaNewYork),
		},
		{
			// DaylightSavingsTimeSkip: same as default for cron - skips to next day
			"skip - spring forward skips to next day",
			"30 2 * * *",
			DaylightSavingsTimeSkip,
			time.Date(2026, 3, 7, 2, 30, 0, 0, americaNewYork),
			time.Date(2026, 3, 9, 2, 30, 0, 0, americaNewYork),
		},
		{
			// DaylightSavingsTimeRunAfterTransition: run at adjusted time on March 8
			// 2:30 AM doesn't exist; time.Date normalizes to 3:30 AM EDT
			"run after transition - spring forward runs at adjusted time",
			"30 2 * * *",
			DaylightSavingsTimeRunAfterTransition,
			time.Date(2026, 3, 7, 2, 30, 0, 0, americaNewYork),
			time.Date(2026, 3, 8, 3, 30, 0, 0, americaNewYork),
		},
		{
			// Normal day with RunAfterTransition - no change
			"run after transition - normal day unchanged",
			"30 2 * * *",
			DaylightSavingsTimeRunAfterTransition,
			time.Date(2026, 6, 15, 2, 30, 0, 0, americaNewYork),
			time.Date(2026, 6, 16, 2, 30, 0, 0, americaNewYork),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cronImpl := newDefaultCronImplementation(false)
			err := cronImpl.IsValid(tt.crontab, americaNewYork, tt.lastRun.Add(-time.Hour))
			require.NoError(t, err)

			j := cronJob{crontab: tt.crontab, cronSchedule: cronImpl, daylightSavingsTimePolicy: tt.daylightSavingsTimePolicy}
			next := j.next(tt.lastRun)
			assert.Equal(t, tt.expectedNextRun, next)
		})
	}
}

func TestDaylightSavingsTimePolicy_DailyJob(t *testing.T) {
	americaNewYork, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	tests := []struct {
		name                      string
		interval                  uint
		atTimes                   []time.Time
		daylightSavingsTimePolicy DaylightSavingsTimePolicy
		lastRun                   time.Time
		expectedNextRun           time.Time
	}{
		{
			// Default behavior: time.Date normalizes 2:30 backwards into
			// the pre-transition period. The job runs at the Go-normalized time.
			"default - spring forward runs at normalized time",
			1,
			[]time.Time{
				time.Date(0, 0, 0, 2, 30, 0, 0, americaNewYork),
			},
			DaylightSavingsTimeDefault,
			time.Date(2026, 3, 7, 2, 30, 0, 0, americaNewYork),
			// Go normalizes 2:30 AM → 1:30 AM EST on the Daylight Saving Time day
			time.Date(2026, 3, 8, 1, 30, 0, 0, americaNewYork),
		},
		{
			// DaylightSavingsTimeRunAfterTransition: run at post-transition time (3:30 AM EDT)
			"run after transition - spring forward runs at adjusted time",
			1,
			[]time.Time{
				time.Date(0, 0, 0, 2, 30, 0, 0, americaNewYork),
			},
			DaylightSavingsTimeRunAfterTransition,
			time.Date(2026, 3, 7, 2, 30, 0, 0, americaNewYork),
			time.Date(2026, 3, 8, 3, 30, 0, 0, americaNewYork),
		},
		{
			// DaylightSavingsTimeSkip: skip March 8 entirely, run on March 9
			"skip - spring forward skips to next day",
			1,
			[]time.Time{
				time.Date(0, 0, 0, 2, 30, 0, 0, americaNewYork),
			},
			DaylightSavingsTimeSkip,
			time.Date(2026, 3, 7, 2, 30, 0, 0, americaNewYork),
			time.Date(2026, 3, 9, 2, 30, 0, 0, americaNewYork),
		},
		{
			// DaylightSavingsTimeSkip with interval=2: skip Daylight Saving Time day, advance to next interval
			"skip with interval 2 - spring forward advances to next interval",
			2,
			[]time.Time{
				time.Date(0, 0, 0, 2, 30, 0, 0, americaNewYork),
			},
			DaylightSavingsTimeSkip,
			time.Date(2026, 3, 6, 2, 30, 0, 0, americaNewYork),
			time.Date(2026, 3, 10, 2, 30, 0, 0, americaNewYork),
		},
		{
			// Normal day with DaylightSavingsTimeSkip - no change
			"skip - normal day unchanged",
			1,
			[]time.Time{
				time.Date(0, 0, 0, 2, 30, 0, 0, americaNewYork),
			},
			DaylightSavingsTimeSkip,
			time.Date(2026, 6, 15, 2, 30, 0, 0, americaNewYork),
			time.Date(2026, 6, 16, 2, 30, 0, 0, americaNewYork),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := dailyJob{
				interval:                  tt.interval,
				atTimes:                   tt.atTimes,
				daylightSavingsTimePolicy: tt.daylightSavingsTimePolicy,
			}

			next := d.next(tt.lastRun)
			assert.Equal(t, tt.expectedNextRun, next)
		})
	}
}

func TestDaylightSavingsTimePolicy_WeeklyJob(t *testing.T) {
	americaNewYork, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	// March 8, 2026 is a Sunday - Daylight Saving Time spring-forward day
	tests := []struct {
		name                      string
		interval                  uint
		daysOfWeek                []time.Weekday
		atTimes                   []time.Time
		daylightSavingsTimePolicy DaylightSavingsTimePolicy
		lastRun                   time.Time
		expectedNextRun           time.Time
	}{
		{
			// Default behavior for weekly job on Daylight Saving Time day: time.Date normalizes backwards
			"default - spring forward runs at normalized time",
			1,
			[]time.Weekday{time.Sunday},
			[]time.Time{
				time.Date(0, 0, 0, 2, 30, 0, 0, americaNewYork),
			},
			DaylightSavingsTimeDefault,
			time.Date(2026, 3, 1, 2, 30, 0, 0, americaNewYork),
			// Go normalizes 2:30 AM → 1:30 AM EST on March 8 (Daylight Saving Time day)
			time.Date(2026, 3, 8, 1, 30, 0, 0, americaNewYork),
		},
		{
			// DaylightSavingsTimeRunAfterTransition: run at post-transition time
			"run after transition - spring forward runs at adjusted time",
			1,
			[]time.Weekday{time.Sunday},
			[]time.Time{
				time.Date(0, 0, 0, 2, 30, 0, 0, americaNewYork),
			},
			DaylightSavingsTimeRunAfterTransition,
			time.Date(2026, 3, 1, 2, 30, 0, 0, americaNewYork),
			time.Date(2026, 3, 8, 3, 30, 0, 0, americaNewYork),
		},
		{
			// DaylightSavingsTimeSkip: skip March 8 (Daylight Saving Time Sunday), run on March 15
			"skip - spring forward skips to next week",
			1,
			[]time.Weekday{time.Sunday},
			[]time.Time{
				time.Date(0, 0, 0, 2, 30, 0, 0, americaNewYork),
			},
			DaylightSavingsTimeSkip,
			time.Date(2026, 3, 1, 2, 30, 0, 0, americaNewYork),
			time.Date(2026, 3, 15, 2, 30, 0, 0, americaNewYork),
		},
		{
			// Normal week with DaylightSavingsTimeSkip - no change
			"skip - normal week unchanged",
			1,
			[]time.Weekday{time.Sunday},
			[]time.Time{
				time.Date(0, 0, 0, 2, 30, 0, 0, americaNewYork),
			},
			DaylightSavingsTimeSkip,
			time.Date(2026, 6, 14, 2, 30, 0, 0, americaNewYork),
			time.Date(2026, 6, 21, 2, 30, 0, 0, americaNewYork),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := weeklyJob{
				interval:                  tt.interval,
				daysOfWeek:                tt.daysOfWeek,
				atTimes:                   tt.atTimes,
				daylightSavingsTimePolicy: tt.daylightSavingsTimePolicy,
			}

			next := w.next(tt.lastRun)
			assert.Equal(t, tt.expectedNextRun, next)
		})
	}
}

func TestDaylightSavingsTimePolicy_MonthlyJob(t *testing.T) {
	americaNewYork, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	// March 8, 2026 is Daylight Saving Time spring-forward day
	tests := []struct {
		name                      string
		interval                  uint
		days                      []int
		daysFromEnd               []int
		atTimes                   []time.Time
		daylightSavingsTimePolicy DaylightSavingsTimePolicy
		lastRun                   time.Time
		expectedNextRun           time.Time
	}{
		{
			// Default behavior for monthly job on Daylight Saving Time day: time.Date normalizes backwards
			"default - spring forward runs at normalized time",
			1,
			[]int{8},
			nil,
			[]time.Time{
				time.Date(0, 0, 0, 2, 30, 0, 0, americaNewYork),
			},
			DaylightSavingsTimeDefault,
			time.Date(2026, 2, 8, 2, 30, 0, 0, americaNewYork),
			// Go normalizes 2:30 AM → 1:30 AM EST on March 8 (Daylight Saving Time day)
			time.Date(2026, 3, 8, 1, 30, 0, 0, americaNewYork),
		},
		{
			// DaylightSavingsTimeRunAfterTransition: run at post-transition time
			"run after transition - spring forward runs at adjusted time",
			1,
			[]int{8},
			nil,
			[]time.Time{
				time.Date(0, 0, 0, 2, 30, 0, 0, americaNewYork),
			},
			DaylightSavingsTimeRunAfterTransition,
			time.Date(2026, 2, 8, 2, 30, 0, 0, americaNewYork),
			time.Date(2026, 3, 8, 3, 30, 0, 0, americaNewYork),
		},
		{
			// DaylightSavingsTimeSkip: skip March 8 (Daylight Saving Time day), run on April 8
			"skip - spring forward skips to next month",
			1,
			[]int{8},
			nil,
			[]time.Time{
				time.Date(0, 0, 0, 2, 30, 0, 0, americaNewYork),
			},
			DaylightSavingsTimeSkip,
			time.Date(2026, 2, 8, 2, 30, 0, 0, americaNewYork),
			time.Date(2026, 4, 8, 2, 30, 0, 0, americaNewYork),
		},
		{
			// Normal month with DaylightSavingsTimeSkip - no change
			"skip - normal month unchanged",
			1,
			[]int{8},
			nil,
			[]time.Time{
				time.Date(0, 0, 0, 2, 30, 0, 0, americaNewYork),
			},
			DaylightSavingsTimeSkip,
			time.Date(2026, 5, 8, 2, 30, 0, 0, americaNewYork),
			time.Date(2026, 6, 8, 2, 30, 0, 0, americaNewYork),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := monthlyJob{
				interval:                  tt.interval,
				days:                      tt.days,
				daysFromEnd:               tt.daysFromEnd,
				atTimes:                   tt.atTimes,
				daylightSavingsTimePolicy: tt.daylightSavingsTimePolicy,
			}

			next := m.next(tt.lastRun)
			assert.Equal(t, tt.expectedNextRun, next)
		})
	}
}

func TestDailyJob_next(t *testing.T) {
	americaChicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)

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
		{
			"daily at time with daylight savings time",
			1,
			[]time.Time{
				time.Date(0, 0, 0, 5, 30, 0, 0, americaChicago),
			},
			time.Date(2023, 3, 11, 5, 30, 0, 0, americaChicago),
			time.Date(2023, 3, 12, 5, 30, 0, 0, americaChicago),
			23 * time.Hour,
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
	americaChicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)

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
		{
			"last run before daylight savings time, next run after",
			1,
			[]time.Weekday{time.Saturday},
			[]time.Time{
				time.Date(0, 0, 0, 5, 30, 0, 0, americaChicago),
			},
			time.Date(2023, 3, 11, 5, 30, 0, 0, americaChicago),
			time.Date(2023, 3, 18, 5, 30, 0, 0, americaChicago),
			7*24*time.Hour - time.Hour,
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
				min: tt.min,
				max: tt.max,
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
		assertion func(t *testing.T, previousRun, nextRun time.Time)
	}{
		{
			"simple - milliseconds",
			DurationJob(
				100 * time.Millisecond,
			),
			func(t *testing.T, previousRun, nextRun time.Time) {
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
			func(t *testing.T, previousRun, nextRun time.Time) {
				// With the fix for NextRun accuracy, the immediate run (Jan 1) is removed
				// from nextScheduled after it completes. So all intervals should be 14 days
				// (2 weeks as configured).
				diff := time.Hour * 14 * 24
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
				tt.assertion(t, nextRuns[i-1], nextRuns[i])
			}

			assert.NoError(t, s.Shutdown())
		})
	}
}

func TestJob_NextRuns_StopTime(t *testing.T) {
	stopTime := time.Now().Add(350 * time.Millisecond)

	s := newTestScheduler(t)
	j, err := s.NewJob(
		DurationJob(100*time.Millisecond),
		NewTask(func() {}),
		WithStopAt(WithStopDateTime(stopTime)),
	)
	require.NoError(t, err)

	s.Start()
	time.Sleep(50 * time.Millisecond)

	// nextScheduled must not contain any time at or after stopTime
	ij, err := requestJob(j.ID(), j.(*job).jobOutRequest)
	require.NoError(t, err)
	require.NotNil(t, ij)
	for _, ns := range ij.nextScheduled {
		assert.True(t, ns.Before(stopTime), "nextScheduled contains time after stopTime: %v >= %v", ns, stopTime)
	}

	// NextRuns with a large count must all be before stopTime
	runs, err := j.NextRuns(100)
	require.NoError(t, err)
	for _, r := range runs {
		assert.True(t, r.Before(stopTime), "NextRuns returned time after stopTime: %v >= %v", r, stopTime)
	}

	require.NoError(t, s.Shutdown())
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

func TestWithIntervalFromCompletion_BasicFunctionality(t *testing.T) {
	t.Run("interval calculated from completion time", func(t *testing.T) {
		s, err := NewScheduler()
		require.NoError(t, err)
		defer func() { _ = s.Shutdown() }()

		var mu sync.Mutex
		executions := []struct {
			startTime    time.Time
			completeTime time.Time
		}{}

		jobExecutionTime := 200 * time.Millisecond
		scheduledInterval := 500 * time.Millisecond

		_, err = s.NewJob(
			DurationJob(scheduledInterval),
			NewTask(func() {
				start := time.Now()
				time.Sleep(jobExecutionTime)
				complete := time.Now()

				mu.Lock()
				executions = append(executions, struct {
					startTime    time.Time
					completeTime time.Time
				}{start, complete})
				mu.Unlock()
			}),
			WithIntervalFromCompletion(),
		)
		require.NoError(t, err)

		s.Start()

		// Wait for at least 3 executions
		// With intervalFromCompletion:
		// Execution 1: 0ms-200ms
		// Wait: 500ms (from 200ms to 700ms)
		// Execution 2: 700ms-900ms
		// Wait: 500ms (from 900ms to 1400ms)
		// Execution 3: 1400ms-1600ms
		time.Sleep(1800 * time.Millisecond)

		mu.Lock()
		executionCount := len(executions)
		mu.Unlock()

		require.GreaterOrEqual(t, executionCount, 2,
			"Expected at least 2 executions")

		mu.Lock()
		defer mu.Unlock()

		for i := 1; i < len(executions); i++ {
			prev := executions[i-1]
			curr := executions[i]

			completionToStartGap := curr.startTime.Sub(prev.completeTime)

			assert.InDelta(t, scheduledInterval.Seconds(), completionToStartGap.Seconds(), 0.15,
				"Gap from completion to start should match the interval")
		}
	})
}

func TestWithIntervalFromCompletion_VariableExecutionTime(t *testing.T) {
	s, err := NewScheduler()
	require.NoError(t, err)
	defer func() { _ = s.Shutdown() }()

	var mu sync.Mutex
	executions := []struct {
		startTime    time.Time
		completeTime time.Time
		executionDur time.Duration
	}{}

	executionTimes := []time.Duration{
		100 * time.Millisecond,
		300 * time.Millisecond,
		50 * time.Millisecond,
	}
	currentExecution := atomic.Int32{}
	scheduledInterval := 400 * time.Millisecond

	_, err = s.NewJob(
		DurationJob(scheduledInterval),
		NewTask(func() {
			idx := int(currentExecution.Add(1)) - 1
			if idx >= len(executionTimes) {
				return
			}

			start := time.Now()
			executionTime := executionTimes[idx]
			time.Sleep(executionTime)
			complete := time.Now()

			mu.Lock()
			executions = append(executions, struct {
				startTime    time.Time
				completeTime time.Time
				executionDur time.Duration
			}{start, complete, executionTime})
			mu.Unlock()
		}),
		WithIntervalFromCompletion(),
	)
	require.NoError(t, err)

	s.Start()

	// Wait for all 3 executions
	// Execution 1: 0ms-100ms, wait 400ms → next at 500ms
	// Execution 2: 500ms-800ms, wait 400ms → next at 1200ms
	// Execution 3: 1200ms-1250ms
	time.Sleep(1500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	require.GreaterOrEqual(t, len(executions), 2, "Expected at least 2 executions")

	for i := 1; i < len(executions); i++ {
		prev := executions[i-1]
		curr := executions[i]

		restPeriod := curr.startTime.Sub(prev.completeTime)

		assert.InDelta(t, scheduledInterval.Seconds(), restPeriod.Seconds(), 0.15,
			"Rest period should be consistent regardless of execution time")
	}
}

func TestWithIntervalFromCompletion_LongRunningJob(t *testing.T) {
	s, err := NewScheduler()
	require.NoError(t, err)
	defer func() { _ = s.Shutdown() }()

	var mu sync.Mutex
	executions := []struct {
		startTime    time.Time
		completeTime time.Time
	}{}

	jobExecutionTime := 600 * time.Millisecond
	scheduledInterval := 300 * time.Millisecond

	_, err = s.NewJob(
		DurationJob(scheduledInterval),
		NewTask(func() {
			start := time.Now()
			time.Sleep(jobExecutionTime)
			complete := time.Now()

			mu.Lock()
			executions = append(executions, struct {
				startTime    time.Time
				completeTime time.Time
			}{start, complete})
			mu.Unlock()
		}),
		WithIntervalFromCompletion(),
		WithSingletonMode(LimitModeReschedule),
	)
	require.NoError(t, err)

	s.Start()

	// Wait for 2 executions
	// Execution 1: 0ms-600ms, wait 300ms → next at 900ms
	// Execution 2: 900ms-1500ms, wait 300ms → next at 1800ms
	// Need to wait at least 1600ms for 2 executions + buffer
	time.Sleep(2200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	require.GreaterOrEqual(t, len(executions), 2, "Expected at least 2 executions")

	if len(executions) < 2 {
		t.Logf("Only got %d execution(s), skipping gap assertion", len(executions))
		return
	}

	prev := executions[0]
	curr := executions[1]

	completionGap := curr.startTime.Sub(prev.completeTime)

	assert.InDelta(t, scheduledInterval.Seconds(), completionGap.Seconds(), 0.15,
		"Gap should be the full interval even when execution time exceeds interval")
}

func TestWithIntervalFromCompletion_ComparedToDefault(t *testing.T) {
	jobExecutionTime := 200 * time.Millisecond
	scheduledInterval := 500 * time.Millisecond

	t.Run("default behavior - interval from scheduled time", func(t *testing.T) {
		s, err := NewScheduler()
		require.NoError(t, err)
		defer func() { _ = s.Shutdown() }()

		var mu sync.Mutex
		executions := []struct {
			startTime    time.Time
			completeTime time.Time
		}{}

		_, err = s.NewJob(
			DurationJob(scheduledInterval),
			NewTask(func() {
				start := time.Now()
				time.Sleep(jobExecutionTime)
				complete := time.Now()

				mu.Lock()
				executions = append(executions, struct {
					startTime    time.Time
					completeTime time.Time
				}{start, complete})
				mu.Unlock()
			}),
		)
		require.NoError(t, err)

		s.Start()
		time.Sleep(1300 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()

		require.GreaterOrEqual(t, len(executions), 2, "Expected at least 2 executions")

		prev := executions[0]
		curr := executions[1]
		completionGap := curr.startTime.Sub(prev.completeTime)

		expectedGap := scheduledInterval - jobExecutionTime
		assert.InDelta(t, expectedGap.Seconds(), completionGap.Seconds(), 0.15,
			"Default behavior: gap should be interval minus execution time")
	})

	t.Run("with intervalFromCompletion - interval from completion time", func(t *testing.T) {
		s, err := NewScheduler()
		require.NoError(t, err)
		defer func() { _ = s.Shutdown() }()

		var mu sync.Mutex
		executions := []struct {
			startTime    time.Time
			completeTime time.Time
		}{}

		_, err = s.NewJob(
			DurationJob(scheduledInterval),
			NewTask(func() {
				start := time.Now()
				time.Sleep(jobExecutionTime)
				complete := time.Now()

				mu.Lock()
				executions = append(executions, struct {
					startTime    time.Time
					completeTime time.Time
				}{start, complete})
				mu.Unlock()
			}),
			WithIntervalFromCompletion(),
		)
		require.NoError(t, err)

		s.Start()
		time.Sleep(1500 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()

		require.GreaterOrEqual(t, len(executions), 2, "Expected at least 2 executions")

		prev := executions[0]
		curr := executions[1]
		completionGap := curr.startTime.Sub(prev.completeTime)

		assert.InDelta(t, scheduledInterval.Seconds(), completionGap.Seconds(), 0.15,
			"With intervalFromCompletion: gap should be the full interval")
	})
}

func TestWithIntervalFromCompletion_DurationRandomJob(t *testing.T) {
	s, err := NewScheduler()
	require.NoError(t, err)
	defer func() { _ = s.Shutdown() }()

	var mu sync.Mutex
	executions := []struct {
		startTime    time.Time
		completeTime time.Time
	}{}

	jobExecutionTime := 100 * time.Millisecond
	minInterval := 300 * time.Millisecond
	maxInterval := 400 * time.Millisecond

	_, err = s.NewJob(
		DurationRandomJob(minInterval, maxInterval),
		NewTask(func() {
			start := time.Now()
			time.Sleep(jobExecutionTime)
			complete := time.Now()

			mu.Lock()
			executions = append(executions, struct {
				startTime    time.Time
				completeTime time.Time
			}{start, complete})
			mu.Unlock()
		}),
		WithIntervalFromCompletion(),
	)
	require.NoError(t, err)

	s.Start()

	time.Sleep(1500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	require.GreaterOrEqual(t, len(executions), 2, "Expected at least 2 executions")

	for i := 1; i < len(executions); i++ {
		prev := executions[i-1]
		curr := executions[i]

		restPeriod := curr.startTime.Sub(prev.completeTime)
		assert.GreaterOrEqual(t, restPeriod.Seconds(), minInterval.Seconds()-0.15,
			"Rest period should be at least minInterval")
		assert.LessOrEqual(t, restPeriod.Seconds(), maxInterval.Seconds()+0.15,
			"Rest period should be at most maxInterval")
	}
}

func TestWithIntervalFromCompletion_FirstRun(t *testing.T) {
	s, err := NewScheduler()
	require.NoError(t, err)
	defer func() { _ = s.Shutdown() }()

	var mu sync.Mutex
	var firstRunTime time.Time

	_, err = s.NewJob(
		DurationJob(500*time.Millisecond),
		NewTask(func() {
			mu.Lock()
			if firstRunTime.IsZero() {
				firstRunTime = time.Now()
			}
			mu.Unlock()
		}),
		WithIntervalFromCompletion(),
		WithStartAt(WithStartImmediately()),
	)
	require.NoError(t, err)

	startTime := time.Now()
	s.Start()

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	require.False(t, firstRunTime.IsZero(), "Job should have run at least once")

	timeSinceStart := firstRunTime.Sub(startTime)
	assert.Less(t, timeSinceStart.Seconds(), 0.2,
		"First run should happen quickly with WithStartImmediately")
}

func TestJob_NextRun_MultipleJobsSimultaneously(t *testing.T) {
	// This test reproduces the bug where multiple jobs completing simultaneously
	// would cause NextRun() to return stale values due to race conditions in
	// nextScheduled cleanup.

	testTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(testTime)

	s := newTestScheduler(t,
		WithClock(fakeClock),
		WithLocation(time.UTC),
	)

	jobsCompleted := make(chan struct{}, 4)

	// Create multiple jobs with different intervals that will complete around the same time
	job1, err := s.NewJob(
		DurationJob(1*time.Minute),
		NewTask(func() {
			jobsCompleted <- struct{}{}
		}),
		WithName("job1"),
		WithStartAt(WithStartImmediately()),
	)
	require.NoError(t, err)

	job2, err := s.NewJob(
		DurationJob(2*time.Minute),
		NewTask(func() {
			jobsCompleted <- struct{}{}
		}),
		WithName("job2"),
		WithStartAt(WithStartImmediately()),
	)
	require.NoError(t, err)

	job3, err := s.NewJob(
		DurationJob(3*time.Minute),
		NewTask(func() {
			jobsCompleted <- struct{}{}
		}),
		WithName("job3"),
		WithStartAt(WithStartImmediately()),
	)
	require.NoError(t, err)

	job4, err := s.NewJob(
		DurationJob(4*time.Minute),
		NewTask(func() {
			jobsCompleted <- struct{}{}
		}),
		WithName("job4"),
		WithStartAt(WithStartImmediately()),
	)
	require.NoError(t, err)

	s.Start()

	// Wait for all 4 jobs to complete their immediate run
	for i := 0; i < 4; i++ {
		<-jobsCompleted
	}

	// Give the scheduler time to process the completions and reschedule
	time.Sleep(50 * time.Millisecond)

	// Verify that NextRun() returns the correct next scheduled time for each job
	// and not a stale value from the just-completed run

	nextRun1, err := job1.NextRun()
	require.NoError(t, err)
	assert.Equal(t, testTime.Add(1*time.Minute), nextRun1, "job1 NextRun should be 1 minute from start")

	nextRun2, err := job2.NextRun()
	require.NoError(t, err)
	assert.Equal(t, testTime.Add(2*time.Minute), nextRun2, "job2 NextRun should be 2 minutes from start")

	nextRun3, err := job3.NextRun()
	require.NoError(t, err)
	assert.Equal(t, testTime.Add(3*time.Minute), nextRun3, "job3 NextRun should be 3 minutes from start")

	nextRun4, err := job4.NextRun()
	require.NoError(t, err)
	assert.Equal(t, testTime.Add(4*time.Minute), nextRun4, "job4 NextRun should be 4 minutes from start")

	// Advance time to trigger job1's next run
	fakeClock.Advance(1 * time.Minute)

	// Wait for job1 to complete
	<-jobsCompleted
	time.Sleep(50 * time.Millisecond)

	// After job1's second run, it should be scheduled for +2 minutes from start
	nextRun1, err = job1.NextRun()
	require.NoError(t, err)
	assert.Equal(t, testTime.Add(2*time.Minute), nextRun1, "job1 NextRun should be 2 minutes from start after first interval")

	require.NoError(t, s.Shutdown())
}

func TestJob_NextRun_ConcurrentCompletions(t *testing.T) {
	// This test verifies that when multiple jobs complete at exactly the same time,
	// their NextRun() values are correctly updated without race conditions.

	testTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(testTime)

	s := newTestScheduler(t,
		WithClock(fakeClock),
		WithLocation(time.UTC), // Set scheduler to use UTC to match our test time
	)

	var wg sync.WaitGroup
	jobCompletionBarrier := make(chan struct{})

	// Create jobs that will all complete at the same instant
	createJob := func(name string, interval time.Duration) Job {
		job, err := s.NewJob(
			DurationJob(interval),
			NewTask(func() {
				wg.Done()
				<-jobCompletionBarrier // Wait until all jobs are ready to complete
			}),
			WithName(name),
			WithStartAt(WithStartImmediately()),
		)
		require.NoError(t, err)
		return job
	}

	wg.Add(4)
	job1 := createJob("concurrent-job1", 1*time.Minute)
	job2 := createJob("concurrent-job2", 2*time.Minute)
	job3 := createJob("concurrent-job3", 3*time.Minute)
	job4 := createJob("concurrent-job4", 4*time.Minute)

	s.Start()

	wg.Wait()
	close(jobCompletionBarrier)

	// Give the scheduler time to process all completions
	time.Sleep(100 * time.Millisecond)

	// Verify NextRun() for all jobs concurrently to stress test the race condition
	var testWg sync.WaitGroup
	testWg.Add(4)

	go func() {
		defer testWg.Done()
		for i := 0; i < 10; i++ {
			nextRun, err := job1.NextRun()
			require.NoError(t, err)
			assert.Equal(t, testTime.Add(1*time.Minute), nextRun)
		}
	}()

	go func() {
		defer testWg.Done()
		for i := 0; i < 10; i++ {
			nextRun, err := job2.NextRun()
			require.NoError(t, err)
			assert.Equal(t, testTime.Add(2*time.Minute), nextRun)
		}
	}()

	go func() {
		defer testWg.Done()
		for i := 0; i < 10; i++ {
			nextRun, err := job3.NextRun()
			require.NoError(t, err)
			assert.Equal(t, testTime.Add(3*time.Minute), nextRun)
		}
	}()

	go func() {
		defer testWg.Done()
		for i := 0; i < 10; i++ {
			nextRun, err := job4.NextRun()
			require.NoError(t, err)
			assert.Equal(t, testTime.Add(4*time.Minute), nextRun)
		}
	}()

	testWg.Wait()
	require.NoError(t, s.Shutdown())
}

func TestJob_LastRunCompletedAt(t *testing.T) {
	s := newTestScheduler(t)

	ch := make(chan struct{}, 1)

	j, err := s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {
				time.Sleep(50 * time.Millisecond)
				ch <- struct{}{}
			},
		),
		WithStartAt(WithStartImmediately()),
	)
	require.NoError(t, err)

	// Before starting, LastRunCompletedAt should be zero
	completedAt, err := j.LastRunCompletedAt()
	assert.NoError(t, err)
	assert.True(t, completedAt.IsZero())

	s.Start()

	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for job to complete")
	}

	// Poll until the timing update propagates to the scheduler
	require.Eventually(t, func() bool {
		completedAt, err = j.LastRunCompletedAt()
		return err == nil && !completedAt.IsZero()
	}, 5*time.Second, 10*time.Millisecond)

	err = s.Shutdown()
	require.NoError(t, err)
}

func TestJob_IsRunning(t *testing.T) {
	s := newTestScheduler(t)

	started := make(chan struct{}, 1)
	finish := make(chan struct{})

	j, err := s.NewJob(
		DurationJob(
			10*time.Second,
		),
		NewTask(
			func() {
				started <- struct{}{}
				<-finish
			},
		),
		WithStartAt(WithStartImmediately()),
	)
	require.NoError(t, err)

	// Before starting, IsRunning should be false
	running, err := j.IsRunning()
	assert.NoError(t, err)
	assert.False(t, running)

	s.Start()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for job to start")
	}

	// Poll until the timing update propagates
	require.Eventually(t, func() bool {
		running, err = j.IsRunning()
		return err == nil && running
	}, 5*time.Second, 10*time.Millisecond)

	// Signal the job to finish
	close(finish)

	// Poll until the job completion propagates
	require.Eventually(t, func() bool {
		running, err = j.IsRunning()
		return err == nil && !running
	}, 5*time.Second, 10*time.Millisecond)

	err = s.Shutdown()
	require.NoError(t, err)
}
