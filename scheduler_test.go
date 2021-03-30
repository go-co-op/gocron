package gocron

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ timeWrapper = (*fakeTime)(nil)

type fakeTime struct {
	onNow func(location *time.Location) time.Time
}

func (f fakeTime) Now(loc *time.Location) time.Time {
	return f.onNow(loc)
}

func (f fakeTime) Unix(i int64, i2 int64) time.Time {
	panic("implement me")
}

func (f fakeTime) Sleep(duration time.Duration) {
	panic("implement me")
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
	case <-time.After(1 * time.Second):
		t.Fatal("job did not run immediately")
	case <-semaphore:
		// test passed
	}

}

func TestInvalidEveryInterval(t *testing.T) {
	testCases := []struct {
		description   string
		interval      interface{}
		expectedError string
	}{
		{"zero", 0, ErrInvalidInterval.Error()},
		{"negative", -1, ErrInvalidInterval.Error()},
		{"invalid string duration", "bad", "time: invalid duration"},
	}

	s := NewScheduler(time.UTC)

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			_, err := s.Every(tc.interval).Do(func() {})
			require.Error(t, err)
			// wonky way to assert on the error message, but between go 1.14
			// and go 1.15 the error value was wrapped in quotes
			assert.True(t, strings.HasPrefix(err.Error(), tc.expectedError))
		})
	}

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

		_, err := s.Every(1).Second().Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)

		s.StartAsync()

		var counter int

		now := time.Now()
		for time.Now().Before(now.Add(1 * time.Second)) {
			if <-semaphore {
				counter++
			}
		}
		s.Stop()
		assert.Equal(t, 2, counter)
	})

	t.Run("string duration", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		semaphore := make(chan bool)

		_, err := s.Every("1s").Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)

		s.StartAsync()

		var counter int

		now := time.Now()
		for time.Now().Before(now.Add(1 * time.Second)) {
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
		interval           = 2
		expectedExecutions = 4
		mu                 sync.RWMutex
	)

	runTime := 6 * time.Second
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

		nextMinuteTime := now.Add(1 * time.Minute)
		startAt := fmt.Sprintf("%02d:%02d:%02d", nextMinuteTime.Hour(), nextMinuteTime.Minute(), nextMinuteTime.Second())
		dayJob, err := s.Every(1).Day().At(startAt).Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)

		s.StartAsync()
		time.Sleep(1 * time.Second)

		select {
		case <-time.After(1 * time.Second):
			assert.Equal(t, now.Add(1*time.Minute), dayJob.nextRun)
		case <-semaphore:
			t.Fatal("job ran even though scheduled in future")
		}
	})

	t.Run("error due to bad time format", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		badTime := "0:0"
		_, err := s.Every(1).Day().At(badTime).Do(func() {})
		assert.EqualError(t, err, ErrUnsupportedTimeFormat.Error())
		assert.Zero(t, s.Len())
	})
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

func TestScheduler_Remove(t *testing.T) {

	t.Run("remove from non-running", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		_, err := s.Every(1).Minute().Do(task)
		require.NoError(t, err)
		_, err = s.Every(1).Minute().Do(taskWithParams, 1, "hello")
		require.NoError(t, err)
		_, err = s.Every(1).Minute().Do(task)
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

		j, err := s.Every(1).Seconds().StartAt(s.time.Now(s.location).Add(time.Second)).Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)

		s.StartAsync()

		s.Remove(j.functions[j.name])

		select {
		case <-time.After(2 * time.Second):
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

		s.RemoveByReference(job1)
		assert.ElementsMatch(t, []*Job{job2}, s.Jobs())
	})

	t.Run("remove from running scheduler", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		semaphore := make(chan bool)

		j, err := s.Every(1).Seconds().StartAt(s.time.Now(s.location).Add(time.Second)).Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)

		s.StartAsync()

		s.RemoveByReference(j)

		select {
		case <-time.After(2 * time.Second):
			// test passed
		case <-semaphore:
			t.Fatal("job ran after being removed")
		}
	})
}

