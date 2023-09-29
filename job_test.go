package gocron

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
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
			runCount := atomic.NewInt64(0)
			runCount.Store(int64(tt.runCount))
			j := &Job{
				mu: &jobMutex{},
				jobFunction: jobFunction{
					runConfig:     tt.runConfig,
					runStartCount: runCount,
				},
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
	j.runStartCount.Add(1)
	assert.Equal(t, j.shouldRun(), true, "Expecting it to run again")
	j.runStartCount.Add(1)
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

	j.runStartCount.Store(5)
	assert.Equal(t, 5, j.RunCount())

	lastRun := time.Now()
	j.mu.Lock()
	j.lastRun = lastRun
	j.mu.Unlock()
	assert.Equal(t, lastRun, j.LastRun())
}

func TestJob_GetName(t *testing.T) {
	s := NewScheduler(time.Local)
	j1, _ := s.Every(1).Second().Name("one").Do(func() {})
	assert.Equal(t, "one", j1.GetName())

	j2, _ := s.Every(1).Second().Do(func() {})
	j2.Name("two")
	assert.Equal(t, "two", j2.GetName())

	j3, _ := s.Every(1).Second().Do(func() {})
	assert.Contains(t, j3.GetName(), "func3")
}

func TestJob_SetEventListeners(t *testing.T) {
	t.Run("run event listeners callbacks for a job", func(t *testing.T) {
		var (
			jobRanPassed         bool
			beforeCallbackPassed bool
			afterCallbackPassed  bool
			beforeJobCallback    bool
			afterJobCallback     bool
			onErrorCallback      bool
			noErrorCallback      bool
			wg                   = &sync.WaitGroup{}
		)
		wg.Add(1)
		s := NewScheduler(time.UTC)
		job, err := s.Tag("tag1").Every("100ms").Do(func() {
			jobRanPassed = true
		})
		require.NoError(t, err)
		job.SetEventListeners(func() {
			beforeCallbackPassed = true
		}, func() {
			defer wg.Done()
			afterCallbackPassed = true
		})

		job2, err := s.Every("100ms").Do(func() error { return fmt.Errorf("failed") })
		require.NoError(t, err)
		wg.Add(1)
		job2.RegisterEventListeners(
			AfterJobRuns(func(_ string) {
				afterJobCallback = true
				wg.Done()
			}),
			BeforeJobRuns(func(_ string) {
				beforeJobCallback = true
			}),
			WhenJobReturnsError(func(_ string, _ error) {
				onErrorCallback = true
			}),
		)

		job3, err := s.Every("100ms").Do(func() {})
		require.NoError(t, err)
		wg.Add(1)
		job3.RegisterEventListeners(
			WhenJobReturnsNoError(func(_ string) {
				noErrorCallback = true
				wg.Done()
			}),
		)

		s.StartAsync()
		wg.Wait()
		s.Stop()

		require.NoError(t, err)
		assert.True(t, jobRanPassed)
		assert.True(t, beforeCallbackPassed)
		assert.True(t, afterCallbackPassed)
		assert.True(t, beforeJobCallback)
		assert.True(t, afterJobCallback)
		assert.True(t, onErrorCallback)
		assert.True(t, noErrorCallback)
	})
}

func TestJob_Context(t *testing.T) {
	t.Run("Context returns the job's context", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		job, err := s.Tag("tag1").Every("100ms").Do(func() {})
		require.NoError(t, err)
		assert.NotNil(t, job.Context())
		assert.Equal(t, job.ctx, job.Context())
	})
}

func TestJob_Stop(t *testing.T) {
	t.Run("stop calls the cancel function", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		job, err := s.Tag("tag1").Every("100ms").Do(func() {})
		require.NoError(t, err)

		jobCtx := job.ctx
		assert.Nil(t, jobCtx.Err())

		job.stop()
		assert.True(t, errors.Is(jobCtx.Err(), context.Canceled))
	})
}

func TestJob_PreviousRun(t *testing.T) {
	t.Run("at time", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		atTime := time.Now().UTC().Add(time.Millisecond * 150)
		_, err := s.Every(1).Day().At(atTime).DoWithJobDetails(func(job Job) {
			assert.Zero(t, job.PreviousRun())
			assert.Equal(t, atTime.Truncate(time.Second), job.LastRun())
		})
		require.NoError(t, err)
		s.StartAsync()
		time.Sleep(500 * time.Millisecond)
		s.Stop()
	})

	t.Run("duration", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		var counter int
		var lastRunTime time.Time
		var mu sync.Mutex
		_, err := s.Every("250ms").DoWithJobDetails(func(job Job) {
			mu.Lock()
			defer mu.Unlock()
			if counter == 0 {
				assert.Zero(t, job.PreviousRun())
				lastRunTime = job.LastRun()
			} else {
				assert.Equalf(t, lastRunTime, job.PreviousRun(), "last lastRun and previous")
			}
			counter++
		})
		require.NoError(t, err)
		s.StartAsync()
		time.Sleep(450 * time.Millisecond)
		s.Stop()
	})
}
