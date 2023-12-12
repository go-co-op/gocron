package gocron

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

var _ TimeWrapper = (*fakeTime)(nil)

type fakeTime struct {
	onNow func(location *time.Location) time.Time
}

func (f fakeTime) Now(loc *time.Location) time.Time {
	return f.onNow(loc)
}

func (f fakeTime) Unix(sec int64, nsec int64) time.Time {
	return time.Unix(sec, nsec)
}

func (f fakeTime) Sleep(d time.Duration) {
	time.Sleep(d)
}

func task() {
	fmt.Println("I am a running job.")
}

func taskWithParams(a int, b string) {
	fmt.Println(a, b)
}

func TestImmediateExecution(t *testing.T) {
	s := NewScheduler(time.UTC)
	semaphore := make(chan bool)
	_, err := s.Every(1).Second().Do(func() {
		semaphore <- true
	})
	require.NoError(t, err)
	s.StartAsync()
	select {
	case <-time.After(time.Second):
		s.stop()
		t.Fatal("job did not run immediately")
	case <-semaphore:
		// test passed
		s.stop()
	}
}

func TestExecutionWithPointerToFunc(t *testing.T) {
	s := NewScheduler(time.UTC)
	semaphore := make(chan bool)
	fn := func() { semaphore <- true }

	_, err := s.Every(1).Second().Do(&fn)
	require.NoError(t, err)
	s.StartAsync()
	select {
	case <-time.After(time.Second):
		s.stop()
		t.Fatal("job did not run immediately")
	case <-semaphore:
		// test passed
		s.stop()
	}
}

func TestScheduler_Every_InvalidInterval(t *testing.T) {
	testCases := []struct {
		description   string
		interval      interface{}
		expectedError string
	}{
		{"zero int", 0, ErrInvalidInterval.Error()},
		{"negative int", -1, ErrInvalidInterval.Error()},
		{"negative time.Duration", -1 * time.Millisecond, ErrInvalidInterval.Error()},
		{"negative string duration", "-1ms", ErrInvalidInterval.Error()},
		{"invalid string duration", "bad", "time: invalid duration \"bad\""},
	}

	s := NewScheduler(time.UTC)

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			_, err := s.Every(tc.interval).Do(func() {})
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.expectedError)
		})
	}
}

func TestScheduler_EveryRandom(t *testing.T) {
	s := NewScheduler(time.UTC)
	semaphore := make(chan bool)

	j, err := s.EveryRandom(50, 100).Milliseconds().Do(func() {
		semaphore <- true
	})
	require.NoError(t, err)
	assert.True(t, j.randomizeInterval)

	s.StartAsync()

	var counter int

	now := time.Now()
	for time.Now().Before(now.Add(1 * time.Second)) {
		if <-semaphore {
			counter++
		}
	}
	s.Stop()
	assert.LessOrEqual(t, counter, 20)
	assert.GreaterOrEqual(t, counter, 10)
}

func TestScheduler_Every(t *testing.T) {
	t.Run("time.Duration", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		semaphore := make(chan bool)

		_, err := s.Every(100 * time.Millisecond).Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)

		s.StartAsync()

		var counter int

		now := time.Now()
		for time.Now().Before(now.Add(500 * time.Millisecond)) {
			if <-semaphore {
				counter++
			}
		}
		s.Stop()
		assert.Equal(t, 6, counter)
	})

	t.Run("int", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		semaphore := make(chan bool)

		_, err := s.Every(100).Milliseconds().Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)

		s.StartAsync()

		var counter int

		now := time.Now()
		for time.Now().Before(now.Add(100 * time.Millisecond)) {
			if <-semaphore {
				counter++
			}
		}
		s.Stop()
		assert.GreaterOrEqual(t, counter, 2)
		assert.LessOrEqual(t, counter, 3)
	})

	t.Run("string duration", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		semaphore := make(chan bool)

		_, err := s.Every("100ms").Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)

		s.StartAsync()

		var counter int

		now := time.Now()
		for time.Now().Before(now.Add(100 * time.Millisecond)) {
			if <-semaphore {
				counter++
			}
		}
		s.Stop()
		assert.Equal(t, 2, counter)
	})
}

func TestExecutionSeconds(t *testing.T) {
	s := NewScheduler(time.UTC)
	jobDone := make(chan bool)

	var (
		executions         []int64
		interval           = 1
		expectedExecutions = 2
		mu                 sync.RWMutex
	)

	runTime := time.Second
	startTime := time.Now()

	// default unit is seconds
	_, err := s.Every(interval).Do(func() {
		mu.Lock()
		defer mu.Unlock()
		executions = append(executions, time.Now().UTC().Unix())
		if time.Now().After(startTime.Add(runTime)) {
			jobDone <- true
		}
	})
	require.NoError(t, err)

	s.StartAsync()
	<-jobDone // Wait job done
	s.Stop()

	mu.RLock()
	defer mu.RUnlock()
	assert.Equal(t, expectedExecutions, len(executions), "did not run expected number of times")

	for i := 1; i < expectedExecutions; i++ {
		durationBetweenExecutions := executions[i] - executions[i-1]
		assert.Equal(t, int64(interval), durationBetweenExecutions, "duration between tasks does not correspond to expectations")
	}
}

func TestScheduled(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		_, err := s.Every(1).Second().Do(task)
		require.NoError(t, err)
		assert.True(t, s.TaskPresent(task))
	})

	t.Run("with tag", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		_, err := s.Every(1).Hour().Tag("my_custom_tag").Do(task)
		require.NoError(t, err)
		assert.True(t, s.TaskPresent(task))
	})
}

func TestAt(t *testing.T) {
	t.Run("job scheduled for future hasn't run yet", func(t *testing.T) {
		ft := fakeTime{onNow: func(l *time.Location) time.Time {
			return time.Date(1970, 1, 1, 12, 0, 0, 0, l)
		}}

		s := NewScheduler(time.UTC)
		s.time = ft
		now := ft.onNow(time.UTC)
		semaphore := make(chan bool)

		nextMinuteTime := now.Add(time.Minute)
		startAt := fmt.Sprintf("%02d:%02d:%02d", nextMinuteTime.Hour(), nextMinuteTime.Minute(), nextMinuteTime.Second())
		dayJob, err := s.Every(1).Day().At(startAt).Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)

		s.StartAsync()
		time.Sleep(time.Second)

		select {
		case <-time.After(time.Second):
			s.stop()
			assert.Equal(t, now.Add(1*time.Minute), dayJob.nextRun)
		case <-semaphore:
			s.stop()
			t.Fatal("job ran even though scheduled in future")
		}
		time.Sleep(2 * time.Second)
	})

	t.Run("error due to bad time format", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		badTime := "0:0"
		_, err := s.Every(1).Day().At(badTime).Do(func() {})
		assert.EqualError(t, err, ErrUnsupportedTimeFormat.Error())
		assert.Zero(t, s.Len())
	})

	testTime := time.Date(2000, 1, 10, 12, 0, 0, 0, time.UTC)
	ft := fakeTime{onNow: func(l *time.Location) time.Time {
		return testTime
	}}
	t.Run("Week() and At()", func(t *testing.T) {
		atTime := testTime.Add(time.Hour).Round(time.Second)

		s := NewScheduler(time.UTC)
		s.time = ft
		job, err := s.Every(1).Week().At(atTime).Do(func() {})
		require.NoError(t, err)
		s.StartAsync()

		assert.Equal(t, atTime, job.NextRun())
		s.Stop()
	})

	t.Run("Week() and At() almost 1 week in future", func(t *testing.T) {
		atTime := testTime.Add(time.Hour * 167).Round(time.Second)

		s := NewScheduler(time.UTC)
		s.time = ft
		job, err := s.Every(1).Week().At(atTime).Do(func() {})
		require.NoError(t, err)
		s.StartAsync()

		assert.Equal(t, atTime, job.NextRun())
		s.Stop()
	})

	t.Run("Week() and multiple At() times, 1 in future", func(t *testing.T) {
		atTime1 := testTime.Add(time.Hour * -2).Round(time.Second)
		atTime2 := testTime.Add(time.Hour * -1).Round(time.Second)
		atTime3 := testTime.Add(time.Hour * 1).Round(time.Second)

		s := NewScheduler(time.UTC)
		s.time = ft
		job, err := s.Every(1).Week().At(atTime1).At(atTime2).At(atTime3).Do(func() {})
		require.NoError(t, err)
		s.StartAsync()

		assert.Equal(t, atTime3, job.NextRun())
		s.Stop()
	})

	t.Run("Week() and multiple At() times, all in past", func(t *testing.T) {
		atTime1 := testTime.Add(time.Hour * -6).Round(time.Second)
		atTime2 := testTime.Add(time.Hour * -5).Round(time.Second)
		atTime3 := testTime.Add(time.Hour * -4).Round(time.Second)

		s := NewScheduler(time.UTC)
		s.time = ft

		job, err := s.Week().At(atTime1).At(atTime2).At(atTime3).Every(1).Do(func() {})
		require.NoError(t, err)
		s.StartAsync()

		assert.Equal(t, atTime1.Add(time.Hour*168), job.NextRun())
		s.Stop()
	})
}

func TestMultipleAtTimesDecoding(t *testing.T) {
	exp := []time.Duration{_getHours(1), _getHours(3), _getHours(4), _getHours(7), _getHours(15)}
	testCases := []struct {
		name   string
		params []interface{}
		result []time.Duration
	}{
		{
			name:   "multiple simple strings",
			params: []interface{}{"03:00", "15:00", "01:00", "07:00", "04:00"},
			result: exp,
		},
		{
			name:   "single string separated by semicolons",
			params: []interface{}{"03:00;15:00;01:00;07:00;04:00"},
			result: exp,
		},
		{
			name:   "interpolation of semicolons string, time.Time and simple string",
			params: []interface{}{"03:00;15:00;01:00", time.Date(0, 0, 0, 7, 0, 0, 0, time.UTC), "04:00"},
			result: exp,
		},
		{
			name:   "repeated values on input don't get duplicated after decoding",
			params: []interface{}{"03:00;15:00;01:00;07:00;04:00;01:00"},
			result: exp,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewScheduler(time.UTC)
			for _, p := range tc.params {
				s.At(p)
			}

			got := s.getCurrentJob().atTimes
			assert.Equalf(t, tc.result, got, fmt.Sprintf("expected %v / got %v", tc.result, got))
		})
	}
}

func schedulerForNextOrPreviousWeekdayEveryNTimes(weekday time.Weekday, next bool, n int, s *Scheduler) *Scheduler {
	switch weekday {
	case time.Monday:
		if next {
			s = s.Every(n).Tuesday()
		} else {
			s = s.Every(n).Sunday()
		}
	case time.Tuesday:
		if next {
			s = s.Every(n).Wednesday()
		} else {
			s = s.Every(n).Monday()
		}
	case time.Wednesday:
		if next {
			s = s.Every(n).Thursday()
		} else {
			s = s.Every(n).Tuesday()
		}
	case time.Thursday:
		if next {
			s = s.Every(n).Friday()
		} else {
			s = s.Every(n).Wednesday()
		}
	case time.Friday:
		if next {
			s = s.Every(n).Saturday()
		} else {
			s = s.Every(n).Thursday()
		}
	case time.Saturday:
		if next {
			s = s.Every(n).Sunday()
		} else {
			s = s.Every(n).Friday()
		}
	case time.Sunday:
		if next {
			s = s.Every(n).Monday()
		} else {
			s = s.Every(n).Saturday()
		}
	}
	return s
}

func TestWeekdayBeforeToday(t *testing.T) {
	now := time.Now().In(time.UTC)
	s := NewScheduler(time.UTC)

	s = schedulerForNextOrPreviousWeekdayEveryNTimes(now.Weekday(), false, 1, s)
	weekJob, err := s.Do(task)
	require.NoError(t, err)
	s.scheduleNextRun(weekJob)
	sixDaysFromNow := now.AddDate(0, 0, 6)

	exp := time.Date(sixDaysFromNow.Year(), sixDaysFromNow.Month(), sixDaysFromNow.Day(), 0, 0, 0, 0, time.UTC)
	assert.Equal(t, exp, weekJob.nextRun)
}

func TestWeekdayAt(t *testing.T) {
	t.Run("asserts weekday scheduling starts at the current week", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		now := time.Now().UTC()
		s = schedulerForNextOrPreviousWeekdayEveryNTimes(now.Weekday(), true, 1, s)
		weekdayJob, _ := s.Do(task)

		s.scheduleNextRun(weekdayJob)

		tomorrow := now.AddDate(0, 0, 1)
		exp := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, time.UTC)
		nextRun := weekdayJob.nextRun
		nextRunDate := time.Date(nextRun.Year(), nextRun.Month(), nextRun.Day(), 0, 0, 0, 0, time.UTC)
		assert.Equal(t, exp, nextRunDate)
	})
}