func TestScheduler_RemoveByTag(t *testing.T) {
	s := NewScheduler(time.UTC)

	// Creating 2 Jobs with Unique tags
	tag1 := "tag one"
	tag2 := "tag two"
	_, err := s.Every(1).Second().Tag(tag1).Do(taskWithParams, 1, "hello") // index 0
	require.NoError(t, err)
	_, err = s.Every(1).Second().Tag(tag2).Do(taskWithParams, 2, "world") // index 1
	require.NoError(t, err)

	// check Jobs()[0] tags is equal with tag "tag one" (tag1)
	assert.Equal(t, s.Jobs()[0].Tags()[0], tag1, "Job With Tag 'tag one' is removed from index 0")

	err = s.RemoveByTag("tag one")
	require.NoError(t, err)
	assert.Equal(t, 1, s.Len(), "Incorrect number of jobs after removing 1 job")

	// check Jobs()[0] tags is equal with tag "tag two" (tag2) after removing "tag one"
	assert.Equal(t, s.Jobs()[0].Tags()[0], tag2, "Job With Tag 'tag two' is removed from index 0")

	// Removing Non Existent Job with "tag one" because already removed above (will not removing any jobs because tag not match)
	err = s.RemoveByTag("tag one")
	assert.EqualError(t, err, ErrJobNotFoundWithTag.Error())
}

func TestScheduler_Jobs(t *testing.T) {
	s := NewScheduler(time.UTC)
	s.Every(1).Minute().Do(task)
	s.Every(2).Minutes().Do(task)
	s.Every(3).Minutes().Do(task)
	s.Every(4).Minutes().Do(task)
	js := s.Jobs()
	assert.Len(t, js, 4)
}

func TestScheduler_Len(t *testing.T) {
	s := NewScheduler(time.UTC)
	s.Every(1).Minute().Do(task)
	s.Every(2).Minutes().Do(task)
	s.Every(3).Minutes().Do(task)
	s.Every(4).Minutes().Do(task)
	l := s.Len()

	assert.Equal(t, l, 4)
}

func TestSwap(t *testing.T) {
	s := NewScheduler(time.UTC)
	_, err := s.Every(1).Minute().Do(task)
	require.NoError(t, err)
	_, err = s.Every(2).Minute().Do(task)
	require.NoError(t, err)

	jb := s.Jobs()
	var jobsBefore []*Job
	jobsBefore = append(jobsBefore, jb...)

	s.Swap(1, 0)

	jobsAfter := s.Jobs()

	assert.Equal(t, jobsBefore[0], jobsAfter[1])
	assert.Equal(t, jobsBefore[1], jobsAfter[0])
}

