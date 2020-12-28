package gocron

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
)

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

func (f fakeTime) NewTicker(duration time.Duration) *time.Ticker {
	panic("implement me")
}

func task() {
	fmt.Println("I am a running job.")
}

func taskWithParams(a int, b string) {
	fmt.Println(a, b)
}

func TestImmediateExecution(t *testing.T) {
	sched := NewScheduler(time.UTC)
	semaphore := make(chan bool)
	sched.Every(1).Second().Do(func() {
		semaphore <- true
	})
	sched.StartAsync()
	select {
	case <-time.After(1 * time.Second):
		t.Fatal("job did not run immediately")
	case <-semaphore:
		// test passed
	}

}

func TestExecutionSeconds(t *testing.T) {
	sched := NewScheduler(time.UTC)
	jobDone := make(chan bool)

	var (
		executions         []int64
		interval           uint64 = 2
		expectedExecutions        = 4
		mu                 sync.RWMutex
	)

	runTime := time.Duration(6 * time.Second)
	startTime := time.Now()

	sched.Every(interval).Seconds().Do(func() {
		mu.Lock()
		defer mu.Unlock()
		executions = append(executions, time.Now().UTC().Unix())
		if time.Now().After(startTime.Add(runTime)) {
			jobDone <- true
		}
	})

	stop := sched.StartAsync()
	<-jobDone // Wait job done
	close(stop)

	mu.RLock()
	defer mu.RUnlock()
	assert.Equal(t, expectedExecutions, len(executions), "did not run expected number of times")

	for i := 1; i < expectedExecutions; i++ {
		durationBetweenExecutions := executions[i] - executions[i-1]
		assert.Equal(t, int64(interval), durationBetweenExecutions, "duration between tasks does not correspond to expectations")
	}
}

func TestScheduled(t *testing.T) {
	n := NewScheduler(time.UTC)
	n.Every(1).Second().Do(task)
	if !n.Scheduled(task) {
		t.Fatal("Task was scheduled but function couldn't find it")
	}
}

func TestScheduledWithTag(t *testing.T) {
	sched := NewScheduler(time.UTC)
	customtag := []string{"mycustomtag"}
	sched.Every(1).Hour().SetTag(customtag).Do(task)
	if !sched.Scheduled(task) {
		t.Fatal("Task was scheduled but function couldn't find it")
	}
}

func TestAtFuture(t *testing.T) {
	t.Run("calls to .At() should parse time correctly", func(t *testing.T) {

		s := NewScheduler(time.UTC)
		now := time.Now().UTC()

		// Schedule to run in next minute
		nextMinuteTime := now.Add(1 * time.Minute)
		startAt := fmt.Sprintf("%02d:%02d:%02d", nextMinuteTime.Hour(), nextMinuteTime.Minute(), nextMinuteTime.Second())
		var hasRan bool
		dayJob, _ := s.Every(1).Day().At(startAt).Do(func() {
			hasRan = true
		})
		s.start()

		// Check first run
		expectedStartTime := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Add(time.Minute).Minute(), now.Second(), 0, time.UTC)
		nextRun := dayJob.ScheduledTime()
		assert.Equal(t, expectedStartTime, nextRun)

		// Check next run's scheduled time
		nextRun = dayJob.ScheduledTime()
		assert.Equal(t, expectedStartTime, nextRun)
		assert.False(t, hasRan, "Day job was not expected to run as it was in the future")

	})

	t.Run("error due to bad time format", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		badTime := "0:0"
		_, err := s.Every(1).Day().At(badTime).Do(func() {})
		assert.Error(t, err, "bad time format should not include jobs to the scheduler")
		assert.Zero(t, len(s.jobs))
	})
}

