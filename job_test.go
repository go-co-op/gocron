package gocron

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTags(t *testing.T) {
	j, _ := NewScheduler(time.UTC).Every(1).Minute().Do(task)
	j.Tag("some")
	j.Tag("tag")
	j.Tag("more")
	j.Tag("tags")

	assert.ElementsMatch(t, j.Tags(), []string{"tags", "tag", "more", "some"})

	j.Untag("more")
	assert.ElementsMatch(t, j.Tags(), []string{"tags", "tag", "some"})
}

func TestGetScheduledTime(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		j, err := NewScheduler(time.UTC).Every(1).Day().At("10:30").Do(task)
		require.NoError(t, err)
		assert.Equal(t, "10:30", j.ScheduledAtTime())
	})
	t.Run("invalid", func(t *testing.T) {
		j, err := NewScheduler(time.UTC).Every(1).Minute().At("10:30").Do(task)
		assert.EqualError(t, err, ErrAtTimeNotSupported.Error())
		assert.Nil(t, j)
	})
}

func TestGetWeekday(t *testing.T) {
	s := NewScheduler(time.UTC)
	wednesday := time.Wednesday
	weedayJob, _ := s.Every(1).Weekday(wednesday).Do(task)
	nonWeekdayJob, _ := s.Every(1).Minute().Do(task)

	testCases := []struct {
		desc            string
		job             *Job
		expectedWeekday time.Weekday
		expectedError   error
	}{
		{"success", weedayJob, wednesday, nil},
		{"fail - not set for weekday", nonWeekdayJob, time.Sunday, ErrNotScheduledWeekday},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			weekday, err := tc.job.Weekday()
			if tc.expectedError != nil {
				assert.Error(t, tc.expectedError, err)
			} else {
				assert.Equal(t, tc.expectedWeekday, weekday)
				assert.Nil(t, err)
			}
		})
	}
}

func TestJob_shouldRunAgain(t *testing.T) {
	tests := []struct {
		name      string
		runConfig runConfig
		runCount  int
		want      bool
	}{
		{
			name:      "should run again (infinite)",
			runConfig: runConfig{finiteRuns: false},
			want:      true,
		},
		{
			name:      "should run again (finite)",
			runConfig: runConfig{finiteRuns: true, maxRuns: 2},
			runCount:  1,
			want:      true,
		},
		{
			name:      "shouldn't run again #1",
			runConfig: runConfig{finiteRuns: true, maxRuns: 2},
			runCount:  2,
			want:      false,
		},
		{
			name:      "shouldn't run again #2",
			runConfig: runConfig{finiteRuns: true, maxRuns: 2},
			runCount:  4,
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := &Job{
				jobFunction: jobFunction{
					runConfig: tt.runConfig,
				},
				runCount: tt.runCount,
			}
			if got := j.shouldRun(); got != tt.want {
				t.Errorf("Job.shouldRunAgain() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJob_LimitRunsTo(t *testing.T) {
	j, _ := NewScheduler(time.Local).Every(1).Second().Do(func() {})
	j.LimitRunsTo(2)
	assert.Equal(t, j.shouldRun(), true, "Expecting it to run again")
	j.runCount++
	assert.Equal(t, j.shouldRun(), true, "Expecting it to run again")
	j.runCount++
	assert.Equal(t, j.shouldRun(), false, "Not expecting it to run again")
}

func TestJob_CommonExports(t *testing.T) {
	s := NewScheduler(time.Local)
	j, _ := s.Every(1).Second().Do(func() {})
	assert.Equal(t, 0, j.RunCount())
	assert.True(t, j.LastRun().IsZero())
	assert.True(t, j.NextRun().IsZero())

	s.StartAsync()
	assert.False(t, j.NextRun().IsZero())

	j.runCount = 5
	assert.Equal(t, 5, j.RunCount())

	lastRun := time.Now()
	j.lastRun = lastRun
	assert.Equal(t, lastRun, j.LastRun())
}