func TestDaylightSavingsScheduled(t *testing.T) {
	loc, err := time.LoadLocation("US/Pacific")
	assert.NoError(t, err)

	fromRFC3339 := func(ts string) time.Time {
		dt, err := time.Parse(time.RFC3339, ts)
		assert.NoError(t, err)
		return dt.In(loc)
	}

	beforeToEDT := fakeTime{onNow: func(l *time.Location) time.Time {
		return time.Date(2023, 3, 12, 1, 50, 50, 0, loc)
	}}
	toEDT := fakeTime{onNow: func(l *time.Location) time.Time {
		return time.Date(2023, 3, 12, 5, 0, 0, 0, loc)
	}}
	beforeToEST := fakeTime{onNow: func(l *time.Location) time.Time {
		return time.Date(2022, 11, 6, 1, 50, 50, 0, loc)
	}}
	afterToEST := fakeTime{onNow: func(l *time.Location) time.Time {
		return fromRFC3339("2022-11-06T09:00:01.000Z")
	}}
	toEST := fakeTime{onNow: func(l *time.Location) time.Time {
		return time.Date(2022, 11, 6, 4, 0, 0, 0, loc)
	}}

	testCases := []struct {
		description     string
		ft              fakeTime
		jobCreateFn     func(s *Scheduler) *Scheduler
		expectedNextRun time.Time
	}{
		{"EST no change", beforeToEDT, func(s *Scheduler) *Scheduler { return s.Every(1).Day().At("01:59") }, time.Date(2023, 3, 12, 1, 59, 0, 0, loc)},
		{"EST->EDT every day", toEDT, func(s *Scheduler) *Scheduler { return s.Every(1).Day().At("20:00") }, time.Date(2023, 3, 12, 20, 0, 0, 0, loc)},
		{"EST->EDT every 2 week", toEDT, func(s *Scheduler) *Scheduler { return s.Every(2).Week().At("17:00") }, time.Date(2023, 3, 12, 17, 0, 0, 0, loc)},
		{"EST->EDT every 2 Tuesday", toEDT, func(s *Scheduler) *Scheduler { return s.Every(2).Tuesday().At("16:00") }, time.Date(2023, 3, 14, 16, 0, 0, 0, loc)},
		{"EST->EDT every Sunday", toEDT, func(s *Scheduler) *Scheduler { return s.Every(1).Sunday().At("04:30") }, time.Date(2023, 3, 19, 4, 30, 0, 0, loc)},
		{"EST->EDT every month", toEDT, func(s *Scheduler) *Scheduler { return s.Every(3).Month(12).At("14:00") }, time.Date(2023, 6, 12, 14, 0, 0, 0, loc)},
		{"EST->EDT every last day of month", toEDT, func(s *Scheduler) *Scheduler { return s.Every(1).MonthLastDay().At("13:00") }, time.Date(2023, 3, 31, 13, 0, 0, 0, loc)},

		{"EDT no change", beforeToEST, func(s *Scheduler) *Scheduler { return s.Every(1).Day().At("01:59") }, time.Date(2022, 11, 6, 1, 59, 0, 0, loc)},
		{"EDT->EST every day", toEST, func(s *Scheduler) *Scheduler { return s.Every(1).Day().At("20:00") }, time.Date(2022, 11, 6, 20, 0, 0, 0, loc)},
		{"EDT->EST every 2 week", toEST, func(s *Scheduler) *Scheduler { return s.Every(2).Week().At("18:00") }, time.Date(2022, 11, 6, 18, 0, 0, 0, loc)},
		{"EDT->EST every 2 Tuesday", toEST, func(s *Scheduler) *Scheduler { return s.Every(2).Tuesday().At("15:00") }, time.Date(2022, 11, 8, 15, 0, 0, 0, loc)},
		{"EDT->EST every Sunday", afterToEST, func(s *Scheduler) *Scheduler { return s.Every(1).Sunday().At("01:30") }, time.Date(2022, 11, 13, 1, 30, 0, 0, loc)},
		{"EDT->EST every month", toEST, func(s *Scheduler) *Scheduler { return s.Every(3).Month(6).At("14:30") }, time.Date(2023, 2, 6, 14, 30, 0, 0, loc)},
		{"EDT->EST every last day of month", toEST, func(s *Scheduler) *Scheduler { return s.Every(1).MonthLastDay().At("13:15") }, time.Date(2022, 11, 30, 13, 15, 0, 0, loc)},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s := NewScheduler(loc)
			s.time = tc.ft

			job, err := tc.jobCreateFn(s).Do(func() {})
			assert.NoError(t, err)

			s.setRunning(true)
			s.scheduleNextRun(job)

			assert.Equal(t, tc.expectedNextRun, job.NextRun())
		})
	}
}

func TestScheduler_Remove(t *testing.T) {
	t.Run("remove from non-running", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		s.TagsUnique()
		_, err := s.Minute().Tag("tag1").Every(1).Do(task)
		require.NoError(t, err)
		_, err = s.Every(1).Minute().Do(taskWithParams, 1, "hello")
		require.NoError(t, err)
		_, err = s.Minute().Every(1).Do(task)
		require.NoError(t, err)

		assert.Equal(t, 3, s.Len(), "Incorrect number of jobs")

		s.Remove(task)
		assert.Equal(t, 1, s.Len(), "Incorrect number of jobs after removing 2 job")

		s.Remove(task)
		assert.Equal(t, 1, s.Len(), "Incorrect number of jobs after removing non-existent job")
	})

	t.Run("remove from running scheduler", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		semaphore := make(chan bool)

		j, err := s.Every("100ms").StartAt(s.time.Now(s.location).Add(time.Second)).Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)

		s.StartAsync()

		s.Remove(j.function)

		select {
		case <-time.After(200 * time.Millisecond):
			// test passed
		case <-semaphore:
			t.Fatal("job ran after being removed")
		}
	})
}

func TestScheduler_RemoveByReference(t *testing.T) {
	t.Run("remove from non-running scheduler", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		job1, _ := s.Every(1).Minute().Do(task)
		job2, _ := s.Every(1).Minute().Do(taskWithParams, 1, "hello")

		assert.Equal(t, 2, s.Len(), "Incorrect number of jobs")

		_ = s.RemoveByID(job1)
		s.jobsMutex.RLock()
		defer s.jobsMutex.RUnlock()
		assert.NotContains(t, s.jobs, job1.id)
		assert.Contains(t, s.jobs, job2.id)
	})

	t.Run("remove from running scheduler", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		s.TagsUnique()
		semaphore := make(chan bool)

		j, err := s.Every("100ms").StartAt(s.time.Now(s.location).Add(100 * time.Millisecond)).Tag("tag1").Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)

		s.StartAsync()

		_ = s.RemoveByID(j)

		select {
		case <-time.After(200 * time.Millisecond):
			// test passed
		case <-semaphore:
			t.Fatal("job ran after being removed")
		}

		_, ok := s.tags.Load("tag1")
		assert.False(t, ok)
	})
}

func TestScheduler_RemoveByTags(t *testing.T) {
	t.Run("non unique tags", func(t *testing.T) {
		s := NewScheduler(time.UTC)

		// Creating 2 jobs with different tags
		tag1 := "a"
		tag2 := "ab"
		j1, err := s.Every(1).Second().Tag(tag1).Do(taskWithParams, 1, "hello") // index 0
		require.NoError(t, err)
		j2, err := s.Every(1).Second().Tag(tag2).Do(taskWithParams, 2, "world") // index 1
		require.NoError(t, err)

		s.jobsMutex.RLock()
		// check j1 tags is equal with tag "a" (tag1)
		assert.Equal(t, s.jobs[j1.id].Tags()[0], tag1, "Job With Tag 'a' is removed from index 0")
		s.jobsMutex.RUnlock()

		err = s.RemoveByTags(tag1)
		require.NoError(t, err)
		assert.Equal(t, 1, s.Len(), "Incorrect number of jobs after removing 1 job")

		s.jobsMutex.RLock()
		// check j2 tags is equal with tag "tag two" (tag2) after removing "a"
		assert.Equal(t, s.jobs[j2.id].Tags()[0], tag2, "Job With Tag 'tag two' is removed from index 0")
		s.jobsMutex.RUnlock()

		// Removing Non-Existent Job with "a" because already removed above (will not removing any jobs because tag not match)
		err = s.RemoveByTags(tag1)
		assert.EqualError(t, err, ErrJobNotFoundWithTag.Error())
	})

	t.Run("unique tags", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		s.TagsUnique()

		// Creating 2 jobs with unique tags
		tag1 := "tag one"
		tag2 := "tag two"
		_, err := s.Every(1).Second().Tag(tag1).Do(taskWithParams, 1, "hello") // index 0
		require.NoError(t, err)
		_, err = s.Every(1).Second().Tag(tag2).Do(taskWithParams, 2, "world") // index 1
		require.NoError(t, err)

		err = s.RemoveByTags("tag one")
		require.NoError(t, err)

		// Adding job with tag after removing by tag, assuming the unique tag has been removed as well
		_, err = s.Every(1).Second().Tag(tag1).Do(taskWithParams, 1, "hello")
		assert.Nil(t, err, "Unique tag is not deleted when removing by tag")
	})

	t.Run("multiple non unique tags", func(t *testing.T) {
		s := NewScheduler(time.UTC)

		// Creating 2 jobs with different tags
		tag1 := "a"
		tag2 := "ab"
		tag3 := "abc"
		j1, err := s.Every(1).Second().Tag(tag1, tag3).Do(taskWithParams, 1, "hello") // index 0
		require.NoError(t, err)
		j2, err := s.Every(1).Second().Tag(tag1, tag2).Do(taskWithParams, 2, "world") // index 1
		require.NoError(t, err)

		// check j1 tags contains tag "a" (tag1) and "abc" (tag3)
		s.jobsMutex.RLock()
		assert.Contains(t, s.jobs[j1.id].Tags(), tag1, "Job With Tag 'a' is removed from index 0")
		assert.Contains(t, s.jobs[j1.id].Tags(), tag3, "Job With Tag 'abc' is removed from index 0")
		s.jobsMutex.RUnlock()

		err = s.RemoveByTags(tag1, tag3)
		require.NoError(t, err)
		assert.Equal(t, 1, s.Len(), "Incorrect number of jobs after removing 1 job")

		// check j2 tags is equal with tag "a" (tag1) and "ab" (tag2) after removing "a"+"abc"
		s.jobsMutex.RLock()
		assert.Contains(t, s.jobs[j2.id].Tags(), tag1, "Job With Tag 'a' is removed from index 0")
		assert.Contains(t, s.jobs[j2.id].Tags(), tag2, "Job With Tag 'ab' is removed from index 0")
		s.jobsMutex.RUnlock()

		// Removing Non-Existent Job with "a"+"abc" because already removed above (will not removing any jobs because tag not match)
		err = s.RemoveByTags(tag1, tag3)
		assert.EqualError(t, err, ErrJobNotFoundWithTag.Error())
	})

	t.Run("multiple unique tags", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		s.TagsUnique()

		// Creating 2 jobs with unique tags
		tag1 := "tag one"
		tag2 := "tag two"
		tag3 := "tag three"
		_, err := s.Every(1).Second().Tag(tag1, tag3).Do(taskWithParams, 1, "hello") // index 0
		require.NoError(t, err)
		_, err = s.Every(1).Second().Tag(tag2).Do(taskWithParams, 2, "world") // index 1
		require.NoError(t, err)

		err = s.RemoveByTags(tag1, tag3)
		require.NoError(t, err)

		// Adding job with tag after removing by tag, assuming the unique tag has been removed as well
		_, err = s.Every(1).Second().Tag(tag1).Do(taskWithParams, 1, "hello")
		assert.Nil(t, err, "Unique tag is not deleted when removing by tag")
	})
}

func TestScheduler_RemoveByTagsAny(t *testing.T) {
	t.Run("non unique tags", func(t *testing.T) {
		s := NewScheduler(time.UTC)

		// Creating 2 jobs with different tags
		tag1 := "a"
		tag2 := "ab"
		j1, err := s.Every(1).Second().Tag(tag1).Do(taskWithParams, 1, "hello") // index 0
		require.NoError(t, err)
		_, err = s.Every(1).Second().Tag(tag2).Do(taskWithParams, 2, "world") // index 1
		require.NoError(t, err)

		// check j1 tags is equal with tag "a" (tag1)
		s.jobsMutex.RLock()
		assert.Equal(t, s.jobs[j1.id].Tags()[0], tag1, "Job With Tag 'a' is removed from index 0")
		s.jobsMutex.RUnlock()

		err = s.RemoveByTagsAny(tag1, tag2)
		require.NoError(t, err)
		assert.Equal(t, 0, s.Len(), "Incorrect number of jobs after removing 1 job")

		// Removing Non-Existent Job with "a" because already removed above (will not removing any jobs because tag not match)
		err = s.RemoveByTagsAny(tag1)
		assert.EqualError(t, err, ErrJobNotFoundWithTag.Error()+": "+tag1)
	})

	t.Run("unique tags", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		s.TagsUnique()

		// Creating 2 jobs with unique tags
		tag1 := "tag one"
		tag2 := "tag two"
		_, err := s.Every(1).Second().Tag(tag1).Do(taskWithParams, 1, "hello") // index 0
		require.NoError(t, err)
		_, err = s.Every(1).Second().Tag(tag2).Do(taskWithParams, 2, "world") // index 1
		require.NoError(t, err)

		err = s.RemoveByTagsAny(tag1, tag2)
		require.NoError(t, err)

		// Adding job with tag after removing by tag, assuming the unique tag has been removed as well
		_, err = s.Every(1).Second().Tag(tag1).Do(taskWithParams, 1, "hello")
		assert.Nil(t, err, "Unique tag is not deleted when removing by tag")
	})

	t.Run("multiple non unique tags", func(t *testing.T) {
		s := NewScheduler(time.UTC)

		// Creating 2 jobs with different tags
		tag1 := "a"
		tag2 := "ab"
		tag3 := "abc"
		j1, err := s.Every(1).Second().Tag(tag1, tag3).Do(taskWithParams, 1, "hello") // index 0
		require.NoError(t, err)
		_, err = s.Every(1).Second().Tag(tag1, tag2).Do(taskWithParams, 2, "world") // index 1
		require.NoError(t, err)

		// check j1 tags contains tag "a" (tag1) and "abc" (tag3)
		s.jobsMutex.RLock()
		assert.Contains(t, s.jobs[j1.id].Tags(), tag1, "Job With Tag 'a' is removed from index 0")
		assert.Contains(t, s.jobs[j1.id].Tags(), tag3, "Job With Tag 'abc' is removed from index 0")
		s.jobsMutex.RUnlock()

		err = s.RemoveByTagsAny(tag1, tag2, tag3)
		require.NoError(t, err)
		assert.Equal(t, 0, s.Len(), "Incorrect number of jobs after removing 1 job")

		// Removing Non-Existent Job with "a"+"abc" because already removed above (will not removing any jobs because tag not match)
		err = s.RemoveByTagsAny(tag1, tag3)
		assert.EqualError(t, err, ErrJobNotFoundWithTag.Error()+": "+tag3+": "+ErrJobNotFoundWithTag.Error()+": "+tag1)
	})

	t.Run("multiple unique tags", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		s.TagsUnique()

		// Creating 2 jobs with unique tags
		tag1 := "tag one"
		tag2 := "tag two"
		tag3 := "tag three"
		_, err := s.Every(1).Second().Tag(tag1, tag3).Do(taskWithParams, 1, "hello") // index 0
		require.NoError(t, err)
		_, err = s.Every(1).Second().Tag(tag2).Do(taskWithParams, 2, "world") // index 1
		require.NoError(t, err)

		err = s.RemoveByTagsAny(tag1)
		require.NoError(t, err)

		// Adding job with tag after removing by tag, assuming the unique tag has been removed as well
		_, err = s.Every(1).Second().Tag(tag1).Do(taskWithParams, 1, "hello")
		assert.Nil(t, err, "Unique tag is not deleted when removing by tag")
	})
}

