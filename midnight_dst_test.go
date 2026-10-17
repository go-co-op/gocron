package gocron

import (
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
)

func TestCalendarJobNextMidnightTransition(t *testing.T) {
	location, err := time.LoadLocation("America/Santiago")
	require.NoError(t, err)
	atTimes := NewAtTimes(NewAtTime(9, 0, 0))
	tests := []struct {
		name       string
		definition JobDefinition
		lastRun    time.Time
	}{
		{"daily", DailyJob(1, atTimes), time.Date(2024, 9, 7, 9, 0, 0, 0, location)},
		{"daily interval two", DailyJob(2, atTimes), time.Date(2024, 9, 6, 9, 0, 0, 0, location)},
		{"weekly", WeeklyJob(1, NewWeekdays(time.Sunday), atTimes), time.Date(2024, 9, 1, 9, 0, 0, 0, location)},
		{"weekly interval two", WeeklyJob(2, NewWeekdays(time.Sunday), atTimes), time.Date(2024, 8, 25, 9, 0, 0, 0, location)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, policy := range []DaylightSavingsTimePolicy{
				DaylightSavingsTimeDefault,
				DaylightSavingsTimeSkip,
				DaylightSavingsTimeRunAfterTransition,
			} {
				j := &internalJob{daylightSavingsTimePolicy: policy}
				require.NoError(t, tt.definition.setup(j, location, tt.lastRun))
				require.Equal(t, time.Date(2024, 9, 8, 9, 0, 0, 0, location), j.next(tt.lastRun), "policy %v", policy)
			}
		})
	}
}

func TestCalendarJobNextMidnightGapSkip(t *testing.T) {
	location, err := time.LoadLocation("America/Santiago")
	require.NoError(t, err)
	atTimes := NewAtTimes(NewAtTime(0, 30, 0))
	tests := []struct {
		name       string
		definition JobDefinition
		lastRun    time.Time
		skippedDay int
	}{
		{"daily", DailyJob(1, atTimes), time.Date(2024, 9, 7, 0, 30, 0, 0, location), 9},
		{"daily interval two", DailyJob(2, atTimes), time.Date(2024, 9, 6, 0, 30, 0, 0, location), 10},
		{"weekly", WeeklyJob(1, NewWeekdays(time.Sunday), atTimes), time.Date(2024, 9, 1, 0, 30, 0, 0, location), 15},
		{"weekly interval two", WeeklyJob(2, NewWeekdays(time.Sunday), atTimes), time.Date(2024, 8, 25, 0, 30, 0, 0, location), 22},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := &internalJob{daylightSavingsTimePolicy: DaylightSavingsTimeSkip}
			require.NoError(t, tt.definition.setup(j, location, tt.lastRun))
			require.Equal(t, time.Date(2024, 9, tt.skippedDay, 0, 30, 0, 0, location), j.next(tt.lastRun))
		})
	}
}

func TestSchedulerNextRunsMidnightTransition(t *testing.T) {
	location, err := time.LoadLocation("America/Santiago")
	require.NoError(t, err)
	atTimes := NewAtTimes(NewAtTime(9, 0, 0))
	tests := []struct {
		name       string
		definition JobDefinition
		startDay   int
	}{
		{"daily", DailyJob(1, atTimes), 7},
		{"weekly", WeeklyJob(1, NewWeekdays(time.Sunday), atTimes), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := clockwork.NewFakeClockAt(time.Date(2024, 9, tt.startDay, 8, 0, 0, 0, location))
			s, err := NewScheduler(WithClock(clock), WithLocation(location))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, s.Shutdown()) })
			j, err := s.NewJob(tt.definition, NewTask(func() {}))
			require.NoError(t, err)
			s.Start()
			next, err := j.NextRuns(2)
			require.NoError(t, err)
			require.Equal(t, []time.Time{
				time.Date(2024, 9, tt.startDay, 9, 0, 0, 0, location),
				time.Date(2024, 9, 8, 9, 0, 0, 0, location),
			}, next)
		})
	}
}