func TestLess(t *testing.T) {
	s := NewScheduler(time.UTC)
	s.Every(1).Minute().Do(task)
	s.Every(2).Minute().Do(task)

	assert.True(t, s.Less(0, 1))
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

	_, err := s.Every(1).Second().Do(func() {
		semaphore <- true
	})
	require.NoError(t, err)

	s.StartAsync()

	s.Clear()
	assert.Equal(t, 0, s.Len())

	var counter int
	now := time.Now()
	for time.Now().Before(now.Add(1 * time.Second)) {
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

func TestSetUnit(t *testing.T) {

	testCases := []struct {
		desc     string
		timeUnit timeUnit
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
			switch tc.timeUnit {
			case seconds:
				s.Every(2).Seconds().Do(task)
			case minutes:
				s.Every(2).Minutes().Do(task)
			case hours:
				s.Every(2).Hours().Do(task)
			case days:
				s.Every(2).Days().Do(task)
			case weeks:
				s.Every(2).Weeks().Do(task)
			}
			j := s.jobs[0]

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
		time.Sleep(1 * time.Millisecond) // wait for stop goroutine to catch up
		assert.False(t, s.IsRunning())
	})
	t.Run("noop on stopped scheduler", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		s.Stop()
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

		s.Every(1).Day().StartAt(s.time.Now(s.location).Add(time.Second)).Do(func() {
			semaphore <- true
		})

		s.StartAsync()

		select {
		case <-time.After(2 * time.Second):
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
}

func TestScheduler_CalculateNextRun(t *testing.T) {
	day := time.Hour * 24
	januaryFirst2020At := func(hour, minute, second int) time.Time {
		return time.Date(2020, time.January, 1, hour, minute, second, 0, time.UTC)
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
		{name: "every second test", job: &Job{interval: 1, unit: seconds, lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: _getSeconds(1)},
		{name: "every 62 seconds test", job: &Job{interval: 62, unit: seconds, lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: _getSeconds(62)},
		// MINUTES
		{name: "every minute test", job: &Job{interval: 1, unit: minutes, lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: _getMinutes(1)},
		{name: "every 62 minutes test", job: &Job{interval: 62, unit: minutes, lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: _getMinutes(62)},
		// HOURS
		{name: "every hour test", job: &Job{interval: 1, unit: hours, lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: _getHours(1)},
		{name: "every 25 hours test", job: &Job{interval: 25, unit: hours, lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: _getHours(25)},
		// DAYS
		{name: "every day at midnight", job: &Job{interval: 1, unit: days, lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: 1 * day},
		{name: "every day at 09:30AM with scheduler starting before 09:30AM should run at same day at time", job: &Job{interval: 1, unit: days, atTime: _getHours(9) + _getMinutes(30), lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: _getHours(9) + _getMinutes(30)},
		{name: "every day at 09:30AM which just ran should run tomorrow at 09:30AM", job: &Job{interval: 1, unit: days, atTime: _getHours(9) + _getMinutes(30), lastRun: januaryFirst2020At(9, 30, 0)}, wantTimeUntilNextRun: 1 * day},
		{name: "every 31 days at midnight should run 31 days later", job: &Job{interval: 31, unit: days, lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: 31 * day},
		{name: "daily job just ran at 8:30AM and should be scheduled for next day's 8:30AM", job: &Job{interval: 1, unit: days, atTime: 8*time.Hour + 30*time.Minute, lastRun: januaryFirst2020At(8, 30, 0)}, wantTimeUntilNextRun: 24 * time.Hour},
		{name: "daily job just ran at 5:30AM and should be scheduled for today at 8:30AM", job: &Job{interval: 1, unit: days, atTime: 8*time.Hour + 30*time.Minute, lastRun: januaryFirst2020At(5, 30, 0)}, wantTimeUntilNextRun: 3 * time.Hour},
		{name: "job runs every 2 days, just ran at 5:30AM and should be scheduled for 2 days at 8:30AM", job: &Job{interval: 2, unit: days, atTime: 8*time.Hour + 30*time.Minute, lastRun: januaryFirst2020At(5, 30, 0)}, wantTimeUntilNextRun: (2 * day) + 3*time.Hour},
		{name: "job runs every 2 days, just ran at 8:30AM and should be scheduled for 2 days at 8:30AM", job: &Job{interval: 2, unit: days, atTime: 8*time.Hour + 30*time.Minute, lastRun: januaryFirst2020At(8, 30, 0)}, wantTimeUntilNextRun: 2 * day},
		//// WEEKS
		{name: "every week should run in 7 days", job: &Job{interval: 1, unit: weeks, lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: 7 * day},
		{name: "every week with .At time rule should run respect .At time rule", job: &Job{interval: 1, atTime: _getHours(9) + _getMinutes(30), unit: weeks, lastRun: januaryFirst2020At(9, 30, 0)}, wantTimeUntilNextRun: 7 * day},
		{name: "every two weeks at 09:30AM should run in 14 days at 09:30AM", job: &Job{interval: 2, unit: weeks, lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: 14 * day},
		{name: "every 31 weeks ran at jan 1st at midnight should run at August 5, 2020", job: &Job{interval: 31, unit: weeks, lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: 31 * 7 * day},
		// MONTHS
		{name: "every month in a 31 days month should be scheduled for 31 days ahead", job: &Job{interval: 1, unit: months, lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: 31 * day},
		{name: "every month in a 30 days month should be scheduled for 30 days ahead", job: &Job{interval: 1, unit: months, lastRun: time.Date(2020, time.April, 1, 0, 0, 0, 0, time.UTC)}, wantTimeUntilNextRun: 30 * day},
		{name: "every month at february on leap year should count 29 days", job: &Job{interval: 1, unit: months, lastRun: time.Date(2020, time.February, 1, 0, 0, 0, 0, time.UTC)}, wantTimeUntilNextRun: 29 * day},
		{name: "every month at february on non leap year should count 28 days", job: &Job{interval: 1, unit: months, lastRun: time.Date(2019, time.February, 1, 0, 0, 0, 0, time.UTC)}, wantTimeUntilNextRun: 28 * day},
		{name: "every month at first day at time should run next month + at time", job: &Job{interval: 1, unit: months, atTime: _getHours(9) + _getMinutes(30), lastRun: januaryFirst2020At(9, 30, 0)}, wantTimeUntilNextRun: 31*day + _getHours(9) + _getMinutes(30)},
		{name: "every month at day should consider at days", job: &Job{interval: 1, unit: months, dayOfTheMonth: 2, lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: 1 * day},
		{name: "every month at day should consider at hours", job: &Job{interval: 1, unit: months, atTime: _getHours(9) + _getMinutes(30), lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: 31*day + _getHours(9) + _getMinutes(30)},
		{name: "every month on the first day, but started on january 8th, should run February 1st", job: &Job{interval: 1, unit: months, dayOfTheMonth: 1, lastRun: januaryFirst2020At(0, 0, 0).AddDate(0, 0, 7)}, wantTimeUntilNextRun: 24 * day},
		{name: "every 2 months at day 1, starting at day 1, should run in 2 months", job: &Job{interval: 2, unit: months, dayOfTheMonth: 1, lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: 31*day + 29*day},                          // 2020 january and february
		{name: "every 2 months at day 2, starting at day 1, should run in 2 months + 1 day", job: &Job{interval: 2, unit: months, dayOfTheMonth: 2, lastRun: januaryFirst2020At(0, 0, 0)}, wantTimeUntilNextRun: 31*day + 29*day + 1*day},          // 2020 january and february
		{name: "every 2 months at day 1, starting at day 2, should run in 2 months - 1 day", job: &Job{interval: 2, unit: months, dayOfTheMonth: 1, lastRun: januaryFirst2020At(0, 0, 0).AddDate(0, 0, 1)}, wantTimeUntilNextRun: 30*day + 29*day}, // 2020 january and february
		{name: "every 13 months at day 1, starting at day 2 run in 13 months - 1 day", job: &Job{interval: 13, unit: months, dayOfTheMonth: 1, lastRun: januaryFirst2020At(0, 0, 0).AddDate(0, 0, 1)}, wantTimeUntilNextRun: januaryFirst2020At(0, 0, 0).AddDate(0, 13, -1).Sub(januaryFirst2020At(0, 0, 0))},
		//// WEEKDAYS
		{name: "every weekday starting on one day before it should run this weekday", job: &Job{interval: 1, unit: weeks, scheduledWeekday: _tuesdayWeekday(), lastRun: mondayAt(0, 0, 0)}, wantTimeUntilNextRun: 1 * day},
		{name: "every weekday starting on same weekday should run on same immediately", job: &Job{interval: 1, unit: weeks, scheduledWeekday: _tuesdayWeekday(), lastRun: mondayAt(0, 0, 0).AddDate(0, 0, 1)}, wantTimeUntilNextRun: 0},
		{name: "every 2 weekdays counting this week's weekday should run next weekday", job: &Job{interval: 2, unit: weeks, scheduledWeekday: _tuesdayWeekday(), lastRun: mondayAt(0, 0, 0)}, wantTimeUntilNextRun: 8 * day},
		{name: "every weekday starting on one day after should count days remaining", job: &Job{interval: 1, unit: weeks, scheduledWeekday: _tuesdayWeekday(), lastRun: mondayAt(0, 0, 0).AddDate(0, 0, 2)}, wantTimeUntilNextRun: 6 * day},
		{name: "every weekday starting before jobs .At() time should run at same day at time", job: &Job{interval: 1, unit: weeks, atTime: _getHours(9) + _getMinutes(30), scheduledWeekday: _tuesdayWeekday(), lastRun: mondayAt(0, 0, 0).AddDate(0, 0, 1)}, wantTimeUntilNextRun: _getHours(9) + _getMinutes(30)},
		{name: "every weekday starting at same day at time that already passed should run at next week at time", job: &Job{interval: 1, unit: weeks, atTime: _getHours(9) + _getMinutes(30), scheduledWeekday: _tuesdayWeekday(), lastRun: mondayAt(10, 30, 0).AddDate(0, 0, 1)}, wantTimeUntilNextRun: 6*day + _getHours(23) + _getMinutes(0)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewScheduler(time.UTC)
			got := s.durationToNextRun(tc.job.LastRun(), tc.job)
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
	var testCases = []struct {
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

		j, err := s.Every(1).Second().Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)
		j.LimitRunsTo(1)

		s.StartAsync()
		time.Sleep(2 * time.Second)

		var counter int
		now := time.Now()
		for time.Now().Before(now.Add(2 * time.Second)) {
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

		j, err := s.Every(1).Second().Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)
		j.LimitRunsTo(1)

		s.StartAsync()
		time.Sleep(2 * time.Second)

		var counter int
		select {
		case <-time.After(2 * time.Second):
			assert.Equal(t, 0, s.Len())
		case <-semaphore:
			counter++
			require.LessOrEqual(t, counter, 1)
		}
	})
}

func TestCalculateMonths(t *testing.T) {
	ft := fakeTime{onNow: func(l *time.Location) time.Time {
		return time.Date(1970, 1, 1, 12, 0, 0, 0, l)
	}}
	s := NewScheduler(time.UTC)
	s.time = ft
	s.StartAsync()
	job, err := s.Every(1).Month(1).At("10:00").Do(func() {
		fmt.Println("hello task")
	})
	require.NoError(t, err)
	s.Stop()

	assert.Equal(t, s.time.Now(s.location).AddDate(0, 1, 0).Month(), job.nextRun.Month())
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
			var trigger int32

			j, err := s.Every(1).Second().SingletonMode().Do(func() {
				if atomic.LoadInt32(&trigger) == 1 {
					t.Fatal("Restart should not occur")
				}
				atomic.AddInt32(&trigger, 1)
				time.Sleep(3 * time.Second)
			})
			require.NoError(t, err)

			s.StartAsync()
			time.Sleep(2 * time.Second)

			if tc.removeJob {
				s.RemoveByReference(j)
				time.Sleep(3 * time.Second)
			}
			s.Stop()
		})
	}

}

func TestScheduler_LimitRunsTo(t *testing.T) {
	t.Run("job added before starting scheduler", func(t *testing.T) {
		semaphore := make(chan bool)

		s := NewScheduler(time.UTC)

		_, err := s.Every(1).Second().LimitRunsTo(1).Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)

		s.StartAsync()
		time.Sleep(2 * time.Second)

		var counter int
		select {
		case <-time.After(2 * time.Second):
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

		_, err := s.Every(1).Second().LimitRunsTo(1).Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)

		time.Sleep(2 * time.Second)

		var counter int
		select {
		case <-time.After(2 * time.Second):
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

		j, err := s.Every(1).Second().Do(func() {
			semaphore <- true
		})
		require.NoError(t, err)
		j.LimitRunsTo(1)

		time.Sleep(2 * time.Second)

		var counter int
		select {
		case <-time.After(2 * time.Second):
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
		// 0s - jobs 1 & 3 run, job 2 hits the limit and is skipped
		// 1s - job 1 hits the limit and is skipped
		// 2s - job 1 & 2 run
		// 3s - job 1 hits the limit and is skipped
		{"reschedule mode", 2, RescheduleMode, 4, false,
			func() {
				semaphore <- true
				time.Sleep(2 * time.Second)
			},
		},

		// Expecting a total of 8 job runs. The exact order of jobs may vary, for example:
		// 0s - jobs 2 & 3 run, job 1 hits the limit and waits
		// 1s - job 1 runs twice, the blocked run and the regularly scheduled run
		// 2s - jobs 1 & 3 run
		// 3s - jobs 2 & 3 run, job 1 hits the limit and waits
		{"wait mode", 2, WaitMode, 8, false,
			func() {
				semaphore <- true
				time.Sleep(1 * time.Second)
			},
		},

		// Same as above - this confirms the same behavior when jobs are removed rather than the scheduler being stopped
		{"wait mode - with job removal", 2, WaitMode, 8, true,
			func() {
				semaphore <- true
				time.Sleep(1 * time.Second)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {

			s := NewScheduler(time.UTC)
			s.SetMaxConcurrentJobs(tc.maxConcurrentJobs, tc.mode)

			j1, err := s.Every(1).Second().Do(tc.f)
			require.NoError(t, err)

			j2, err := s.Every(2).Second().Do(tc.f)
			require.NoError(t, err)

			j3, err := s.Every(3).Second().Do(tc.f)
			require.NoError(t, err)

			s.StartAsync()

			var counter int

			now := time.Now()
			for time.Now().Before(now.Add(4 * time.Second)) {
				select {
				case <-semaphore:
					counter++
				default:
				}
			}

			if tc.removeJobs {
				s.RemoveByReference(j1)
				s.RemoveByReference(j2)
				s.RemoveByReference(j3)
				defer s.Stop()
			} else {
				s.Stop()
			}

			// make sure no more jobs are run as the executor
			// or job should be properly stopped

			now = time.Now()
			for time.Now().Before(now.Add(1 * time.Second)) {
				select {
				case <-semaphore:
					counter++
				default:
				}
			}

			assert.Equal(t, tc.expectedRuns, counter)
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
	s := NewScheduler(time.UTC)

	j1, err := s.Every("1s").Do(func() { log.Println("one") })
	require.NoError(t, err)
	assert.Equal(t, j1, s.getCurrentJob())

	j2, err := s.Every("1s").Do(func() { log.Println("two") })
	require.NoError(t, err)
	assert.Equal(t, j2, s.getCurrentJob())

	s.Job(j1)
	assert.Equal(t, j1, s.getCurrentJob())

	s.Job(j2)
	assert.Equal(t, j2, s.getCurrentJob())
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
		s.Stop()

		counterMutex.RLock()
		defer counterMutex.RUnlock()
		assert.Equal(t, 3, counter)
	})

	t.Run("update called with job call", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		_, err := s.Every("1s").Do(func() {})
		require.NoError(t, err)

		_, err = s.Update()
		assert.EqualError(t, err, ErrUpdateCalledWithoutJob.Error())
	})
}