func TestScheduler_Jobs(t *testing.T) {
	s := NewScheduler(time.UTC)
	_, _ = s.Every(1).Minute().Do(task)
	_, _ = s.Every(2).Minutes().Do(task)
	_, _ = s.Every(3).Minutes().Do(task)
	_, _ = s.Every(4).Minutes().Do(task)

	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	assert.Len(t, s.jobs, 4)
}

func TestScheduler_Len(t *testing.T) {
	s := NewScheduler(time.UTC)
	_, _ = s.Every(1).Minute().Do(task)
	_, _ = s.Every(2).Minutes().Do(task)
	_, _ = s.Every(3).Minutes().Do(task)
	_, _ = s.Every(4).Minutes().Do(task)
	l := s.Len()

	assert.Equal(t, l, 4)
}

func TestSetLocation(t *testing.T) {
	s := NewScheduler(time.FixedZone("UTC-8", -8*60*60))

	assert.Equal(t, time.FixedZone("UTC-8", -8*60*60), s.Location())

	s.ChangeLocation(time.UTC)

	assert.Equal(t, time.UTC, s.Location())
}

func TestClear(t *testing.T) {
	s := NewScheduler(time.UTC)
	semaphore := make(chan bool)

	_, err := s.Every("100ms").Do(func() {
		semaphore <- true
	})
	require.NoError(t, err)

	s.StartAsync()

	s.Clear()
	assert.Equal(t, 0, s.Len())

	var counter int
	now := time.Now()
	for time.Now().Before(now.Add(200 * time.Millisecond)) {
		select {
		case <-semaphore:
			counter++
		default:
		}
	}

	// job should run only once - immediately and then
	// be stopped on s.Clear()
	assert.Equal(t, 1, counter)
}

func TestClearUnique(t *testing.T) {
	s := NewScheduler(time.UTC)
	s.TagsUnique()
	semaphore := make(chan bool)

	_, err := s.Every("100ms").Tag("tag1").Do(func() {
		semaphore <- true
	})
	require.NoError(t, err)

	s.StartAsync()

	s.Clear()
	assert.Equal(t, 0, s.Len())

	var counter int
	now := time.Now()
	for time.Now().Before(now.Add(200 * time.Millisecond)) {
		select {
		case <-semaphore:
			counter++
		default:
		}
	}

	// job should run only once - immediately and then
	// be stopped on s.Clear()
	assert.Equal(t, 1, counter)

	s.tags.Range(func(key, value interface{}) bool {
		assert.FailNow(t, "map should be empty")
		return true
	})
}

func TestSetUnit(t *testing.T) {
	testCases := []struct {
		desc     string
		timeUnit schedulingUnit
	}{
		{"seconds", seconds},
		{"minutes", minutes},
		{"hours", hours},
		{"days", days},
		{"weeks", weeks},
	}

	for _, tc := range testCases {
		s := NewScheduler(time.UTC)
		t.Run(tc.desc, func(t *testing.T) {
			var j *Job
			switch tc.timeUnit {
			case seconds:
				j, _ = s.Every(2).Seconds().Do(task)
			case minutes:
				j, _ = s.Every(2).Minutes().Do(task)
			case hours:
				j, _ = s.Every(2).Hours().Do(task)
			case days:
				j, _ = s.Every(2).Days().Do(task)
			case weeks:
				j, _ = s.Every(2).Weeks().Do(task)
			}

			assert.Equal(t, tc.timeUnit, j.unit)
		})
	}
}

func TestScheduler_Stop(t *testing.T) {
	t.Run("stops a running scheduler", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		s.StartAsync()
		assert.True(t, s.IsRunning())
		s.Stop()
		assert.False(t, s.IsRunning())
	})
	t.Run("stops a running scheduler calling .Stop()", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		s.StartAsync()
		assert.True(t, s.IsRunning())
		s.Stop()
		time.Sleep(time.Millisecond) // wait for stop goroutine to catch up
		assert.False(t, s.IsRunning())
	})
	t.Run("noop on stopped scheduler", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		s.Stop()
		assert.False(t, s.IsRunning())
	})
	t.Run("stops all jobs", func(t *testing.T) {
		t.Parallel()
		s := NewScheduler(time.UTC)
		job, _ := s.Every(3).Second().Do(func() {
			// noop
		})
		s.StartAsync()
		time.Sleep(time.Second) // enough time for job to run
		preStopJobTimer := job.timer
		s.Stop()
		time.Sleep(3 * time.Second) // enough time for job timer to reset
		afterStopJobTimer := job.timer

		assert.Same(t, preStopJobTimer, afterStopJobTimer)
	})
	t.Run("waits for jobs to finish processing before returning .Stop()", func(t *testing.T) {
		t.Parallel()
		i := atomic.NewInt64(0)

		s := NewScheduler(time.UTC)
		_, _ = s.Every(10).Second().Do(func() {
			time.Sleep(2 * time.Second)
			i.Add(1)
		})
		s.StartAsync()
		time.Sleep(time.Second) // enough time for job to run
		s.Stop()

		assert.EqualValues(t, 1, i.Load())
	})
	t.Run("stops a running scheduler calling .Stop()", func(t *testing.T) {
		s := NewScheduler(time.UTC)

		go func() {
			time.Sleep(time.Second)
			assert.True(t, s.IsRunning())
			s.Stop()
			time.Sleep(100 * time.Millisecond) // wait for stop goroutine to catch up
		}()

		s.StartBlocking()
		assert.False(t, s.IsRunning())
	})
}

func TestScheduler_StartAt(t *testing.T) {
	t.Run("scheduling", func(t *testing.T) {
		s := NewScheduler(time.Local)
		now := time.Now()

		// With StartAt
		job, _ := s.Every(3).Seconds().StartAt(now.Add(time.Second * 5)).Do(func() {})
		assert.False(t, job.getStartsImmediately())
		s.start()
		assert.Equal(t, now.Add(time.Second*5).Truncate(time.Second), job.NextRun().Truncate(time.Second))
		s.stop()

		// Without StartAt
		job, _ = s.Every(3).Seconds().Do(func() {})
		assert.True(t, job.getStartsImmediately())
	})

	t.Run("run", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		semaphore := make(chan bool)

		_, _ = s.Every(1).Day().StartAt(s.time.Now(s.location).Add(100 * time.Millisecond)).Do(func() {
			semaphore <- true
		})

		s.StartAsync()

		select {
		case <-time.After(200 * time.Millisecond):
			t.Fatal("job did not run at 1 second")
		case <-semaphore:
			// test passed
		}
	})

	t.Run("start in past", func(t *testing.T) {
		s := NewScheduler(time.Local)
		now := time.Now()

		// Start 5 seconds ago and make sure next run is in the future
		job, _ := s.Every(24).Hours().StartAt(now.Add(-24 * time.Hour).Add(10 * time.Minute)).Do(func() {})
		assert.False(t, job.getStartsImmediately())
		s.start()
		assert.Equal(t, now.Add(10*time.Minute).Truncate(time.Second), job.NextRun().Truncate(time.Second))
		s.stop()
	})

	t.Run("StartAt() in future with Weekday() and At()", func(t *testing.T) {
		ft := fakeTime{onNow: func(l *time.Location) time.Time {
			// 1/1/1970 is a Thursday
			return time.Date(1970, 1, 1, 12, 0, 0, 0, l)
		}}

		s := NewScheduler(time.UTC)
		s.time = ft

		dt := time.Date(1970, 1, 10, 0, 0, 0, 0, time.UTC)
		job, err := s.Every(1).Friday().At("20:19").StartAt(dt).Do(func() {})
		require.NoError(t, err)

		s.StartAsync()
		exp := time.Date(1970, 1, 16, 20, 19, 0, 0, time.UTC)
		assert.Equal(t, exp, job.NextRun())
		s.Stop()
	})

	t.Run("StartAt() in the past with Weekday() and At()", func(t *testing.T) {
		ft := fakeTime{onNow: func(l *time.Location) time.Time {
			// 1/1/1971 is a Friday
			return time.Date(1971, 1, 1, 12, 0, 0, 0, l)
		}}

		s := NewScheduler(time.UTC)
		s.time = ft

		dt := time.Date(1970, 12, 3, 0, 0, 0, 0, time.UTC)
		job, err := s.Every(1).Thursday().At("20:19").StartAt(dt).Do(func() {})
		require.NoError(t, err)

		s.StartAsync()
		exp := time.Date(1971, 1, 7, 20, 19, 0, 0, time.UTC)
		assert.Equal(t, exp, job.NextRun())
		s.Stop()
	})

	t.Run("StartAt() with Week() no At()", func(t *testing.T) {
		s := NewScheduler(time.UTC)

		dt := time.Now().UTC().Add(time.Second)
		job, err := s.Every(1).Week().StartAt(dt).Do(func() {})
		require.NoError(t, err)

		s.StartAsync()
		assert.Equal(t, dt, job.NextRun())
		time.Sleep(time.Millisecond * 1500)
		s.Stop()

		assert.Equal(t, dt.Add(time.Hour*168).Truncate(time.Second), job.NextRun())
	})
}

func calculateNextRunHelper(
	interval int,
	unit schedulingUnit,
	lastRun time.Time,
	atTimes []time.Duration,
	scheduledWeekdays []time.Weekday,
	daysOfTheMonth []int,
) *Job {
	return &Job{
		mu:       &jobMutex{},
		interval: interval,
		unit:     unit,
		jobFunction: jobFunction{
			jobRunTimes: &jobRunTimes{
				jobRunTimesMu: &sync.Mutex{},
				lastRun:       lastRun,
			},
		},
		atTimes:           atTimes,
		scheduledWeekdays: scheduledWeekdays,
		daysOfTheMonth:    daysOfTheMonth,
	}
}

