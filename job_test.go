package gocron

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurationJob(t *testing.T) {
	tests := []struct {
		name        string
		duration    time.Duration
		expectedErr *string
	}{
		{"success", time.Second, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewScheduler()
			require.NoError(t, err)

			_, err = s.NewJob(
				DurationJob(
					tt.duration,
					NewTask(
						func() {},
						nil,
					),
				),
			)
			require.NoError(t, err)

			s.Start()
			err = s.Shutdown()
			require.NoError(t, err)
		})
	}
}

func TestMonthlyJob_next(t *testing.T) {
	americaChicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)

	tests := []struct {
		name                      string
		days                      []int
		daysFromEnd               []int
		atTimes                   []time.Time
		lastRun                   time.Time
		expectedNextRun           time.Time
		expectedDurationToNextRun time.Duration
	}{
		{
			"same day",
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
			"daylight savings time",
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
			"day not in current month, runs next month",
			[]int{31},
			nil,
			[]time.Time{
				time.Date(0, 0, 0, 5, 30, 0, 0, time.UTC),
			},
			time.Date(2000, 1, 31, 5, 30, 0, 0, time.UTC),
			time.Date(2000, 3, 31, 5, 30, 0, 0, time.UTC),
			29*24*time.Hour + 31*24*time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := monthlyJob{
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
