package gocron

import (
	"sync"
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

func TestHasTags(t *testing.T) {
	tests := []struct {
		name      string
		jobTags   []string
		matchTags []string
		expected  bool
	}{
		{
			"OneTagMatch",
			[]string{"tag1"},
			[]string{"tag1"},
			true,
		},
		{
			"OneTagNoMatch",
			[]string{"tag1"},
			[]string{"tag2"},
			false,
		},
		{
			"DuplicateJobTagsMatch",
			[]string{"tag1", "tag1"},
			[]string{"tag1"},
			true,
		},
		{
			"DuplicateInputTagsMatch",
			[]string{"tag1"},
			[]string{"tag1", "tag1"},
			true,
		},
		{
			"MultipleTagsMatch",
			[]string{"tag1", "tag2"},
			[]string{"tag2", "tag1"},
			true,
		},
		{
			"MultipleTagsNoMatch",
			[]string{"tag1", "tag2"},
			[]string{"tag2", "tag1", "tag3"},
			false,
		},
		{
			"MultipleDuplicateTagsMatch",
			[]string{"tag1", "tag1", "tag1", "tag2"},
			[]string{"tag1", "tag2"},
			true,
		},
		{
			"MultipleDuplicateTagsNoMatch",
			[]string{"tag1", "tag1", "tag1"},
			[]string{"tag1", "tag1", "tag3"},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j, _ := NewScheduler(time.UTC).Every(1).Minute().Do(task)
			j.Tag(tt.jobTags...)
			assert.Equal(t, tt.expected, j.hasTags(tt.matchTags...))
		})
	}
}

func TestJob_IsRunning(t *testing.T) {
	s := NewScheduler(time.UTC)
	j, err := s.Every(10).Seconds().Do(func() { time.Sleep(2 * time.Second) })
	require.NoError(t, err)
	assert.False(t, j.IsRunning())

	s.StartAsync()

	time.Sleep(time.Second)
	assert.True(t, j.IsRunning())

	time.Sleep(time.Second)
	s.Stop()

	assert.False(t, j.IsRunning())
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
				mu: &jobMutex{},
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
	s.Stop()

	assert.False(t, j.NextRun().IsZero())

	j.runCount = 5
	assert.Equal(t, 5, j.RunCount())

	lastRun := time.Now()
	j.mu.Lock()
	j.lastRun = lastRun
	j.mu.Unlock()
	assert.Equal(t, lastRun, j.LastRun())
}

func TestJob_SetEventListeners(t *testing.T) {
	t.Run("run event listeners callbacks for a job", func(t *testing.T) {
		var (
			jobRanPassed         = false
			beforeCallbackPassed = false
			afterCallbackPassed  = false
			wg                   = &sync.WaitGroup{}
		)
		wg.Add(1)
		s := NewScheduler(time.UTC)
		job, err := s.Tag("tag1").Every("1ms").Do(func() {
			jobRanPassed = true
		})
		job.SetEventListeners(func() {
			beforeCallbackPassed = true
		}, func() {
			defer wg.Done()
			afterCallbackPassed = true
		})

		s.StartAsync()
		s.stop()
		wg.Wait()

		require.NoError(t, err)
		assert.True(t, jobRanPassed)
		assert.True(t, beforeCallbackPassed)
		assert.True(t, afterCallbackPassed)
	})
}