func TestScheduler_CalculateNextRun(t *testing.T) {
	day := time.Hour * 24
	januaryFirst2020At := func(hour, minute, second int) time.Time {
		return time.Date(2020, time.January, 1, hour, minute, second, 0, time.UTC)
	}
	januaryFirst2019At := func(hour, minute, second int) time.Time {
		return time.Date(2019, time.January, 1, hour, minute, second, 0, time.UTC)
	}
	mondayAt := func(hour, minute, second int) time.Time {
		return time.Date(2020, time.January, 6, hour, minute, second, 0, time.UTC)
	}

	testCases := []struct {
		name                 string
		job                  *Job
		wantTimeUntilNextRun time.Duration
	}{
		// SECONDS
		{name: "every second test", job: calculateNextRunHelper(1, seconds, januaryFirst2020At(0, 0, 0), nil, nil, nil), wantTimeUntilNextRun: _getSeconds(1)},
		{name: "every 62 seconds test", job: calculateNextRunHelper(62, seconds, januaryFirst2020At(0, 0, 0), nil, nil, nil), wantTimeUntilNextRun: _getSeconds(62)},
		// MINUTES
		{name: "every minute test", job: calculateNextRunHelper(1, minutes, januaryFirst2020At(0, 0, 0), nil, nil, nil), wantTimeUntilNextRun: _getMinutes(1)},
		{name: "every 62 minutes test", job: calculateNextRunHelper(62, minutes, januaryFirst2020At(0, 0, 0), nil, nil, nil), wantTimeUntilNextRun: _getMinutes(62)},
		// HOURS
		{name: "every hour test", job: calculateNextRunHelper(1, hours, januaryFirst2020At(0, 0, 0), nil, nil, nil), wantTimeUntilNextRun: _getHours(1)},
		{name: "every 25 hours test", job: calculateNextRunHelper(25, hours, januaryFirst2020At(0, 0, 0), nil, nil, nil), wantTimeUntilNextRun: _getHours(25)},
		// DAYS
		{name: "every day at midnight", job: calculateNextRunHelper(1, days, januaryFirst2020At(0, 0, 1), nil, nil, nil), wantTimeUntilNextRun: 1*day - time.Second},
		{name: "every day at 09:30AM with scheduler starting before 09:30AM should run at same day at time", job: calculateNextRunHelper(1, days, januaryFirst2020At(0, 0, 0), []time.Duration{_getHours(9) + _getMinutes(30)}, nil, nil), wantTimeUntilNextRun: _getHours(9) + _getMinutes(30)},
		{name: "every day at 09:30AM which just ran should run tomorrow at 09:30AM", job: calculateNextRunHelper(1, days, januaryFirst2020At(9, 30, 0), []time.Duration{_getHours(9) + _getMinutes(30)}, nil, nil), wantTimeUntilNextRun: 1 * day},
		{name: "every 31 days at midnight should run 31 days later", job: calculateNextRunHelper(31, days, januaryFirst2020At(0, 0, 0), nil, nil, nil), wantTimeUntilNextRun: 31 * day},
		{name: "daily job just ran at 8:30AM and should be scheduled for next day's 8:30AM", job: calculateNextRunHelper(1, days, januaryFirst2020At(8, 30, 0), []time.Duration{8*time.Hour + 30*time.Minute}, nil, nil), wantTimeUntilNextRun: 24 * time.Hour},
		{name: "daily job just ran at 5:30AM and should be scheduled for today at 8:30AM", job: calculateNextRunHelper(1, days, januaryFirst2020At(5, 30, 0), []time.Duration{8*time.Hour + 30*time.Minute}, nil, nil), wantTimeUntilNextRun: 3 * time.Hour},
		{name: "job runs every 2 days, just ran at 5:30AM and should be scheduled for same day 8:30AM", job: calculateNextRunHelper(2, days, januaryFirst2020At(5, 30, 0), []time.Duration{5*time.Hour + 30*time.Minute, 8*time.Hour + 30*time.Minute}, nil, nil), wantTimeUntilNextRun: 3 * time.Hour},
		{name: "job runs every 2 days, just ran at 8:30AM and should be scheduled for 2 days at 8:30AM", job: calculateNextRunHelper(2, days, januaryFirst2020At(8, 30, 0), []time.Duration{8*time.Hour + 30*time.Minute}, nil, nil), wantTimeUntilNextRun: 2 * day},
		{name: "daily, last run was 1 second ago", job: calculateNextRunHelper(1, days, januaryFirst2020At(11, 59, 59), []time.Duration{12 * time.Hour}, nil, nil), wantTimeUntilNextRun: time.Second},
		//// WEEKS
		{name: "every week should run in 7 days", job: calculateNextRunHelper(1, weeks, januaryFirst2020At(0, 0, 0), nil, nil, nil), wantTimeUntilNextRun: 7 * day},
		{name: "every week with .At time rule should run respect .At time rule", job: calculateNextRunHelper(1, weeks, januaryFirst2020At(9, 31, 0), []time.Duration{_getHours(9) + _getMinutes(30)}, nil, nil), wantTimeUntilNextRun: 7*day - time.Minute},
		{name: "every two weeks at 09:30AM should run in 14 days at 09:30AM", job: calculateNextRunHelper(2, weeks, januaryFirst2020At(0, 0, 0), nil, nil, nil), wantTimeUntilNextRun: 14 * day},
		{name: "every 31 weeks ran at jan 1st at midnight should run at August 5, 2020", job: calculateNextRunHelper(31, weeks, januaryFirst2020At(0, 0, 0), nil, nil, nil), wantTimeUntilNextRun: 31 * 7 * day},
		// MONTHS
		{name: "every month in a 31 days month should be scheduled for 31 days ahead", job: calculateNextRunHelper(1, months, januaryFirst2020At(0, 0, 0), nil, nil, nil), wantTimeUntilNextRun: 31 * day},
		{name: "every month in a 30 days month should be scheduled for 30 days ahead", job: calculateNextRunHelper(1, months, time.Date(2020, time.April, 1, 0, 0, 0, 0, time.UTC), nil, nil, nil), wantTimeUntilNextRun: 30 * day},
		{name: "every month at february on leap year should count 29 days", job: calculateNextRunHelper(1, months, time.Date(2020, time.February, 1, 0, 0, 0, 0, time.UTC), nil, nil, nil), wantTimeUntilNextRun: 29 * day},
		{name: "every month at february on non leap year should count 28 days", job: calculateNextRunHelper(1, months, time.Date(2019, time.February, 1, 0, 0, 0, 0, time.UTC), nil, nil, nil), wantTimeUntilNextRun: 28 * day},
		{name: "every month at first day at time should run next month", job: calculateNextRunHelper(1, months, januaryFirst2020At(9, 30, 0), []time.Duration{_getHours(9) + _getMinutes(30)}, nil, nil), wantTimeUntilNextRun: 31 * day},
		{name: "every month at day should consider at days", job: calculateNextRunHelper(1, months, januaryFirst2020At(0, 0, 0), nil, nil, []int{2}), wantTimeUntilNextRun: 1 * day},
		{name: "every month at day should consider at hours", job: calculateNextRunHelper(1, months, januaryFirst2020At(0, 0, 0), []time.Duration{_getHours(9) + _getMinutes(30)}, nil, nil), wantTimeUntilNextRun: 31*day + _getHours(9) + _getMinutes(30)},
		{name: "every month on the first day, but started on january 8th, should run February 1st", job: calculateNextRunHelper(1, months, januaryFirst2020At(0, 0, 0).AddDate(0, 0, 7), nil, nil, []int{1}), wantTimeUntilNextRun: 24 * day},
		{name: "every month same as lastRun, should run February 1st", job: calculateNextRunHelper(1, months, januaryFirst2020At(0, 0, 0), nil, nil, []int{1}), wantTimeUntilNextRun: 31 * day},
		{name: "every 2 months at day 1, starting at day 1, should run in 2 months", job: calculateNextRunHelper(2, months, januaryFirst2020At(0, 0, 0), nil, nil, []int{1}), wantTimeUntilNextRun: 31*day + 29*day},                          // 2020 january and february
		{name: "every 2 months at day 2, starting at day 1, should run in 2 months + 1 day", job: calculateNextRunHelper(2, months, januaryFirst2020At(0, 0, 0), nil, nil, []int{2}), wantTimeUntilNextRun: 31*day + 29*day + 1*day},          // 2020 january and february
		{name: "every 2 months at day 1, starting at day 2, should run in 2 months - 1 day", job: calculateNextRunHelper(2, months, januaryFirst2020At(0, 0, 0).AddDate(0, 0, 1), nil, nil, []int{1}), wantTimeUntilNextRun: 30*day + 29*day}, // 2020 january and february
		{name: "every 13 months at day 1, starting at day 2 run in 13 months - 1 day", job: calculateNextRunHelper(13, months, januaryFirst2020At(0, 0, 0).AddDate(0, 0, 1), nil, nil, []int{1}), wantTimeUntilNextRun: januaryFirst2020At(0, 0, 0).AddDate(0, 13, -1).Sub(januaryFirst2020At(0, 0, 0))},
		{name: "every last day of the month started on leap year february should run on march 31", job: calculateNextRunHelper(1, months, time.Date(2020, time.February, 29, 0, 0, 0, 0, time.UTC), nil, nil, []int{-1}), wantTimeUntilNextRun: 31 * day},
		{name: "every last day of the month started on non-leap year february should run on march 31", job: calculateNextRunHelper(1, months, time.Date(2019, time.February, 28, 0, 0, 0, 0, time.UTC), nil, nil, []int{-1}), wantTimeUntilNextRun: 31 * day},
		{name: "every last day of 2 months started on leap year february should run on april 30", job: calculateNextRunHelper(2, months, time.Date(2020, time.February, 29, 0, 0, 0, 0, time.UTC), nil, nil, []int{-1}), wantTimeUntilNextRun: 31*day + 30*day},
		{name: "every last day of 2 months started on non-leap year february should run on april 30", job: calculateNextRunHelper(2, months, time.Date(2019, time.February, 28, 0, 0, 0, 0, time.UTC), nil, nil, []int{-1}), wantTimeUntilNextRun: 31*day + 30*day},
		{name: "every last day of the month started on january 1 in leap year should run on january 31", job: calculateNextRunHelper(1, months, januaryFirst2020At(0, 0, 0), nil, nil, []int{-1}), wantTimeUntilNextRun: 30 * day},
		{name: "every last day of the month started on january 1 in non-leap year should run on january 31", job: calculateNextRunHelper(1, months, januaryFirst2019At(0, 0, 0), nil, nil, []int{-1}), wantTimeUntilNextRun: 30 * day},
		{name: "every last day of the month started on january 30 in leap year should run on january 31", job: calculateNextRunHelper(1, months, januaryFirst2020At(0, 0, 0).AddDate(0, 0, 29), nil, nil, []int{-1}), wantTimeUntilNextRun: 1 * day},
		{name: "every last day of the month started on january 30 in non-leap year should run on january 31", job: calculateNextRunHelper(1, months, januaryFirst2019At(0, 0, 0).AddDate(0, 0, 29), nil, nil, []int{-1}), wantTimeUntilNextRun: 1 * day},
		{name: "every last day of the month started on january 31 in leap year should run on february 29", job: calculateNextRunHelper(1, months, januaryFirst2020At(0, 0, 0).AddDate(0, 0, 30), nil, nil, []int{-1}), wantTimeUntilNextRun: 29 * day},
		{name: "every last day of the month started on january 31 in non-leap year should run on february 28", job: calculateNextRunHelper(1, months, januaryFirst2019At(0, 0, 0).AddDate(0, 0, 30), nil, nil, []int{-1}), wantTimeUntilNextRun: 28 * day},
		{name: "every last day of the month started on december 31 should run on january 31 of the next year", job: calculateNextRunHelper(1, months, januaryFirst2019At(0, 0, 0).AddDate(0, 0, -1), nil, nil, []int{-1}), wantTimeUntilNextRun: 31 * day},
		{name: "every last day of 2 months started on december 31, 2018 should run on february 28, 2019", job: calculateNextRunHelper(2, months, januaryFirst2019At(0, 0, 0).AddDate(0, 0, -1), nil, nil, []int{-1}), wantTimeUntilNextRun: 31*day + 28*day},
		{name: "every last day of 2 months started on december 31, 2019 should run on february 29, 2020", job: calculateNextRunHelper(2, months, januaryFirst2020At(0, 0, 0).AddDate(0, 0, -1), nil, nil, []int{-1}), wantTimeUntilNextRun: 31*day + 29*day},
		//// WEEKDAYS
		{name: "every weekday starting on one day before it should run this weekday", job: calculateNextRunHelper(1, weeks, mondayAt(0, 0, 0), nil, []time.Weekday{*_tuesdayWeekday()}, nil), wantTimeUntilNextRun: 1 * day},
		{name: "every weekday starting on same weekday should run in 7 days", job: calculateNextRunHelper(1, weeks, mondayAt(0, 0, 0).AddDate(0, 0, 1), nil, []time.Weekday{*_tuesdayWeekday()}, nil), wantTimeUntilNextRun: 7 * day},
		{name: "every 2 weekdays counting this week's weekday should run next weekday", job: calculateNextRunHelper(2, weeks, mondayAt(0, 0, 0), nil, []time.Weekday{*_tuesdayWeekday()}, nil), wantTimeUntilNextRun: day},
		{name: "every weekday starting on one day after should count days remaining", job: calculateNextRunHelper(1, weeks, mondayAt(0, 0, 0).AddDate(0, 0, 2), nil, []time.Weekday{*_tuesdayWeekday()}, nil), wantTimeUntilNextRun: 6 * day},
		{name: "every weekday starting before jobs .At() time should run at same day at time", job: calculateNextRunHelper(1, weeks, mondayAt(0, 0, 0).AddDate(0, 0, 1), []time.Duration{_getHours(9) + _getMinutes(30)}, []time.Weekday{*_tuesdayWeekday()}, nil), wantTimeUntilNextRun: _getHours(9) + _getMinutes(30)},
		{name: "every weekday starting at same day at time that already passed should run at next week at time", job: calculateNextRunHelper(1, weeks, mondayAt(10, 30, 0).AddDate(0, 0, 1), []time.Duration{_getHours(9) + _getMinutes(30)}, []time.Weekday{*_tuesdayWeekday()}, nil), wantTimeUntilNextRun: 6*day + _getHours(23) + _getMinutes(0)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ft := fakeTime{onNow: func(l *time.Location) time.Time {
				return tc.job.lastRun
			}}

			s := NewScheduler(time.UTC)
			s.time = ft
			tc.job.runStartCount = atomic.NewInt64(0)
			got := s.durationToNextRun(tc.job.LastRun(), tc.job).duration
			assert.Equalf(t, tc.wantTimeUntilNextRun, got, fmt.Sprintf("expected %s / got %s", tc.wantTimeUntilNextRun.String(), got.String()))
		})
	}
}

// helper test method
func _tuesdayWeekday() *time.Weekday {
	tuesday := time.Tuesday
	return &tuesday
}

// helper test method
func _getDays(i int) time.Duration {
	return time.Duration(i) * time.Hour * 24
}

// helper test method
func _getSeconds(i int) time.Duration {
	return time.Duration(i) * time.Second
}

// helper test method
func _getHours(i int) time.Duration {
	return time.Duration(i) * time.Hour
}

// helper test method
func _getMinutes(i int) time.Duration {
	return time.Duration(i) * time.Minute
}

func TestScheduler_Do(t *testing.T) {
	testCases := []struct {
		description string
		evalFunc    func(*Scheduler)
	}{
		{
			"adding a new job before scheduler starts does not schedule job",
			func(s *Scheduler) {
				s.setRunning(false)
				job, err := s.Every(1).Second().Do(func() {})
				assert.Equal(t, nil, err)
				assert.True(t, job.NextRun().IsZero())
			},
		},
		{
			"adding a new job when scheduler is running schedules job",
			func(s *Scheduler) {
				s.setRunning(true)
				job, err := s.Every(1).Second().Do(func() {})
				assert.Equal(t, nil, err)
				assert.False(t, job.NextRun().IsZero())
			},
		},
		{
			description: "error due to the arg passed to Do() not being a function",
			evalFunc: func(s *Scheduler) {
				_, err := s.Every(1).Second().Do(1)
				assert.EqualError(t, err, ErrNotAFunction.Error())
				assert.Zero(t, s.Len(), "The job should be deleted if the arg passed to Do() is not a function")
			},
		},
		{
			description: "error due to every/cron not called",
			evalFunc: func(s *Scheduler) {
				_, err := s.Do(1)
				assert.EqualError(t, err, ErrInvalidInterval.Error())
				assert.Zero(t, s.Len(), "The job should be deleted if Every or Cron is not called")
			},
		},
		{
			description: "positive case",
			evalFunc: func(s *Scheduler) {
				_, err := s.Every(1).Day().Do(func() {})
				require.NoError(t, err)
				assert.Equal(t, 1, s.Len())
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s := NewScheduler(time.Local)
			tc.evalFunc(s)
		})
	}
}

func TestRunJobsWithLimit(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		semaphore := make(chan bool)

		s := NewScheduler(time.UTC)

		j, err := s.Every("100ms").Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)
		j.LimitRunsTo(1)

		s.StartAsync()
		time.Sleep(200 * time.Millisecond)

		var counter int
		now := time.Now()
		for time.Now().Before(now.Add(200 * time.Millisecond)) {
			select {
			case <-semaphore:
				counter++
			default:
			}
		}

		assert.Equal(t, 1, counter)
	})

	t.Run("remove after last run", func(t *testing.T) {
		semaphore := make(chan bool)

		s := NewScheduler(time.UTC)

		j, err := s.Every("100ms").Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)
		j.LimitRunsTo(1)

		s.StartAsync()
		time.Sleep(200 * time.Millisecond)

		var counter int
		select {
		case <-time.After(200 * time.Millisecond):
			assert.Equal(t, 0, s.Len())
		case <-semaphore:
			counter++
			require.LessOrEqual(t, counter, 1)
		}
	})

	t.Run("remove unique tags also", func(t *testing.T) {
		semaphore := make(chan bool)

		s := NewScheduler(time.UTC)
		s.TagsUnique()

		j, err := s.Every("100ms").Tag("tag1", "tag2", "tag3").Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)
		j.LimitRunsTo(1)

		s.StartAsync()
		time.Sleep(200 * time.Millisecond)

		var counter int
		select {
		case <-time.After(200 * time.Millisecond):
			assert.Equal(t, 0, s.Len())
		case <-semaphore:
			counter++
			require.LessOrEqual(t, counter, 1)
		}

		s.tags.Range(func(key, value interface{}) bool {
			assert.FailNow(t, "map should be empty")
			return true
		})
	})
}

