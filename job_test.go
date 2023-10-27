package gocron

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurationJob_next(t *testing.T) {

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