func schedulerForNextOrPreviousWeekdayEveryNTimes(weekday time.Weekday, next bool, n uint64, s *Scheduler) *Scheduler {
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
	weekJob, _ := s.Do(task)
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

func TestRemove(t *testing.T) {
	scheduler := NewScheduler(time.UTC)
	scheduler.Every(1).Minute().Do(task)
	scheduler.Every(1).Minute().Do(taskWithParams, 1, "hello")
	scheduler.Every(1).Minute().Do(task)

	assert.Equal(t, 3, scheduler.Len(), "Incorrect number of jobs")

	scheduler.Remove(task)
	assert.Equal(t, 1, scheduler.Len(), "Incorrect number of jobs after removing 2 job")

	scheduler.Remove(task)
	assert.Equal(t, 1, scheduler.Len(), "Incorrect number of jobs after removing non-existent job")
}

func TestRemoveByRef(t *testing.T) {
	scheduler := NewScheduler(time.UTC)
	job1, _ := scheduler.Every(1).Minute().Do(task)
	job2, _ := scheduler.Every(1).Minute().Do(taskWithParams, 1, "hello")

	assert.Equal(t, 2, scheduler.Len(), "Incorrect number of jobs")

	scheduler.RemoveByReference(job1)
	assert.ElementsMatch(t, []*Job{job2}, scheduler.Jobs())
}

func TestRemoveByTag(t *testing.T) {
	scheduler := NewScheduler(time.UTC)

	// Creating 2 Jobs with Unique tags
	customtag1 := []string{"tag one"}
	customtag2 := []string{"tag two"}
	scheduler.Every(1).Minute().SetTag(customtag1).Do(taskWithParams, 1, "hello") // index 0
	scheduler.Every(1).Minute().SetTag(customtag2).Do(taskWithParams, 2, "world") // index 1

	assert.Equal(t, 2, scheduler.Len(), "Incorrect number of jobs")

	// check Jobs()[0] tags is equal with tag "tag one" (customtag1)
	assert.Equal(t, scheduler.Jobs()[0].Tags(), customtag1, "Job With Tag 'tag one' is removed from index 0")

	scheduler.RemoveJobByTag("tag one")
	assert.Equal(t, 1, scheduler.Len(), "Incorrect number of jobs after removing 1 job")

	// check Jobs()[0] tags is equal with tag "tag two" (customtag2) after removing "tag one"
	assert.Equal(t, scheduler.Jobs()[0].Tags(), customtag2, "Job With Tag 'tag two' is removed from index 0")

	// Removing Non Existent Job with "tag one" because already removed above (will not removing any jobs because tag not match)
	scheduler.RemoveJobByTag("tag one")
	assert.Equal(t, 1, scheduler.Len(), "Incorrect number of jobs after removing non-existent job")
}

func TestJobs(t *testing.T) {
	s := NewScheduler(time.UTC)
	s.Every(1).Minute().Do(task)
	s.Every(2).Minutes().Do(task)
	s.Every(3).Minutes().Do(task)
	s.Every(4).Minutes().Do(task)
	js := s.Jobs()
	assert.Len(t, js, 4)
}

func TestLen(t *testing.T) {
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
	s.Every(1).Minute().Do(task)
	s.Every(2).Minute().Do(task)

	jb := s.Jobs()
	var jobsBefore []*Job
	for _, p := range jb {
		jobsBefore = append(jobsBefore, p)
	}

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
	s.Every(1).Minute().Do(task)
	s.Every(2).Minute().Do(task)

	assert.Equal(t, 2, s.Len())

	s.Clear()

	assert.Equal(t, 0, s.Len())
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
	t.Run("noop on stopped scheduler", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		s.Stop()
		assert.False(t, s.IsRunning())
	})
}

func TestScheduler_StartAt(t *testing.T) {
	scheduler := NewScheduler(time.Local)
	now := time.Now()

	// With StartAt
	job, _ := scheduler.Every(3).Seconds().StartAt(now.Add(time.Second * 5)).Do(func() {})
	assert.False(t, job.getStartsImmediately())
	scheduler.start()
	assert.Equal(t, now.Add(time.Second*5), job.NextRun())
	scheduler.stop()

	// Without StartAt
	job, _ = scheduler.Every(3).Seconds().Do(func() {})
	assert.True(t, job.getStartsImmediately())
}