func TestCalculateMonthsError(t *testing.T) {
	testCases := []struct {
		desc       string
		dayOfMonth []int
	}{
		// -1 is now interpreted as "last day of the month"
		{"invalid 29", []int{29}},
		{"invalid -1 in list", []int{27, -1}},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			s := NewScheduler(time.UTC)
			job, err := s.Every(1).Month(tc.dayOfMonth...).Do(func() {
				fmt.Println("hello task")
			})
			require.Error(t, err)
			require.Nil(t, job)
		})
	}
}

func TestCalculateMonths(t *testing.T) {
	maySecond2021At0200 := time.Date(2021, 5, 2, 2, 0, 0, 0, time.UTC)

	maySecond2021At0800 := time.Date(2021, 5, 2, 8, 0, 0, 0, time.UTC)

	maySixth2021At0200 := time.Date(2021, 5, 6, 2, 0, 0, 0, time.UTC)

	maySixth2021At0500 := fakeTime{onNow: func(l *time.Location) time.Time {
		return time.Date(2021, 5, 6, 5, 0, 0, 0, l)
	}}

	maySixth2021At0800 := time.Date(2021, 5, 6, 8, 0, 0, 0, time.UTC)

	mayTenth2021At0200 := time.Date(2021, 5, 10, 2, 0, 0, 0, time.UTC)

	mayTenth2021At0800 := time.Date(2021, 5, 10, 8, 0, 0, 0, time.UTC)

	day := time.Hour * 24

	testCases := []struct {
		description          string
		job                  *Job
		wantTimeUntilNextRun time.Duration
	}{
		{description: "day before current and before current time, should run next month", job: calculateNextRunHelper(1, months, maySixth2021At0500.Now(time.UTC), []time.Duration{_getHours(2)}, nil, []int{2}), wantTimeUntilNextRun: (31 * day) - maySixth2021At0500.Now(time.UTC).Sub(maySecond2021At0200)},
		{description: "day before current and after current time, should run next month", job: calculateNextRunHelper(1, months, maySixth2021At0500.Now(time.UTC), []time.Duration{_getHours(8)}, nil, []int{2}), wantTimeUntilNextRun: (31 * day) - maySixth2021At0500.Now(time.UTC).Sub(maySecond2021At0800)},
		{description: "current day and before current time, should run next month", job: calculateNextRunHelper(1, months, maySixth2021At0500.Now(time.UTC), []time.Duration{_getHours(2)}, nil, []int{6}), wantTimeUntilNextRun: (31 * day) - maySixth2021At0500.Now(time.UTC).Sub(maySixth2021At0200)},
		{description: "current day and after current time, should run on current day", job: calculateNextRunHelper(1, months, maySixth2021At0500.Now(time.UTC), []time.Duration{_getHours(8)}, nil, []int{6}), wantTimeUntilNextRun: maySixth2021At0800.Sub(maySixth2021At0500.Now(time.UTC))},
		{description: "day after current and before current time, should run on current month", job: calculateNextRunHelper(1, months, maySixth2021At0500.Now(time.UTC), []time.Duration{_getHours(2)}, nil, []int{10}), wantTimeUntilNextRun: mayTenth2021At0200.Sub(maySixth2021At0500.Now(time.UTC))},
		{description: "day after current and after current time, should run on current month", job: calculateNextRunHelper(1, months, maySixth2021At0500.Now(time.UTC), []time.Duration{_getHours(8)}, nil, []int{10}), wantTimeUntilNextRun: mayTenth2021At0800.Sub(maySixth2021At0500.Now(time.UTC))},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s := NewScheduler(time.UTC)
			s.time = maySixth2021At0500
			got := s.durationToNextRun(tc.job.LastRun(), tc.job).duration
			assert.Equalf(t, tc.wantTimeUntilNextRun, got, fmt.Sprintf("expected %s / got %s", tc.wantTimeUntilNextRun.String(), got.String()))
		})
	}
}

func TestScheduler_SingletonMode(t *testing.T) {
	testCases := []struct {
		description string
		removeJob   bool
	}{
		{"with scheduler stop", false},
		{"with job removal", true},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s := NewScheduler(time.UTC)
			trigger := atomic.NewInt64(0)

			j, err := s.Every("100ms").SingletonMode().Do(func() {
				if trigger.Load() == 1 {
					t.Fatal("Restart should not occur")
				}
				trigger.Add(1)
				time.Sleep(300 * time.Millisecond)
			})
			require.NoError(t, err)

			s.StartAsync()
			time.Sleep(200 * time.Millisecond)

			if tc.removeJob {
				err = s.RemoveByID(j)
				require.NoError(t, err)
				time.Sleep(300 * time.Millisecond)
			}
			s.Stop()
		})
	}
}

func TestScheduler_SingletonModeAll(t *testing.T) {
	testCases := []struct {
		description string
		removeJob   bool
	}{
		{"with scheduler stop", false},
		{"with job removal", true},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s := NewScheduler(time.UTC)
			s.SingletonModeAll()

			trigger := atomic.NewInt64(0)

			j, err := s.Every("100ms").Do(func() {
				if trigger.Load() == 1 {
					t.Fatal("Restart should not occur")
				}
				trigger.Add(1)
				time.Sleep(300 * time.Millisecond)
			})
			require.NoError(t, err)

			s.StartAsync()
			time.Sleep(200 * time.Millisecond)

			if tc.removeJob {
				err = s.RemoveByID(j)
				require.NoError(t, err)
				time.Sleep(300 * time.Millisecond)
			}
			s.Stop()
		})
	}
}

func TestScheduler_LimitRunsTo(t *testing.T) {
	t.Run("job added before starting scheduler", func(t *testing.T) {
		semaphore := make(chan bool)

		s := NewScheduler(time.UTC)

		_, err := s.Every("100ms").LimitRunsTo(1).Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)

		s.StartAsync()
		time.Sleep(200 * time.Millisecond)

		var counter int
		select {
		case <-time.After(200 * time.Millisecond):
			assert.Equal(t, 1, counter)
		case <-semaphore:
			counter++
			require.LessOrEqual(t, counter, 1)
		}
	})

	t.Run("job added after starting scheduler", func(t *testing.T) {
		semaphore := make(chan bool)

		s := NewScheduler(time.UTC)
		s.StartAsync()

		_, err := s.Every("100ms").LimitRunsTo(1).Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)

		time.Sleep(200 * time.Millisecond)

		var counter int
		select {
		case <-time.After(200 * time.Millisecond):
			assert.Equal(t, 1, counter)
		case <-semaphore:
			counter++
			require.LessOrEqual(t, counter, 1)
		}
	})

	t.Run("job added after starting scheduler - using job's LimitRunsTo - results in two runs", func(t *testing.T) {
		semaphore := make(chan bool)

		s := NewScheduler(time.UTC)
		s.StartAsync()

		j, err := s.Every("100ms").Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)
		j.LimitRunsTo(1)

		time.Sleep(200 * time.Millisecond)

		var counter int
		select {
		case <-time.After(200 * time.Millisecond):
			assert.Equal(t, 2, counter)
		case <-semaphore:
			counter++
			require.LessOrEqual(t, counter, 2)
		}
	})
}

func TestScheduler_SetMaxConcurrentJobs(t *testing.T) {
	semaphore := make(chan bool)

	testCases := []struct {
		description       string
		maxConcurrentJobs int
		mode              limitMode
		expectedRuns      int
		removeJobs        bool
		f                 func()
	}{
		// Expecting a total of 4 job runs:
		// 0ms - 2 jobs are run, 1 is skipped
		// 100ms - 3 jobs hit the limit and are skipped
		// 200ms - 2 jobs are run, 1 is skipped
		// 300ms - 3 jobs hit the limit and are skipped
		{
			"reschedule mode", 2, RescheduleMode, 4, false,
			func() {
				semaphore <- true
				time.Sleep(200 * time.Millisecond)
			},
		},
		{
			"reschedule mode with job removal", 2, RescheduleMode, 4, true,
			func() {
				semaphore <- true
				time.Sleep(200 * time.Millisecond)
			},
		},

		// Expecting a total of 8 job runs. The exact order of jobs may vary, for example:
		// 0ms - jobs 2 & 3 run, job 1 hits the limit and waits
		// 100ms - job 1 runs twice, the blocked run and the regularly scheduled run
		// 200ms - jobs 1 & 3 run
		// 300ms - jobs 2 & 3 run, job 1 hits the limit and waits
		{
			"wait mode", 2, WaitMode, 8, false,
			func() {
				semaphore <- true
				time.Sleep(100 * time.Millisecond)
			},
		},

		//// Same as above - this confirms the same behavior when jobs are removed rather than the scheduler being stopped
		{
			"wait mode - with job removal", 2, WaitMode, 8, true,
			func() {
				semaphore <- true
				time.Sleep(100 * time.Millisecond)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s := NewScheduler(time.UTC)
			s.SetMaxConcurrentJobs(tc.maxConcurrentJobs, tc.mode)

			j1, err := s.Every("100ms").Do(tc.f)
			require.NoError(t, err)

			j2, err := s.Every("100ms").Do(tc.f)
			require.NoError(t, err)

			j3, err := s.Every("100ms").Do(tc.f)
			require.NoError(t, err)

			s.StartAsync()

			var counter int

			now := time.Now()
			for time.Now().Before(now.Add(400 * time.Millisecond)) {
				select {
				case <-semaphore:
					counter++
				default:
				}
			}

			if tc.removeJobs {
				_ = s.RemoveByID(j1)
				_ = s.RemoveByID(j2)
				_ = s.RemoveByID(j3)
			} else {
				s.Stop()
			}

			// make sure no more jobs are run as the executor
			// or job should be properly stopped

			now = time.Now()
			for time.Now().Before(now.Add(200 * time.Millisecond)) {
				select {
				case <-semaphore:
					t.Error("received a job run after jobs were removed or scheduler stopeed")
				default:
				}
			}

			assert.GreaterOrEqual(t, counter, tc.expectedRuns)
			assert.LessOrEqual(t, counter, tc.expectedRuns)

			if tc.removeJobs {
				s.Stop()
			}
		})
	}
}

func TestScheduler_TagsUnique(t *testing.T) {
	const (
		foo = "foo"
		bar = "bar"
		baz = "baz"
	)

	s := NewScheduler(time.UTC)
	s.TagsUnique()

	j, err := s.Every("1s").Tag(foo, bar).Do(func() {})
	require.NoError(t, err)

	// uniqueness not enforced on jobs tagged with job.Tag()
	// thus tagging the job here is allowed
	j.Tag(baz)
	_, err = s.Every("1s").Tag(baz).Do(func() {})
	require.NoError(t, err)

	_, err = s.Every("1s").Tag(foo).Do(func() {})
	assert.EqualError(t, err, ErrTagsUnique(foo).Error())

	_, err = s.Every("1s").Tag(bar).Do(func() {})
	assert.EqualError(t, err, ErrTagsUnique(bar).Error())
}

func TestScheduler_MultipleTagsChained(t *testing.T) {
	const (
		tag1 = "tag1"
		tag2 = "tag2"
	)

	s := NewScheduler(time.UTC)

	j, err := s.Every("1s").Tag(tag1).Tag(tag2).Do(func() {})
	require.NoError(t, err)

	assert.EqualValues(t, []string{tag1, tag2}, j.Tags())
}

func TestScheduler_DoParameterValidation(t *testing.T) {
	testCases := []struct {
		description string
		parameters  []interface{}
	}{
		{"less than expected", []interface{}{"p1"}},
		{"more than expected", []interface{}{"p1", "p2", "p3"}},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s := NewScheduler(time.UTC)
			f := func(s1, s2 string) {
				fmt.Println("ok")
			}

			_, err := s.Every(1).Days().StartAt(time.Now().UTC().Add(time.Second*10)).Do(f, tc.parameters...)
			assert.EqualError(t, err, ErrWrongParams.Error())
		})
	}
}

func TestScheduler_Job(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		s := NewScheduler(time.UTC)

		j1, err := s.Every("1s").Do(func() {})
		require.NoError(t, err)
		assert.Equal(t, j1, s.jobs[j1.id])

		j2, err := s.Every("1s").Do(func() {})
		require.NoError(t, err)
		assert.Equal(t, j2, s.jobs[j2.id])

		s.Job(j1)
		assert.Equal(t, j1, s.jobs[j1.id])

		s.Job(j2)
		assert.Equal(t, j2, s.jobs[j2.id])
	})
	t.Run("job address doesn't match after de/re-referencing", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		j1, err := s.Every("1s").Do(func() {})
		require.NoError(t, err)
		assert.Equal(t, j1, s.jobs[j1.id])

		j1PtrDeReferenced := *j1
		j1PtrReReferenced := &j1PtrDeReferenced

		_, err = s.Job(j1PtrReReferenced).Every("2s").Update()
		assert.EqualError(t, err, ErrUpdateCalledWithoutJob.Error())
	})
}

