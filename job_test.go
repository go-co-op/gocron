package gocron

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
	j, _ := NewScheduler(time.UTC).Every(1).Minute().At("10:30").Do(task)
	assert.Equal(t, "10:30", j.ScheduledAtTime())
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
				runConfig: tt.runConfig,
				runCount:  tt.runCount,
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
	j.run()
	assert.Equal(t, j.shouldRun(), true, "Expecting it to run again")
	j.run()
	assert.Equal(t, j.shouldRun(), false, "Not expecting it to run again")
}