func TestScheduler_CalculateNextRun(t *testing.T) {
	day := time.Hour * 24
	januaryFirst2020At := func(hour, minute, second int) time.Time {
		return time.Date(2020, time.January, 1, hour, minute, second, 0, time.UTC)
	}
	mondayAt := func(hour, minute, second int) time.Time {
		return time.Date(2020, time.January, 6, hour, minute, second, 0, time.UTC)
	}

	var tests = []struct {
		name                 string
		job                  Job
		wantTimeUntilNextRun time.Duration
	}{
		// SECONDS
		{
			name: "every second test",
			job: Job{
				interval: 1,
				unit:     seconds,
				lastRun:  januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: _getSeconds(1),
		},
		{
			name: "every 62 seconds test",
			job: Job{
				interval: 62,
				unit:     seconds,
				lastRun:  januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: _getSeconds(62),
		},
		// MINUTES
		{
			name: "every minute test",
			job: Job{
				interval: 1,
				unit:     minutes,
				lastRun:  januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: _getMinutes(1),
		},
		{
			name: "every 62 minutes test",
			job: Job{
				interval: 62,
				unit:     minutes,
				lastRun:  januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: _getMinutes(62),
		},
		// HOURS
		{
			name: "every hour test",
			job: Job{
				interval: 1,
				unit:     hours,
				lastRun:  januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: _getHours(1),
		},
		{
			name: "every 25 hours test",
			job: Job{
				interval: 25,
				unit:     hours,
				lastRun:  januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: _getHours(25),
		},
		// DAYS
		{
			name: "every day at midnight",
			job: Job{
				interval: 1,
				unit:     days,
				lastRun:  januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: 1 * day,
		},
		{
			name: "every day at 09:30AM with scheduler starting before 09:30AM should run at same day at time",
			job: Job{
				interval: 1,
				unit:     days,
				atTime:   _getHours(9) + _getMinutes(30),
				lastRun:  januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: _getHours(9) + _getMinutes(30),
		},
		{
			name: "every day at 09:30AM which just ran should run tomorrow at 09:30AM",
			job: Job{
				interval: 1,
				unit:     days,
				atTime:   _getHours(9) + _getMinutes(30),
				lastRun:  januaryFirst2020At(9, 30, 0),
			},
			wantTimeUntilNextRun: 1 * day,
		},
		{
			name: "every 31 days at midnight should run 31 days later",
			job: Job{
				interval: 31,
				unit:     days,
				lastRun:  januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: 31 * day,
		},
		{
			name: "daily job just ran at 8:30AM and should be scheduled for next day's 8:30AM",
			job: Job{
				interval: 1,
				unit:     days,
				atTime:   8*time.Hour + 30*time.Minute,
				lastRun:  januaryFirst2020At(8, 30, 0),
			},
			wantTimeUntilNextRun: 24 * time.Hour,
		},
		{
			name: "daily job just ran at 5:30AM and should be scheduled for today at 8:30AM",
			job: Job{
				interval: 1,
				unit:     days,
				atTime:   8*time.Hour + 30*time.Minute,
				lastRun:  januaryFirst2020At(5, 30, 0),
			},
			wantTimeUntilNextRun: 3 * time.Hour,
		},
		{
			name: "job runs every 2 days, just ran at 5:30AM and should be scheduled for 2 days at 8:30AM",
			job: Job{
				interval: 2,
				unit:     days,
				atTime:   8*time.Hour + 30*time.Minute,
				lastRun:  januaryFirst2020At(5, 30, 0),
			},
			wantTimeUntilNextRun: (2 * day) + 3*time.Hour,
		},
		{
			name: "job runs every 2 days, just ran at 8:30AM and should be scheduled for 2 days at 8:30AM",
			job: Job{
				interval: 2,
				unit:     days,
				atTime:   8*time.Hour + 30*time.Minute,
				lastRun:  januaryFirst2020At(8, 30, 0),
			},
			wantTimeUntilNextRun: 2 * day,
		},
		//// WEEKS
		{
			name: "every week should run in 7 days",
			job: Job{
				interval: 1,
				unit:     weeks,
				lastRun:  januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: 7 * day,
		},
		{
			name: "every week with .At time rule should run respect .At time rule",
			job: Job{
				interval: 1,
				atTime:   _getHours(9) + _getMinutes(30),
				unit:     weeks,
				lastRun:  januaryFirst2020At(9, 30, 0),
			},
			wantTimeUntilNextRun: 7 * day,
		},
		{
			name: "every two weeks at 09:30AM should run in 14 days at 09:30AM",
			job: Job{
				interval: 2,
				unit:     weeks,
				lastRun:  januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: 14 * day,
		},
		{
			name: "every 31 weeks ran at jan 1st at midnight should run at August 5, 2020",
			job: Job{
				interval: 31,
				unit:     weeks,
				lastRun:  januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: 31 * 7 * day,
		},
		// MONTHS
		{
			name: "every month in a 31 days month should be scheduled for 31 days ahead",
			job: Job{
				interval: 1,
				unit:     months,
				lastRun:  januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: 31 * day,
		},
		{
			name: "every month in a 30 days month should be scheduled for 30 days ahead",
			job: Job{
				interval: 1,
				unit:     months,
				lastRun:  time.Date(2020, time.April, 1, 0, 0, 0, 0, time.UTC),
			},
			wantTimeUntilNextRun: 30 * day,
		},
		{
			name: "every month at february on leap year should count 29 days",
			job: Job{
				interval: 1,
				unit:     months,
				lastRun:  time.Date(2020, time.February, 1, 0, 0, 0, 0, time.UTC),
			},
			wantTimeUntilNextRun: 29 * day,
		},
		{
			name: "every month at february on non leap year should count 28 days",
			job: Job{
				interval: 1,
				unit:     months,
				lastRun:  time.Date(2019, time.February, 1, 0, 0, 0, 0, time.UTC),
			},
			wantTimeUntilNextRun: 28 * day,
		},
		{
			name: "every month at first day at time should run next month + at time",
			job: Job{
				interval: 1,
				unit:     months,
				atTime:   _getHours(9) + _getMinutes(30),
				lastRun:  januaryFirst2020At(9, 30, 0),
			},
			wantTimeUntilNextRun: 31*day + _getHours(9) + _getMinutes(30),
		},
		{
			name: "every month at day should consider at days",
			job: Job{
				interval:      1,
				unit:          months,
				dayOfTheMonth: 2,
				lastRun:       januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: 1 * day,
		},
		{
			name: "every month at day should consider at hours",
			job: Job{
				interval: 1,
				unit:     months,
				atTime:   _getHours(9) + _getMinutes(30),
				lastRun:  januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: 31*day + _getHours(9) + _getMinutes(30),
		},
		{
			name: "every month on the first day, but started on january 8th, should run February 1st",
			job: Job{
				interval:      1,
				unit:          months,
				dayOfTheMonth: 1,
				lastRun:       januaryFirst2020At(0, 0, 0).AddDate(0, 0, 7),
			},
			wantTimeUntilNextRun: 24 * day,
		},
		{
			name: "every 2 months at day 1, starting at day 1, should run in 2 months",
			job: Job{
				interval:      2,
				unit:          months,
				dayOfTheMonth: 1,
				lastRun:       januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: 31*day + 29*day, // 2020 january and february
		},
		{
			name: "every 2 months at day 2, starting at day 1, should run in 2 months + 1 day",
			job: Job{
				interval:      2,
				unit:          months,
				dayOfTheMonth: 2,
				lastRun:       januaryFirst2020At(0, 0, 0),
			},
			wantTimeUntilNextRun: 31*day + 29*day + 1*day, // 2020 january and february
		},
		{
			name: "every 2 months at day 1, starting at day 2, should run in 2 months - 1 day",
			job: Job{
				interval:      2,
				unit:          months,
				dayOfTheMonth: 1,
				lastRun:       januaryFirst2020At(0, 0, 0).AddDate(0, 0, 1),
			},
			wantTimeUntilNextRun: 30*day + 29*day, // 2020 january and february
		},
		{
			name: "every 13 months at day 1, starting at day 2 run in 13 months - 1 day",
			job: Job{
				interval:      13,
				unit:          months,
				dayOfTheMonth: 1,
				lastRun:       januaryFirst2020At(0, 0, 0).AddDate(0, 0, 1),
			},
			wantTimeUntilNextRun: januaryFirst2020At(0, 0, 0).AddDate(0, 13, -1).Sub(januaryFirst2020At(0, 0, 0)),
		},
		//// WEEKDAYS
		{
			name: "every weekday starting on one day before it should run this weekday",
			job: Job{
				interval:         1,
				unit:             weeks,
				scheduledWeekday: _tuesdayWeekday(),
				lastRun:          mondayAt(0, 0, 0),
			},
			wantTimeUntilNextRun: 1 * day,
		},
		{
			name: "every weekday starting on same weekday should run on same immediately",
			job: Job{
				interval:         1,
				unit:             weeks,
				scheduledWeekday: _tuesdayWeekday(),
				lastRun:          mondayAt(0, 0, 0).AddDate(0, 0, 1),
			},
			wantTimeUntilNextRun: 0,
		},
		{
			name: "every 2 weekdays counting this week's weekday should run next weekday",
			job: Job{
				interval:         2,
				unit:             weeks,
				scheduledWeekday: _tuesdayWeekday(),
				lastRun:          mondayAt(0, 0, 0),
			},
			wantTimeUntilNextRun: 8 * day,
		},
		{
			name: "every weekday starting on one day after should count days remaning",
			job: Job{
				interval:         1,
				unit:             weeks,
				scheduledWeekday: _tuesdayWeekday(),
				lastRun:          mondayAt(0, 0, 0).AddDate(0, 0, 2),
			},
			wantTimeUntilNextRun: 6 * day,
		},
		{
			name: "every weekday starting before jobs .At() time should run at same day at time",
			job: Job{
				interval:         1,
				unit:             weeks,
				atTime:           _getHours(9) + _getMinutes(30),
				scheduledWeekday: _tuesdayWeekday(),
				lastRun:          mondayAt(0, 0, 0).AddDate(0, 0, 1),
			},
			wantTimeUntilNextRun: _getHours(9) + _getMinutes(30),
		},
		{
			name: "every weekday starting at same day at time that already passed should run at next week at time",
			job: Job{
				interval:         1,
				unit:             weeks,
				atTime:           _getHours(9) + _getMinutes(30),
				scheduledWeekday: _tuesdayWeekday(),
				lastRun:          mondayAt(10, 30, 0).AddDate(0, 0, 1),
			},
			wantTimeUntilNextRun: 6*day + _getHours(23) + _getMinutes(0),
		},
	}

	for i := range tests {
		t.Run(tests[i].name, func(t *testing.T) {
			sched := NewScheduler(time.UTC)
			got := sched.durationToNextRun(tests[i].job.LastRun(), &tests[i].job)
			assert.Equalf(t, tests[i].wantTimeUntilNextRun, got, fmt.Sprintf("expected %s / got %s", tests[i].wantTimeUntilNextRun.String(), got.String()))
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
	t.Run("adding a new job before scheduler starts does not schedule job", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		s.setRunning(false)
		job, err := s.Every(1).Second().Do(func() {})
		assert.Equal(t, nil, err)
		assert.True(t, job.nextRun.IsZero())
	})

	t.Run("adding a new job when scheduler is running schedules job", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		s.setRunning(true)
		job, err := s.Every(1).Second().Do(func() {})
		assert.Equal(t, nil, err)
		assert.False(t, job.NextRun().IsZero())
	})
}

func TestRunJobsWithLimit(t *testing.T) {
	f := func(in *int, mu *sync.RWMutex) {
		mu.Lock()
		defer mu.Unlock()
		*in = *in + 1
	}

	s := NewScheduler(time.UTC)

	var j1Counter, j2Counter int
	var j1Mutex, j2Mutex sync.RWMutex
	j1, err := s.Every(1).Second().Do(f, &j1Counter, &j1Mutex)
	require.NoError(t, err)

	j1.LimitRunsTo(1)

	j2, err := s.Every(1).Second().Do(f, &j2Counter, &j2Mutex)
	require.NoError(t, err)

	j2.LimitRunsTo(1)

	s.StartAsync()
	time.Sleep(3 * time.Second)

	j1Mutex.RLock()
	j1Mutex.RUnlock()
	assert.Exactly(t, 1, j1Counter)

	j2Mutex.RLock()
	j2Mutex.RUnlock()
	assert.Exactly(t, 1, j2Counter)
}

func TestDo(t *testing.T) {
	var tests = []struct {
		name     string
		evalFunc func(*Scheduler)
	}{
		{
			name: "error due to the arg passed to Do() not being a function",
			evalFunc: func(s *Scheduler) {
				s.Every(1).Second().Do(1)
				assert.Zero(t, len(s.jobs), "The job should be deleted if the arg passed to Do() is not a function")
			},
		},
		{
			name: "positive case",
			evalFunc: func(s *Scheduler) {
				s.Every(1).Day().Do(func() {})
				assert.Equal(t, 1, len(s.jobs))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewScheduler(time.Local)
			tt.evalFunc(s)
		})
	}
}

func TestRemoveAfterExec(t *testing.T) {
	s := NewScheduler(time.UTC)

	job, err := s.Every(1).Second().Do(task, s)
	require.NoError(t, err)

	job.LimitRunsTo(1)
	job.RemoveAfterLastRun()
	s.StartAsync()

	time.Sleep(2 * time.Second)

	assert.Zero(t, len(s.Jobs()))
}