func TestScheduler_Update(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		s := NewScheduler(time.UTC)

		var counterMutex sync.RWMutex
		counter := 0

		j, err := s.Every(1).Day().Do(func() { counterMutex.Lock(); defer counterMutex.Unlock(); counter++ })
		require.NoError(t, err)

		s.StartAsync()

		time.Sleep(300 * time.Millisecond)
		_, err = s.Job(j).Every("500ms").Update()
		require.NoError(t, err)

		time.Sleep(550 * time.Millisecond)
		_, err = s.Job(j).Every("750ms").Update()
		require.NoError(t, err)

		time.Sleep(800 * time.Millisecond)
		_, err = s.Job(j).CronWithSeconds("*/1 * * * * *").Update()
		require.NoError(t, err)

		time.Sleep(1200 * time.Millisecond)
		s.Stop()

		counterMutex.RLock()
		defer counterMutex.RUnlock()
		assert.GreaterOrEqual(t, counter, 2)
		assert.LessOrEqual(t, counter, 3)
	})

	t.Run("happy singleton mode", func(t *testing.T) {
		s := NewScheduler(time.UTC)

		var counterMutex sync.RWMutex
		counter := 0

		j, err := s.Every(1).Day().SingletonMode().Do(func() {
			counterMutex.Lock()
			defer counterMutex.Unlock()
			counter++
		})
		require.NoError(t, err)

		s.StartAsync()

		time.Sleep(300 * time.Millisecond)
		j1, err := s.Job(j).Every("500ms").Update()
		require.Equal(t, 500*time.Millisecond, j1.getDuration())
		require.NoError(t, err)

		time.Sleep(550 * time.Millisecond)
		j2, err := s.Job(j).Every("750ms").Update()
		require.Equal(t, 750*time.Millisecond, j2.getDuration())
		require.NoError(t, err)

		time.Sleep(time.Second)
		s.Stop()

		counterMutex.RLock()
		defer counterMutex.RUnlock()
		assert.GreaterOrEqual(t, counter, 2)
		assert.LessOrEqual(t, counter, 3)
	})

	t.Run("update called without job call", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		_, err := s.Every("1s").Do(func() {})
		require.NoError(t, err)

		_, err = s.Update()
		assert.EqualError(t, err, ErrUpdateCalledWithoutJob.Error())
	})

	t.Run("update, delete, create", func(t *testing.T) {
		s := NewScheduler(time.UTC)

		var counterMutex sync.RWMutex
		counter := 0

		j, err := s.Every(1).Day().Do(func() {
			counterMutex.Lock()
			defer counterMutex.Unlock()
			counter++
		})
		require.NoError(t, err)

		s.StartAsync()
		time.Sleep(300 * time.Millisecond)

		_, err = s.Job(j).Every("500ms").Update()
		require.NoError(t, err)

		time.Sleep(550 * time.Millisecond)
		_ = s.RemoveByID(j)

		j, err = s.Every("750ms").WaitForSchedule().Do(func() { counterMutex.Lock(); defer counterMutex.Unlock(); counter++ })
		require.NoError(t, err)

		time.Sleep(800 * time.Millisecond)
		_, err = s.Job(j).CronWithSeconds("*/1 * * * * *").Update()
		require.NoError(t, err)

		time.Sleep(time.Second)
		s.Stop()

		counterMutex.RLock()
		defer counterMutex.RUnlock()
		assert.GreaterOrEqual(t, counter, 3)
		assert.LessOrEqual(t, counter, 4)
	})

	// Verifies https://github.com/go-co-op/gocron/issues/424
	t.Run("next run calculated correctly", func(t *testing.T) {
		s := NewScheduler(time.UTC)

		job, err := s.Every(time.Second).Do(func() {})
		require.NoError(t, err)
		s.StartAsync()

		newInterval := 5 * time.Second
		_, err = s.Job(job).Every(newInterval).Update()
		require.NoError(t, err)
		s.Stop()

		last := job.LastRun().Round(time.Millisecond)
		actualNext := job.NextRun().Round(time.Millisecond)
		expectedNext := last.Add(newInterval)

		assert.Equal(t, expectedNext, actualNext)
	})

	// Verifies https://github.com/go-co-op/gocron/issues/499
	t.Run("at time is not set when updating incompatible unit jobs", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		startAt := time.Now().UTC().Add(time.Millisecond * 250)
		j, err := s.Every(24 * time.Hour).StartAt(startAt).Do(func() {})
		require.NoError(t, err)
		s.StartAsync()
		time.Sleep(time.Millisecond * 500)

		_, err = s.Job(j).StartAt(startAt.Add(1 * time.Second)).Update()
		require.NoError(t, err)
		s.Stop()
	})

	// Verifies https://github.com/go-co-op/gocron/issues/528
	t.Run("update with DoWithJobDetails", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		j, err := s.Every(time.Second).DoWithJobDetails(func(job Job) {})
		require.NoError(t, err)
		s.StartAsync()
		time.Sleep(time.Millisecond * 500)

		_, err = s.Job(j).Every(2 * time.Second).Update()
		require.NoError(t, err)
		s.Stop()
	})
}

func TestScheduler_RunByTag(t *testing.T) {
	var (
		s            = NewScheduler(time.Local)
		wg           sync.WaitGroup
		counterMutex sync.RWMutex
		count        = 0
	)

	_, _ = s.Every(1).Day().StartAt(time.Now().Add(time.Hour)).Tag("tag").Do(func() {
		counterMutex.Lock()
		defer counterMutex.Unlock()
		count++
		wg.Done()
	})
	wg.Add(3)
	s.StartAsync()

	assert.NoError(t, s.RunByTag("tag"))
	assert.NoError(t, s.RunByTag("tag"))
	assert.NoError(t, s.RunByTag("tag"))

	wg.Wait()
	counterMutex.RLock()
	defer counterMutex.RUnlock()
	assert.Equal(t, 3, count)
	assert.Error(t, s.RunByTag("wrong-tag"))
}

func TestScheduler_Cron(t *testing.T) {
	ft := fakeTime{onNow: func(l *time.Location) time.Time {
		// January 1st, 12 noon, Thursday, 1970
		return time.Date(1970, 1, 1, 12, 0, 0, 0, l)
	}}

	s := NewScheduler(time.UTC)
	s.time = ft

	testCases := []struct {
		description     string
		cronTab         string
		expectedNextRun time.Time
		expectedError   error
	}{
		// https://crontab.guru/
		{"every minute", "*/1 * * * *", ft.onNow(time.UTC).Add(time.Minute), nil},
		{"every day 1am", "0 1 * * *", ft.onNow(time.UTC).Add(13 * time.Hour), nil},
		{"weekends only", "0 0 * * 6,0", ft.onNow(time.UTC).Add(36 * time.Hour), nil},
		{"at time monday thru friday", "0 22 * * 1-5", ft.onNow(time.UTC).Add(10 * time.Hour), nil},
		{"every minute in range, monday thru friday", "15-30 * * * 1-5", ft.onNow(time.UTC).Add(15 * time.Minute), nil},
		{"at every minute past every hour from 1 through 5 on every day-of-week from Monday through Friday.", "* 1-5 * * 1-5", ft.onNow(time.UTC).Add(13 * time.Hour), nil},
		{"hourly", "@hourly", ft.onNow(time.UTC).Add(time.Hour), nil},
		{"every day 1am in shanghai", "CRON_TZ=Asia/Shanghai 0 1 * * *", ft.onNow(time.UTC).Add(5 * time.Hour), nil},
		{"bad expression", "bad", time.Time{}, wrapOrError(fmt.Errorf("expected exactly 5 fields, found 1: [bad]"), ErrCronParseFailure)},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			j, err := s.Cron(tc.cronTab).Do(func() {})
			if tc.expectedError == nil {
				require.NoError(t, err)

				s.scheduleNextRun(j)

				assert.Exactly(t, tc.expectedNextRun, j.NextRun())
			} else {
				assert.EqualError(t, err, tc.expectedError.Error())
			}
		})
	}

	t.Run("error At() called with Cron()", func(t *testing.T) {
		_, err := s.Cron("@hourly").At("1:00").Do(func() {})
		assert.EqualError(t, err, ErrAtTimeNotSupported.Error())
	})

	t.Run("error Weekday() called with Cron()", func(t *testing.T) {
		_, err := s.Cron("@hourly").Sunday().Do(func() {})
		assert.EqualError(t, err, wrapOrError(ErrInvalidIntervalUnitsSelection, ErrWeekdayNotSupported).Error())
	})
}

func TestScheduler_CronWithSeconds(t *testing.T) {
	ft := fakeTime{onNow: func(l *time.Location) time.Time {
		// January 1st, 12 noon, Thursday, 1970
		return time.Date(1970, 1, 1, 12, 0, 0, 0, l)
	}}

	s := NewScheduler(time.UTC)
	s.time = ft

	testCases := []struct {
		description     string
		cronTab         string
		expectedNextRun time.Time
		expectedError   error
	}{
		// https://crontab.guru/
		{"every second", "*/1 * * * * *", ft.onNow(time.UTC).Add(time.Second), nil},
		{"every second from 0-30", "0-30 * * * * *", ft.onNow(time.UTC).Add(time.Second), nil},
		{"every minute", "0 */1 * * * *", ft.onNow(time.UTC).Add(time.Minute), nil},
		{"every day 1am", "* 0 1 * * *", ft.onNow(time.UTC).Add(13 * time.Hour), nil},
		{"weekends only", "* 0 0 * * 6,0", ft.onNow(time.UTC).Add(36 * time.Hour), nil},
		{"at time monday thru friday", "* 0 22 * * 1-5", ft.onNow(time.UTC).Add(10 * time.Hour), nil},
		{"every minute in range, monday thru friday", "* 15-30 * * * 1-5", ft.onNow(time.UTC).Add(15 * time.Minute), nil},
		{"at every minute past every hour from 1 through 5 on every day-of-week from Monday through Friday.", "* * 1-5 * * 1-5", ft.onNow(time.UTC).Add(13 * time.Hour), nil},
		{"hourly", "@hourly", ft.onNow(time.UTC).Add(time.Hour), nil},
		{"bad expression", "bad", time.Time{}, wrapOrError(fmt.Errorf("expected exactly 6 fields, found 1: [bad]"), ErrCronParseFailure)},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			j, err := s.CronWithSeconds(tc.cronTab).Do(func() {})
			if tc.expectedError == nil {
				require.NoError(t, err)

				s.scheduleNextRun(j)

				assert.Exactly(t, tc.expectedNextRun, j.NextRun())
			} else {
				assert.EqualError(t, err, tc.expectedError.Error())
			}
		})
	}

	t.Run("error At() called with Cron()", func(t *testing.T) {
		_, err := s.Cron("@hourly").At("1:00").Do(func() {})
		assert.EqualError(t, err, ErrAtTimeNotSupported.Error())
	})

	t.Run("error Weekday() called with Cron()", func(t *testing.T) {
		_, err := s.Cron("@hourly").Sunday().Do(func() {})
		assert.EqualError(t, err, wrapOrError(ErrInvalidIntervalUnitsSelection, ErrWeekdayNotSupported).Error())
	})
}

func TestScheduler_WaitForSchedule(t *testing.T) {
	s := NewScheduler(time.UTC)

	var counterMutex sync.RWMutex
	counter := 0

	_, err := s.Every("100ms").WaitForSchedule().Do(func() { counterMutex.Lock(); defer counterMutex.Unlock(); counter++ })
	require.NoError(t, err)
	s.StartAsync()

	time.Sleep(350 * time.Millisecond)
	s.Stop()

	counterMutex.RLock()
	defer counterMutex.RUnlock()
	assert.Equal(t, 3, counter)
}

func TestScheduler_WaitForSchedules(t *testing.T) {
	s := NewScheduler(time.UTC)
	s.WaitForScheduleAll()

	var counterMutex sync.RWMutex
	counter := 0

	_, err := s.Every("1s").Do(func() {
		counterMutex.Lock()
		defer counterMutex.Unlock()
		counter++
	})
	require.NoError(t, err)

	_, err = s.CronWithSeconds("*/1 * * * * *").Do(func() {
		counterMutex.Lock()
		defer counterMutex.Unlock()
		counter++
	})
	require.NoError(t, err)
	s.StartAsync()

	time.Sleep(1050 * time.Millisecond)
	s.Stop()

	counterMutex.RLock()
	defer counterMutex.RUnlock()
	assert.GreaterOrEqual(t, counter, 2)
	assert.LessOrEqual(t, counter, 3)
}

func TestScheduler_LenWeekDays(t *testing.T) {
	testCases := []struct {
		description string
		weekDays    []time.Weekday
		finalLen    int
	}{
		{"no week day", []time.Weekday{}, 0},
		{"equal week day", []time.Weekday{time.Friday, time.Friday, time.Friday}, 1},
		{"more than one week day", []time.Weekday{time.Friday, time.Saturday, time.Sunday}, 3},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s := NewScheduler(time.UTC)
			s = s.Every(1)
			for _, weekDay := range tc.weekDays {
				s = s.Weekday(weekDay)
			}
			j, err := s.Do(func() {})
			require.NoError(t, err)
			assert.Equal(t, len(j.scheduledWeekdays), tc.finalLen)
		})
	}
}

func TestScheduler_CallNextWeekDay(t *testing.T) {
	januaryFirst2020At := func(hour, minute, second int) time.Time {
		return time.Date(2020, time.January, 1, hour, minute, second, 0, time.UTC)
	}

	const wantTimeUntilNextRun = time.Hour * 24 * 2
	lastRun := januaryFirst2020At(0, 0, 0)

	testCases := []struct {
		description string
		weekDays    []time.Weekday
	}{
		{"week days not in order", []time.Weekday{time.Monday, time.Friday}},
		{"week days in order", []time.Weekday{time.Friday, time.Monday}},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s := NewScheduler(time.UTC)
			s.Every(1)

			for _, weekDay := range tc.weekDays {
				s.Weekday(weekDay)
			}

			job, err := s.Do(func() {})
			require.NoError(t, err)
			job.lastRun = lastRun

			got := s.durationToNextRun(lastRun, job).duration
			assert.Equal(t, wantTimeUntilNextRun, got)
		})
	}
}

func TestScheduler_Midday(t *testing.T) {
	currentMidday := time.Date(2022, time.January, 0, 12, 0, 0, 0, time.UTC)
	expectedTime := 24 * time.Hour

	t.Run("check time till next midday", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		job, _ := s.Every(1).Day().Midday().Do(func() {})
		job.lastRun = currentMidday
		durationToNextTime := s.durationToNextRun(currentMidday, job).duration
		assert.Equal(t, expectedTime, durationToNextTime)
	})
}

func TestScheduler_EveryMonthFirstWeekday(t *testing.T) {
	testCases := []struct {
		current  time.Time
		expected time.Time
		weekday  time.Weekday
	}{
		{
			current:  time.Date(2022, time.March, 7, 0, 0, 0, 0, time.UTC),
			expected: time.Date(2022, time.April, 6, 0, 0, 0, 0, time.UTC),
			weekday:  time.Wednesday,
		},
		{
			current:  time.Date(2022, time.March, 1, 0, 0, 0, 0, time.UTC),
			expected: time.Date(2022, time.March, 3, 0, 0, 0, 0, time.UTC),
			weekday:  time.Thursday,
		},
		{
			current:  time.Date(2022, time.March, 23, 0, 0, 0, 0, time.UTC),
			expected: time.Date(2022, time.April, 4, 0, 0, 0, 0, time.UTC),
			weekday:  time.Monday,
		},
		{
			current:  time.Date(2022, time.February, 28, 0, 0, 0, 0, time.UTC),
			expected: time.Date(2022, time.March, 7, 0, 0, 0, 0, time.UTC),
			weekday:  time.Monday,
		},
		{
			current:  time.Date(2022, time.December, 7, 0, 0, 0, 0, time.UTC),
			expected: time.Date(2023, time.January, 2, 0, 0, 0, 0, time.UTC),
			weekday:  time.Monday,
		},
	}

	for _, tc := range testCases {
		t.Run("check first weekday of the month", func(t *testing.T) {
			s := NewScheduler(time.UTC)
			s.time = fakeTime{
				onNow: func(l *time.Location) time.Time {
					return tc.current
				},
			}
			job, _ := s.MonthFirstWeekday(tc.weekday).Do(func() {})
			job.lastRun = tc.current
			durationToNextTime := s.durationToNextRun(tc.current, job)
			assert.Equal(t, tc.expected, durationToNextTime.dateTime)
		})
	}
}

func TestScheduler_CheckNextWeekDay(t *testing.T) {
	januaryFirst2020At := func(hour, minute, second int) time.Time {
		return time.Date(2020, time.January, 1, hour, minute, second, 0, time.UTC)
	}
	januarySecond2020At := func(hour, minute, second int) time.Time {
		return time.Date(2020, time.January, 2, hour, minute, second, 0, time.UTC)
	}
	const (
		wantTimeUntilNextFirstRun = time.Second
		// all day long
		wantTimeUntilNextSecondRun = 24 * time.Hour
	)

	t.Run("check slice next run", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		lastRun := januaryFirst2020At(23, 59, 59)
		secondLastRun := januarySecond2020At(0, 0, 0)

		job, err := s.Every(1).Week().Friday().Thursday().Do(func() {})
		require.NoError(t, err)
		job.lastRun = lastRun

		gotFirst := s.durationToNextRun(lastRun, job).duration
		assert.Equal(t, wantTimeUntilNextFirstRun, gotFirst)

		job.lastRun = secondLastRun
		gotSecond := s.durationToNextRun(secondLastRun, job).duration
		assert.Equal(t, wantTimeUntilNextSecondRun, gotSecond)
	})
}

func TestScheduler_CheckEveryWeekHigherThanOne(t *testing.T) {
	januaryDay2020At := func(day int) time.Time {
		return time.Date(2020, time.January, day, 0, 0, 0, 0, time.UTC)
	}

	testCases := []struct {
		description string
		interval    int
		weekDays    []time.Weekday
		daysToTest  []int
		caseTest    int
	}{
		{description: "every two weeks after run the first scheduled task", interval: 2, weekDays: []time.Weekday{time.Thursday}, daysToTest: []int{1, 2, 15, 16}, caseTest: 1},
		{description: "every three weeks after run the first scheduled task", interval: 3, weekDays: []time.Weekday{time.Thursday}, daysToTest: []int{1, 2, 15, 16}, caseTest: 2},
		{description: "every two weeks after run the first 2 scheduled tasks", interval: 2, weekDays: []time.Weekday{time.Friday, time.Thursday}, daysToTest: []int{1, 2, 3, 15, 16, 17}, caseTest: 3},
	}

	const (
		wantTimeUntilNextRunOneDay = 24 * time.Hour
		// two weeks difference
		wantTimeUntilNextRunTwoWeeks = 24 * time.Hour * 14
		// three weeks difference
		wantTimeUntilNextRunThreeWeeks = 24 * time.Hour * 21
		// two weeks difference less one day
		wantTimeUntilNextRunTwoWeeksLessOneDay = 24 * time.Hour * (14 - 1)
	)

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s := NewScheduler(time.UTC)
			s.Every(tc.interval)

			for _, weekDay := range tc.weekDays {
				s.Weekday(weekDay)
			}
			job, err := s.Do(func() {})
			require.NoError(t, err)
			for _, day := range tc.daysToTest {
				lastRun := januaryDay2020At(day)

				job.lastRun = lastRun
				got := s.durationToNextRun(lastRun, job).duration

				jobWeekdays := job.Weekdays()
				if lastRun.Weekday() < jobWeekdays[len(jobWeekdays)-1] {
					assert.Equal(t, wantTimeUntilNextRunOneDay, got)
				} else {
					if tc.caseTest == 1 {
						assert.Equal(t, wantTimeUntilNextRunTwoWeeks, got)
					} else if tc.caseTest == 2 {
						assert.Equal(t, wantTimeUntilNextRunThreeWeeks, got)
					} else if tc.caseTest == 3 {
						assert.Equal(t, wantTimeUntilNextRunTwoWeeksLessOneDay, got)
					}
				}
				job.runStartCount.Add(1)
			}
		})
	}
}

func TestScheduler_StartImmediately(t *testing.T) {
	testCases := []struct {
		description                string
		scheduler                  *Scheduler
		expectedToStartImmediately bool
	}{
		{"true cron", NewScheduler(time.UTC).Cron("0 0 * * 6,0").StartImmediately(), true},
		{"true default", NewScheduler(time.UTC).Every("1m"), true},
		{"true every", NewScheduler(time.UTC).Every("1m").StartImmediately(), true},
		{"false cron default", NewScheduler(time.UTC).Cron("0 0 * * 6,0"), false},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			j, err := tc.scheduler.Do(func() {})
			require.NoError(t, err)

			assert.Exactly(t, tc.expectedToStartImmediately, j.startsImmediately)
		})
	}
}

func TestScheduler_CheckCalculateDaysOfMonth(t *testing.T) {
	lastRunFirstCaseDate := time.Date(2021, 9, 10, 0, 0, 0, 0, time.UTC)
	lastRunSecondCaseDate := time.Date(2021, 9, 10, 5, 0, 0, 0, time.UTC)
	lastRunThirdCaseDate := time.Date(2021, 10, 6, 0, 0, 0, 0, time.UTC)
	lastRunFourthCaseDate := time.Date(2021, 9, 11, 0, 0, 0, 0, time.UTC)

	curTime := fakeTime{onNow: func(l *time.Location) time.Time {
		return time.Date(2021, 9, 8, 0, 0, 0, 0, l)
	}}

	testCases := []struct {
		description          string
		job                  *Job
		wantTimeUntilNextRun time.Duration
	}{
		{description: "should run current month 10", job: calculateNextRunHelper(1, months, curTime.Now(time.UTC), []time.Duration{_getHours(0)}, nil, []int{10, 6}), wantTimeUntilNextRun: lastRunFirstCaseDate.Sub(curTime.Now(time.UTC))},
		{description: "should run current month 10", job: calculateNextRunHelper(1, months, curTime.Now(time.UTC), []time.Duration{_getHours(5)}, nil, []int{10, 6}), wantTimeUntilNextRun: lastRunSecondCaseDate.Sub(curTime.Now(time.UTC))},
		{description: "should run next month 6", job: calculateNextRunHelper(1, months, curTime.Now(time.UTC), []time.Duration{_getHours(0)}, nil, []int{6, 7}), wantTimeUntilNextRun: lastRunThirdCaseDate.Sub(curTime.Now(time.UTC))},
		{description: "should run next month 11", job: calculateNextRunHelper(1, months, curTime.Now(time.UTC), []time.Duration{_getHours(0)}, nil, []int{12, 11}), wantTimeUntilNextRun: lastRunFourthCaseDate.Sub(curTime.Now(time.UTC))},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s := NewScheduler(time.UTC)
			s.time = curTime
			got := s.durationToNextRun(tc.job.LastRun(), tc.job).duration
			assert.Equalf(t, tc.wantTimeUntilNextRun, got, fmt.Sprintf("expected %s / got %s", tc.wantTimeUntilNextRun.String(), got.String()))
		})
	}
}

func TestScheduler_CheckSetBehaviourBeforeJobCreated(t *testing.T) {
	s := NewScheduler(time.UTC)
	_, err := s.Month(1, 2).Every(1).Do(func() {})
	assert.NoError(t, err)
}

func TestScheduler_MonthLastDayAtTime(t *testing.T) {
	testCases := []struct {
		name                 string
		job                  *Job
		wantTimeUntilNextRun time.Duration
	}{
		{
			name:                 "month last day before run at time",
			job:                  calculateNextRunHelper(1, months, time.Date(2022, 2, 28, 10, 0, 0, 0, time.UTC), []time.Duration{_getHours(20) + _getMinutes(0)}, nil, []int{-1}),
			wantTimeUntilNextRun: _getHours(10),
		},
		{
			name:                 "month last day penultimate day before run at time",
			job:                  calculateNextRunHelper(1, months, time.Date(2022, 2, 27, 10, 0, 0, 0, time.UTC), []time.Duration{_getHours(20) + _getMinutes(0)}, nil, []int{-2}),
			wantTimeUntilNextRun: _getHours(10),
		},
		{
			name:                 "month last day 2 days before last day before run at time",
			job:                  calculateNextRunHelper(1, months, time.Date(2022, 2, 26, 10, 0, 0, 0, time.UTC), []time.Duration{_getHours(20) + _getMinutes(0)}, nil, []int{-3}),
			wantTimeUntilNextRun: _getHours(10),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewScheduler(time.UTC)
			got := s.durationToNextRun(tc.job.LastRun(), tc.job).duration
			assert.Equalf(t, tc.wantTimeUntilNextRun, got, fmt.Sprintf("expected %s / got %s", tc.wantTimeUntilNextRun.String(), got.String()))
		})
	}
}

func TestScheduler_WeekdayIsCurrentDay(t *testing.T) {
	s := NewScheduler(time.UTC)
	s.time = fakeTime{onNow: func(l *time.Location) time.Time {
		return time.Date(2022, 2, 17, 20, 0, 0, 0, l)
	}}
	s.StartAsync()

	job, _ := s.Every(1).Week().Thursday().Friday().Saturday().At("23:00").Do(func() {})
	assert.Equal(t, time.Date(2022, 2, 17, 23, 0, 0, 0, s.Location()), job.NextRun())
}

func TestScheduler_MultipleAtTime(t *testing.T) {
	getTime := func(hour, min, sec int) time.Time {
		return time.Date(2022, 2, 16, hour, min, sec, 0, time.UTC)
	}

	getMonthLastDayTime := func(hour, min, sec int) time.Time {
		return time.Date(2022, 2, 28, hour, min, sec, 0, time.UTC)
	}

	atTimes := []time.Duration{
		_getHours(3) + _getMinutes(20),
		_getHours(5) + _getMinutes(30),
		_getHours(7) + _getMinutes(0),
		_getHours(14) + _getMinutes(10),
	}
	testCases := []struct {
		description          string
		job                  *Job
		wantTimeUntilNextRun time.Duration
	}{
		{description: "day test1", job: calculateNextRunHelper(1, days, getTime(1, 0, 0), atTimes, nil, nil), wantTimeUntilNextRun: _getHours(2) + _getMinutes(20)},
		{description: "day test2", job: calculateNextRunHelper(1, days, getTime(3, 30, 0), atTimes, nil, nil), wantTimeUntilNextRun: _getHours(2)},
		{description: "day test3", job: calculateNextRunHelper(1, days, getTime(5, 27, 10), atTimes, nil, nil), wantTimeUntilNextRun: _getMinutes(2) + _getSeconds(50)},
		{description: "day test4", job: calculateNextRunHelper(1, days, getTime(5, 30, 0), atTimes, nil, nil), wantTimeUntilNextRun: _getHours(1) + _getMinutes(30)},
		{description: "day test5", job: calculateNextRunHelper(1, days, getTime(15, 0, 0), atTimes, nil, nil), wantTimeUntilNextRun: _getHours(12) + _getMinutes(20)},
		{description: "day test6", job: calculateNextRunHelper(2, days, getTime(15, 0, 0), atTimes, nil, nil), wantTimeUntilNextRun: _getHours(36) + _getMinutes(20)},
		{description: "day test7", job: calculateNextRunHelper(2, days, getTime(1, 0, 0), atTimes, nil, nil), wantTimeUntilNextRun: _getHours(2) + _getMinutes(20)},
		{description: "week test1", job: calculateNextRunHelper(1, weeks, getTime(5, 30, 0), atTimes, nil, nil), wantTimeUntilNextRun: _getHours(1) + _getMinutes(30)},
		{description: "week test2", job: calculateNextRunHelper(1, weeks, getTime(15, 0, 0), atTimes, nil, nil), wantTimeUntilNextRun: _getDays(7) - _getHours(15) + _getHours(3) + _getMinutes(20)},
		{description: "week test2", job: calculateNextRunHelper(2, weeks, getTime(1, 0, 0), atTimes, nil, nil), wantTimeUntilNextRun: _getHours(2) + _getMinutes(20)},
		{description: "weekday before test1", job: calculateNextRunHelper(1, weeks, getTime(5, 30, 0), atTimes, []time.Weekday{time.Tuesday}, nil), wantTimeUntilNextRun: _getDays(6) - _getHours(2) - _getMinutes(10)},
		{description: "weekday before test2", job: calculateNextRunHelper(1, weeks, getTime(15, 0, 0), atTimes, []time.Weekday{time.Tuesday}, nil), wantTimeUntilNextRun: _getDays(6) - _getHours(15) + _getHours(3) + _getMinutes(20)},
		{description: "weekday equals test1", job: calculateNextRunHelper(1, weeks, getTime(5, 30, 0), atTimes, []time.Weekday{time.Wednesday}, nil), wantTimeUntilNextRun: _getHours(1) + _getMinutes(30)},
		{description: "weekday equals test2", job: calculateNextRunHelper(1, weeks, getTime(15, 0, 0), atTimes, []time.Weekday{time.Wednesday}, nil), wantTimeUntilNextRun: _getDays(6) + _getHours(9) + _getHours(3) + _getMinutes(20)},
		{description: "weekday after test1", job: calculateNextRunHelper(1, weeks, getTime(5, 30, 0), atTimes, []time.Weekday{time.Thursday}, nil), wantTimeUntilNextRun: _getDays(1) - _getHours(2) - _getMinutes(10)},
		{description: "weekday after test2", job: calculateNextRunHelper(1, weeks, getTime(15, 0, 0), atTimes, []time.Weekday{time.Thursday}, nil), wantTimeUntilNextRun: _getDays(1) - _getHours(15) + _getHours(3) + _getMinutes(20)},
		{description: "month test1", job: calculateNextRunHelper(1, months, getTime(5, 30, 0), atTimes, nil, []int{1}), wantTimeUntilNextRun: _getDays(13) - _getHours(2) - _getMinutes(10)},
		{description: "month test2", job: calculateNextRunHelper(1, months, getTime(15, 0, 0), atTimes, nil, []int{1}), wantTimeUntilNextRun: _getDays(12) + _getHours(9) + _getHours(3) + _getMinutes(20)},
		{description: "month last day test1", job: calculateNextRunHelper(1, months, getMonthLastDayTime(5, 30, 0), atTimes, nil, []int{-1}), wantTimeUntilNextRun: _getHours(1) + _getMinutes(30)},
		{description: "month last day test2", job: calculateNextRunHelper(1, months, getMonthLastDayTime(15, 0, 0), atTimes, nil, []int{-1}), wantTimeUntilNextRun: _getDays(30) + _getHours(9) + _getHours(3) + _getMinutes(20)},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			ft := fakeTime{onNow: func(l *time.Location) time.Time {
				return tc.job.LastRun()
			}}

			s := NewScheduler(time.UTC)
			s.time = ft

			got := s.durationToNextRun(tc.job.LastRun(), tc.job).duration
			assert.Equalf(t, tc.wantTimeUntilNextRun, got, fmt.Sprintf("expected %s / got %s", tc.wantTimeUntilNextRun.String(), got.String()))
		})
	}
}

func TestScheduler_DoWithJobDetails(t *testing.T) {
	testCases := []struct {
		description   string
		jobFunc       interface{}
		params        []interface{}
		expectedError string
	}{
		{"no error", func(foo, bar string, job Job) {}, []interface{}{"foo", "bar"}, ""},
		{"too few params", func(foo, bar string, job Job) {}, []interface{}{"foo"}, ErrWrongParams.Error()},
		{"too many params", func(foo, bar string, job Job) {}, []interface{}{"foo", "bar", "baz"}, ErrWrongParams.Error()},
		{"jobFunc doesn't have Job param", func(foo, bar string) {}, []interface{}{"foo"}, ErrDoWithJobDetails.Error()},
		{"jobFunc has Job param but not last param", func(job Job, foo, bar string) {}, []interface{}{"foo", "bar"}, ErrDoWithJobDetails.Error()},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s := NewScheduler(time.UTC)
			_, err := s.Every("1s").DoWithJobDetails(tc.jobFunc, tc.params...)
			if tc.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.expectedError)
			}
		})
	}

	t.Run("run job with details", func(t *testing.T) {
		s := NewScheduler(time.UTC)

		_, err := s.Tag("tag1").Every("100ms").DoWithJobDetails(func(job Job) {})
		require.NoError(t, err)
		s.StartAsync()
		time.Sleep(500 * time.Millisecond)
	})
}

func TestScheduler_GetAllTags_Unique(t *testing.T) {
	testCases := []struct {
		description   string
		tags          []string
		expected      []string
		expectedError error
	}{
		{"unique: no tags", []string{}, nil, nil},
		{"unique: one tag", []string{"tag1"}, []string{"tag1"}, nil},
		{"unique: two tags", []string{"tag1", "tag2"}, []string{"tag1", "tag2"}, nil},
		{"unique: two tags with duplicates", []string{"tag1", "tag2", "tag1"}, nil, ErrTagsUnique("tag1")},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s := NewScheduler(time.UTC)
			s.TagsUnique()

			_, err := s.Tag(tc.tags...).Every("100ms").Do(func() {})
			if tc.expectedError == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.expectedError.Error())
			}

			tags := s.GetAllTags()
			sort.Strings(tc.expected)
			sort.Strings(tags)

			assert.Equal(t, tc.expected, tags)
		})
	}
}

func TestScheduler_GetAllTags_NotUnique(t *testing.T) {
	testCases := []struct {
		description string
		tags        []string
		expected    []string
	}{
		{"no tags", []string{}, nil},
		{"one tag", []string{"tag1"}, []string{"tag1"}},
		{"two tags", []string{"tag1", "tag2"}, []string{"tag1", "tag2"}},
		{"two tags with duplicates", []string{"tag1", "tag2", "tag1"}, []string{"tag1", "tag2", "tag1"}},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s := NewScheduler(time.UTC)

			_, err := s.Tag(tc.tags...).Every("100ms").Do(func() {})
			require.NoError(t, err)

			tags := s.GetAllTags()
			sort.Strings(tc.expected)
			sort.Strings(tags)

			assert.Equal(t, tc.expected, tags)
		})
	}
}

func TestScheduler_ChainOrder(t *testing.T) {
	s := NewScheduler(time.UTC)

	func1 := func() { panic("func 1 not implemented") }
	func2 := func() { panic("func 2 not implemented") }
	func3 := func() { panic("func 3 not implemented") }

	_, err := s.Tag("1").SingletonMode().Milliseconds().EveryRandom(100, 200).Do(func1)
	require.NoError(t, err)

	_, err = s.Monday().Every(4).Tag("2").Do(func2)
	require.NoError(t, err)

	_, err = s.Months(1).Tag("3").Every(1).SingletonMode().At("1:00").Do(func3)
	require.NoError(t, err)

	require.Len(t, s.jobs, 3)

	err = s.RemoveByTag("2")
	require.NoError(t, err)

	require.Len(t, s.jobs, 2)
}

func TestScheduler_Register_Event(t *testing.T) {
	userDefinedError := errors.New("user defined error")
	testCases := []struct {
		description   string
		jobFunc       func() error
		expected      []uint8
		expectedError error
	}{
		{"event order: no error", func() error { return nil }, []uint8{1, 2, 3, 4}, nil},
		{"event order: on error", func() error { return userDefinedError }, []uint8{1, 2, 3, 4}, userDefinedError},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s := NewScheduler(time.UTC)
			order := make(chan uint8, 10)
			done := make(chan struct{})
			var expectedErr error
			s = s.BeforeJobRuns(func(jobName string) {
				order <- 1
			})
			s = s.WhenJobReturnsError(func(jobName string, err error) {
				order <- 3
				expectedErr = err
			})
			s = s.WhenJobReturnsNoError(func(jobName string) {
				order <- 3
			})
			s = s.AfterJobRuns(func(jobName string) {
				order <- 4
				done <- struct{}{}
			})

			_, err := s.Day().Every(1).StartImmediately().Do(func() error {
				order <- 2
				return tc.jobFunc()
			})

			require.NoError(t, err)
			s.StartAsync()
			select {
			case <-done:
				assert.Equal(t, tc.expectedError, expectedErr)
				assert.Equal(t, len(tc.expected), len(order))
				if len(tc.expected) == len(order) {
					for i := 0; i < len(tc.expected); i++ {
						assert.Equal(t, tc.expected[i], <-order)
					}
				}
				s.Clear()
			case <-time.After(1 * time.Second):
				t.Fatal("timeout")
			}
		})
	}
}

var _ Locker = (*locker)(nil)

type locker struct {
	mu    sync.Mutex
	store map[string]struct{}
}

func (l *locker) Lock(_ context.Context, key string) (Lock, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.store[key]; ok {
		return nil, ErrFailedToObtainLock
	}
	l.store[key] = struct{}{}
	return &lock{key: key, locker: l}, nil
}

var _ Lock = (*lock)(nil)

type lock struct {
	key    string
	locker *locker
}

func (l *lock) Unlock(_ context.Context) error {
	l.locker.mu.Lock()
	defer l.locker.mu.Unlock()
	delete(l.locker.store, l.key)
	return nil
}

func TestScheduler_EnableDistributedLocking(t *testing.T) {
	runTestWithDistributedLocking(t, 0)
}

func TestScheduler_EnableMaxConcurrentJobs(t *testing.T) {
	runTestWithDistributedLocking(t, 10)
}

func runTestWithDistributedLocking(t *testing.T, maxConcurrentJobs int) {
	resultChan := make(chan int, 10)
	f := func(schedulerInstance int) {
		resultChan <- schedulerInstance
		time.Sleep(100 * time.Millisecond)
	}

	l := &locker{
		store: make(map[string]struct{}, 0),
	}

	schedulers := make([]*Scheduler, 0)
	for i := 0; i < 3; i++ {
		s := NewScheduler(time.UTC)
		s.WithDistributedLocker(l)
		if maxConcurrentJobs > 0 {
			s.SetMaxConcurrentJobs(maxConcurrentJobs, WaitMode)
		}
		_, err := s.Every("500ms").Do(f, 1)
		require.NoError(t, err)
		schedulers = append(schedulers, s)
	}
	for i := range schedulers {
		schedulers[i].StartAsync()
	}
	time.Sleep(1700 * time.Millisecond)
	for i := range schedulers {
		schedulers[i].Stop()
	}
	close(resultChan)

	var results []int
	for r := range resultChan {
		results = append(results, r)
	}
	assert.Len(t, results, 4)
}

func TestWithDistributedLockingBlocking(t *testing.T) {
	var (
		maxConcurrentJobs = 1
		counter           = 10
		resultChan        = make(chan time.Time, 20)
	)

	f := func() {
		if counter == 0 {
			close(resultChan)
			return
		}

		resultChan <- time.Now()
		counter--
	}

	l := &locker{
		store: make(map[string]struct{}, 0),
	}

	s := NewScheduler(time.UTC)
	s.WithDistributedLocker(l)
	s.SetMaxConcurrentJobs(maxConcurrentJobs, WaitMode)

	s.Every(1).Seconds().Do(func() {})
	s.Every(150).Milliseconds().Do(f)
	s.StartAsync()

	last := time.Now()
	for ts := range resultChan {
		assert.True(t, ts.Sub(last) <= 200*time.Millisecond)
		last = ts
	}

	s.Stop()
}

func TestScheduler_WithDistributedLocker_With_Name(t *testing.T) {
	testCases := []struct {
		description string
		name        string
		expected    int
	}{
		{"with job name set", "foo", 4},
		// The distributed locking will use the function funcName as its lock key
		// if the job funcName is empty. So the result will be 8 instead of 4 since
		// they are regarded as different jobs.
		{"without job name", "", 8},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			resultChan := make(chan int, 10)
			f1 := func(schedulerInstance int) {
				resultChan <- schedulerInstance
				time.Sleep(100 * time.Millisecond)
			}
			f2 := func(schedulerInstance int) {
				resultChan <- schedulerInstance
				time.Sleep(100 * time.Millisecond)
			}

			l := &locker{
				store: make(map[string]struct{}, 0),
			}

			schedulers := make([]*Scheduler, 0)

			var err error
			var job *Job
			for i := 0; i < 2; i++ {
				s := NewScheduler(time.UTC)
				s.WithDistributedLocker(l)
				if i%2 == 0 {
					job, err = s.Every("500ms").Name(tc.name).Do(f1, 1)
				} else {
					job, err = s.Every("500ms").Name(tc.name).Do(f2, 1)
				}
				require.NoError(t, err)
				require.Equal(t, job.jobName, tc.name)
				schedulers = append(schedulers, s)
			}
			for i := range schedulers {
				schedulers[i].StartAsync()
			}
			time.Sleep(1700 * time.Millisecond)
			for i := range schedulers {
				schedulers[i].Stop()
			}
			close(resultChan)

			var results []int
			for r := range resultChan {
				results = append(results, r)
			}
			assert.Len(t, results, tc.expected)
		})
	}
}

func TestScheduler_PauseJobExecution(t *testing.T) {
	s := NewScheduler(time.UTC)
	var counter int
	var mu sync.Mutex

	_, err := s.Every("100ms").Do(func() {
		mu.Lock()
		counter++
		mu.Unlock()
	})
	require.NoError(t, err)

	s.StartAsync()
	time.Sleep(50 * time.Millisecond)

	s.PauseJobExecution(true)
	time.Sleep(200 * time.Millisecond)

	s.PauseJobExecution(false)
	time.Sleep(100 * time.Millisecond)
	s.Stop()

	mu.Lock()
	defer mu.Unlock()
	assert.GreaterOrEqual(t, counter, 1)
	assert.LessOrEqual(t, counter, 2)
}

func TestDataRace(t *testing.T) {
	// This used to fail with the -race flag
	sut := NewScheduler(time.UTC)
	sut.StartAsync()
	maxRuns := 10
	task := func() {
	}

	_, err := sut.
		Every(1).
		Milliseconds().
		Tag("name").
		SingletonMode().
		LimitRunsTo(maxRuns).
		Do(task)
	assert.NoError(t, err)

	time.Sleep(1 * time.Second)
	sut.Stop()
}

var _ Elector = (*elector)(nil)

type elector struct {
	isLeader bool
}

func (e *elector) IsLeader(_ context.Context) error {
	if e.isLeader {
		return nil
	}
	return errors.New("is not leader")
}

func (e *elector) setLeader() {
	e.isLeader = true
}

func TestScheduler_EnableDistributedElector(t *testing.T) {
	runTestWithDistributedElector(t, 0)
}

func TestScheduler_EnableDistributedElectorWithMaxConcurrent(t *testing.T) {
	runTestWithDistributedElector(t, 1)
}

func runTestWithDistributedElector(t *testing.T, maxConcurrentJobs int) {
	resultChan := make(chan int, 20)
	f := func(schedulerInstance int) {
		resultChan <- schedulerInstance
	}

	leaderIndex := 0
	schedulers := make([]*Scheduler, 0)
	for i := 0; i < 3; i++ {
		el := &elector{}
		if i == leaderIndex {
			el.setLeader()
		}

		s := NewScheduler(time.UTC)
		s.WithDistributedElector(el)
		if maxConcurrentJobs > 0 {
			s.SetMaxConcurrentJobs(maxConcurrentJobs, WaitMode)
		}
		_, err := s.Every("50ms").Do(f, i)
		require.NoError(t, err)
		schedulers = append(schedulers, s)
	}
	for i := range schedulers {
		schedulers[i].StartAsync()
	}
	time.Sleep(530 * time.Millisecond)
	for i := range schedulers {
		schedulers[i].Stop()
	}
	close(resultChan)

	// 10 <- len <- 12
	assert.Greater(t, len(resultChan), 10)
	assert.Less(t, len(resultChan), 12)
	for r := range resultChan {
		assert.Equal(t, leaderIndex, r)
	}
}
