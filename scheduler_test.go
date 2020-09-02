package gocron

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func task() {
	fmt.Println("I am a running job.")
}

func taskWithParams(a int, b string) {
	fmt.Println(a, b)
}

func TestExecutionSecond(t *testing.T) {
	sched := NewScheduler(time.UTC)
	success := false
	sched.Every(1).Second().Do(func(mutableValue *bool) {
		*mutableValue = !*mutableValue
	}, &success)
	sched.RunAllWithDelay(1)
	assert.Equal(t, true, success, "Task did not get called")
}

func TestExecutionSeconds(t *testing.T) {
	sched := NewScheduler(time.UTC)
	jobDone := make(chan bool)
	var executionTimes []int64
	numberOfIterations := 2

	sched.Every(2).Seconds().Do(func() {
		executionTimes = append(executionTimes, time.Now().Unix())
		if len(executionTimes) >= numberOfIterations {
			jobDone <- true
		}
	})

	stop := sched.StartAsync()
	<-jobDone // Wait job done
	close(stop)

	assert.Equal(t, numberOfIterations, len(executionTimes), "did not run expected number of times")

	for i := 1; i < numberOfIterations; i++ {
		durationBetweenExecutions := executionTimes[i] - executionTimes[i-1]
		assert.Equal(t, int64(2), durationBetweenExecutions, "Duration between tasks does not correspond to expectations")
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

func TestStartImmediately(t *testing.T) {
	sched := NewScheduler(time.UTC)
	now := time.Now().UTC()

	job, _ := sched.Every(1).Hour().StartImmediately().Do(task)
	sched.scheduleAllJobs()
	next := job.ScheduledTime()

	nextRounded := time.Date(next.Year(), next.Month(), next.Day(), next.Hour(), next.Minute(), next.Second(), 0, time.UTC)
	expected := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), 0, time.UTC)

	assert.Exactly(t, expected, nextRounded)
}

func TestAt(t *testing.T) {
	t.Run("runAt(3 seconds in the future) should run today at current time plus 3 seconds", func(t *testing.T) {
		// WARNING: non-deterministic test
		s := NewScheduler(time.UTC)
		now := time.Now().UTC()
		scheduletime := now.Add(3 * time.Second)

		// Schedule every day At
		startAt := fmt.Sprintf("%02d:%02d:%02d", scheduletime.Hour(), scheduletime.Minute(), scheduletime.Second())
		job, _ := s.Every(1).Day().At(startAt).Do(func() {

		})
		s.scheduleAllJobs()

		// Expected start time
		expectedStartTime := time.Date(scheduletime.Year(), scheduletime.Month(), scheduletime.Day(), now.Hour(), now.Minute(), now.Add(3*time.Second).Second(), 0, time.UTC)
		assert.Equal(t, expectedStartTime, job.ScheduledTime())
	})
	t.Run("runAt(3 seconds in the past) should run tomorrow at current time plus 3 seconds", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		now := time.Now().UTC()
		scheduletime := now.Add(3 * (-time.Second))

		// Schedule every day At
		startAt := fmt.Sprintf("%02d:%02d:%02d", scheduletime.Hour(), scheduletime.Minute(), scheduletime.Second())
		job, _ := s.Every(1).Day().At(startAt).Do(func() {

		})
		s.scheduleAllJobs()

		// Expected start time
		tomorrow := now.AddDate(0, 0, 1)
		expectedStartTime := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), now.Hour(), now.Minute(), now.Add(3*(-time.Second)).Second(), 0, time.UTC)
		assert.Equal(t, expectedStartTime, job.ScheduledTime())
	})
}

func TestAtFuture(t *testing.T) {
	s := NewScheduler(time.UTC)
	now := time.Now().UTC()

	// Schedule to run in next minute
	nextMinuteTime := now.Add(1 * time.Minute)
	startAt := fmt.Sprintf("%02d:%02d:%02d", nextMinuteTime.Hour(), nextMinuteTime.Minute(), nextMinuteTime.Second())
	shouldBeFalse := false
	dayJob, _ := s.Every(1).Day().At(startAt).Do(func() {
		shouldBeFalse = true
	})
	s.scheduleAllJobs()

	// Check first run
	expectedStartTime := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Add(time.Minute).Minute(), now.Second(), 0, time.UTC)
	nextRun := dayJob.ScheduledTime()
	assert.Equal(t, expectedStartTime, nextRun)

	s.RunPending()

	// Check next run's scheduled time
	nextRun = dayJob.ScheduledTime()
	assert.Equal(t, expectedStartTime, nextRun)
	assert.Equal(t, false, shouldBeFalse, "Day job was not expected to run as it was in the future")
}

func TestDay(t *testing.T) {
	now := time.Now().UTC()

	// Create new scheduler to have clean test env
	s := NewScheduler(time.UTC)

	// schedule next run 1 day
	dayJob, _ := s.Every(1).Day().Do(task)
	s.scheduleNextRun(dayJob)
	tomorrow := now.AddDate(0, 0, 1)
	expectedTime := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, time.UTC)
	assert.Equal(t, expectedTime, dayJob.nextRun)

	// schedule next run 2 days
	dayJob, _ = s.Every(2).Days().Do(task)
	s.scheduleNextRun(dayJob)
	twoDaysFromNow := now.AddDate(0, 0, 2)
	expectedTime = time.Date(twoDaysFromNow.Year(), twoDaysFromNow.Month(), twoDaysFromNow.Day(), 0, 0, 0, 0, time.UTC)
	assert.Equal(t, expectedTime, dayJob.nextRun)

	// Job running longer than next schedule 1day 2 hours
	dayJob, _ = s.Every(1).Day().Do(task)
	twoHoursFromNow := now.Add(2 * time.Hour)
	dayJob.lastRun = time.Date(twoHoursFromNow.Year(), twoHoursFromNow.Month(), twoHoursFromNow.Day(), twoHoursFromNow.Hour(), 0, 0, 0, time.UTC)
	s.scheduleNextRun(dayJob)
	expectedTime = time.Date(now.Year(), now.Month(), now.AddDate(0, 0, 1).Day(), 0, 0, 0, 0, time.UTC)
	assert.Equal(t, expectedTime, dayJob.nextRun)

	// At() 2 hours before now
	twoHoursBefore := now.Add(time.Duration(-2 * time.Hour))
	startAt := fmt.Sprintf("%02d:%02d", twoHoursBefore.Hour(), twoHoursBefore.Minute())
	dayJob, _ = s.Every(1).Day().At(startAt).Do(task)
	s.scheduleNextRun(dayJob)

	expectedTime = time.Date(twoHoursBefore.Year(), twoHoursBefore.Month(),
		twoHoursBefore.AddDate(0, 0, 1).Day(),
		twoHoursBefore.Hour(), twoHoursBefore.Minute(), 0, 0, time.UTC)

	assert.Equal(t, expectedTime, dayJob.nextRun)
}

func schedulerForNextWeekdayEveryNTimes(weekday time.Weekday, n uint64, s *Scheduler) *Scheduler {
	switch weekday {
	case time.Monday:
		s = s.Every(n).Tuesday()
	case time.Tuesday:
		s = s.Every(n).Wednesday()
	case time.Wednesday:
		s = s.Every(n).Thursday()
	case time.Thursday:
		s = s.Every(n).Friday()
	case time.Friday:
		s = s.Every(n).Saturday()
	case time.Saturday:
		s = s.Every(n).Sunday()
	case time.Sunday:
		s = s.Every(n).Monday()
	}
	return s
}

func TestWeekdayBeforeToday(t *testing.T) {
	now := time.Now().In(time.UTC)
	s := NewScheduler(time.UTC)

	// Schedule job at day before
	switch now.Weekday() {
	case time.Monday:
		s = s.Every(1).Sunday()
	case time.Tuesday:
		s = s.Every(1).Monday()
	case time.Wednesday:
		s = s.Every(1).Tuesday()
	case time.Thursday:
		s = s.Every(1).Wednesday()
	case time.Friday:
		s = s.Every(1).Thursday()
	case time.Saturday:
		s = s.Every(1).Friday()
	case time.Sunday:
		s = s.Every(1).Saturday()
	}
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
		s = schedulerForNextWeekdayEveryNTimes(now.Weekday(), 1, s)
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
	var jobsBefore []Job
	for _, p := range jb {
		jobsBefore = append(jobsBefore, *p)
	}

	s.Swap(1, 0)

	jobsAfter := s.Jobs()

	assert.Equal(t, &jobsBefore[0], jobsAfter[1])
	assert.Equal(t, &jobsBefore[1], jobsAfter[0])
}

func TestLess(t *testing.T) {
	s := NewScheduler(time.UTC)
	s.Every(1).Minute().Do(task)
	s.Every(2).Minute().Do(task)

	assert.True(t, s.Less(0, 1))
}

func TestSetLocation(t *testing.T) {
	s := NewScheduler(time.FixedZone("UTC-8", -8*60*60))

	assert.Equal(t, time.FixedZone("UTC-8", -8*60*60), s.loc)

	s.ChangeLocation(time.UTC)

	assert.Equal(t, time.UTC, s.loc)
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
		sched := NewScheduler(time.UTC)
		sched.StartAsync()
		sched.Stop()
		assert.False(t, sched.running)
	})
	t.Run("noop on stopped scheduler", func(t *testing.T) {
		sched := NewScheduler(time.UTC)
		sched.Stop()
		assert.False(t, sched.running)
	})
}

func TestScheduler_StartAt(t *testing.T) {
	scheduler := NewScheduler(time.Local)
	now := time.Now()

	// With StartAt
	job, _ := scheduler.Every(3).Seconds().StartAt(now.Add(time.Second * 5)).Do(func() {})
	scheduler.scheduleAllJobs()
	_, nextRun := scheduler.NextRun()
	t.Log("now: " + now.String())
	t.Log("next run : " + nextRun.String())
	t.Log("now plus 5 run : " + now.Add(time.Second*5).String())
	assert.Equal(t, now.Add(time.Second*5), nextRun)
	scheduler.Remove(job)

	// Without StartAt
	job, _ = scheduler.Every(3).Seconds().Do(func() {})
	scheduler.scheduleNextRun(job)
	_, nextRun = scheduler.NextRun()
	assert.Equal(t, now.Add(time.Second*3).Second(), nextRun.Second())
}

func TestScheduler_ScheduleDays(t *testing.T) {
	sched := NewScheduler(time.UTC)
	fakeTime := fakeTime{}
	sched.time = &fakeTime

	t.Run("schedule for days without .At(time)", func(t *testing.T) {
		t.Run("1 day at midnight", func(t *testing.T) {
			j, err := sched.Every(1).Day().Do(func() {})
			assert.Nil(t, err)

			// scheduler started
			startRunningTime := time.Date(2020, time.August, 1, 15, 15, 15, 0, time.UTC)
			fakeTime.onNow = func(location *time.Location) time.Time {
				return startRunningTime
			}
			sched.scheduleAllJobs()

			// expected for tomorrow at midnight
			firstRun := sched.roundToMidnight(startRunningTime.AddDate(0, 0, 1))
			assert.Equal(t, firstRun, j.nextRun)

			// then it ran
			fakeTime.onNow = func(location *time.Location) time.Time {
				return firstRun
			}
			sched.runAndReschedule(j)

			// scheduled for next night at midnight
			assert.Equal(t, firstRun.AddDate(0, 0, 1), j.nextRun)
		})

		t.Run("5 days from start at midnight", func(t *testing.T) {
			j, err := sched.Every(5).Days().Do(func() {})
			assert.Nil(t, err)

			// scheduler started
			startRunningTime := time.Date(2020, time.August, 1, 15, 15, 15, 0, time.UTC)
			fakeTime.onNow = func(location *time.Location) time.Time {
				return startRunningTime
			}
			sched.scheduleAllJobs()

			// expected five days from start at midnight
			firstRun := sched.roundToMidnight(startRunningTime).AddDate(0, 0, 5)
			assert.Equal(t, firstRun, j.nextRun)

			// then it ran
			fakeTime.onNow = func(location *time.Location) time.Time {
				return firstRun
			}
			sched.runAndReschedule(j)

			// expects 10 days from start at midnight
			assert.Equal(t, sched.roundToMidnight(startRunningTime).AddDate(0, 0, 10), j.nextRun)
		})
	})

	t.Run("schedule for days with .At(time) option", func(t *testing.T) {
		t.Run("1 day at time and starting today", func(t *testing.T) {
			j, err := sched.Every(1).Day().At("15:20:15").Do(func() {})
			assert.Nil(t, err)

			// scheduler started
			startRunningTime := time.Date(2020, time.August, 1, 15, 15, 15, 0, time.UTC)
			fakeTime.onNow = func(location *time.Location) time.Time {
				return startRunningTime
			}
			sched.scheduleAllJobs()

			// scheduled for today
			firstRun := time.Date(startRunningTime.Year(), startRunningTime.Month(), startRunningTime.Day(), 15, 20, 15, 0, time.UTC)
			assert.Equal(t, firstRun, j.nextRun)

			// then it ran
			fakeTime.onNow = func(location *time.Location) time.Time {
				return firstRun
			}
			sched.runAndReschedule(j)

			// scheduled 1 day from start
			expectedNextRunDate := firstRun.AddDate(0, 0, 1)
			assert.Equal(t, expectedNextRunDate, j.nextRun)
		})

		t.Run("1 day at time and starting tomorrow", func(t *testing.T) {
			j, err := sched.Every(1).Day().At("15:15:00").Do(func() {})
			assert.Nil(t, err)

			// scheduler started
			startRunningTime := time.Date(2020, time.August, 1, 15, 15, 15, 0, time.UTC)
			fakeTime.onNow = func(location *time.Location) time.Time {
				return startRunningTime
			}
			sched.scheduleAllJobs()

			// scheduled for tomorrow
			nextRun := startRunningTime.AddDate(0, 0, 1)
			expectedFirstRun := time.Date(nextRun.Year(), nextRun.Month(), nextRun.Day(), 15, 15, 0, 0, time.UTC)
			assert.Equal(t, expectedFirstRun, j.nextRun)

			// then it ran
			fakeTime.onNow = func(location *time.Location) time.Time {
				return expectedFirstRun
			}
			sched.runAndReschedule(j)

			// scheduled for 2 days from expectedFirstRun
			expectedNextRunDate := expectedFirstRun.AddDate(0, 0, 1)
			assert.Equal(t, expectedNextRunDate, j.nextRun)
		})

		t.Run("5 days at time", func(t *testing.T) {
			j, err := sched.Every(5).Days().At("15:15:15").Do(func() {})
			assert.Nil(t, err)

			// scheduler started
			startRunningTime := time.Date(2020, time.August, 1, 15, 15, 15, 0, time.UTC)
			fakeTime.onNow = func(location *time.Location) time.Time {
				return startRunningTime
			}
			sched.scheduleAllJobs()

			// scheduled for five days from start
			nextRun := startRunningTime.AddDate(0, 0, 5)
			assert.Equal(t, nextRun, j.nextRun)

			// then it ran
			fakeTime.onNow = func(location *time.Location) time.Time {
				return nextRun
			}
			sched.runAndReschedule(j)

			// scheduled for 10 from start
			expectedNextRunDate := startRunningTime.AddDate(0, 0, 10)
			assert.Equal(t, expectedNextRunDate, j.nextRun)
		})
	})
}

func TestScheduler_ScheduleWeeks(t *testing.T) {
	sched := NewScheduler(time.UTC)
	fakeTime := fakeTime{}
	sched.time = &fakeTime

	t.Run("schedule for weeks at midnight", func(t *testing.T) {
		t.Run("1 week at midnight", func(t *testing.T) {
			j, err := sched.Every(1).Week().Do(func() {})
			assert.Nil(t, err)

			// scheduler started
			startRunningTime := time.Date(2020, time.August, 1, 15, 15, 15, 0, time.UTC)
			fakeTime.onNow = func(location *time.Location) time.Time {
				return startRunningTime
			}
			sched.scheduleAllJobs()

			// expected for 7 days from start
			firstRun := sched.roundToMidnight(startRunningTime).AddDate(0, 0, 7)
			assert.Equal(t, firstRun, j.nextRun)

			// then it ran
			fakeTime.onNow = func(location *time.Location) time.Time {
				return firstRun
			}
			sched.runAndReschedule(j)

			// expected for 14 days from start
			assert.Equal(t, sched.roundToMidnight(startRunningTime).AddDate(0, 0, 14), j.nextRun)
		})
		t.Run("5 weeks at midnight", func(t *testing.T) {
			j, err := sched.Every(5).Weeks().Do(func() {})
			assert.Nil(t, err)

			// scheduler started
			startRunningTime := time.Date(2020, time.August, 1, 15, 15, 15, 0, time.UTC)
			fakeTime.onNow = func(location *time.Location) time.Time {
				return startRunningTime
			}
			sched.scheduleAllJobs()

			// expected for 5 weeks from start
			firstRun := sched.roundToMidnight(startRunningTime).AddDate(0, 0, 7*5)
			assert.Equal(t, firstRun, j.nextRun)

			// then it ran
			fakeTime.onNow = func(location *time.Location) time.Time {
				return firstRun
			}
			sched.runAndReschedule(j)

			// expected for 10 weeks from start
			assert.Equal(t, sched.roundToMidnight(startRunningTime).AddDate(0, 0, 7*10), j.nextRun)
		})
	})
}

func TestScheduler_ScheduleMonths(t *testing.T) {
	sched := NewScheduler(time.UTC)
	fakeTime := fakeTime{}
	sched.time = &fakeTime

	t.Run("schedule for months", func(t *testing.T) {
		t.Run("next month at midnight", func(t *testing.T) {
			j, err := sched.Every(1).Month(1).Do(func() {})
			assert.Nil(t, err)

			// scheduler started
			startRunningTime := time.Date(2020, time.August, 1, 15, 15, 15, 0, time.UTC)
			fakeTime.onNow = func(location *time.Location) time.Time {
				return startRunningTime
			}
			sched.scheduleAllJobs()

			// expected next month at midnight
			nextMonth := sched.roundToMidnight(startRunningTime).AddDate(0, 1, 0)
			assert.Equal(t, nextMonth, j.nextRun)

			// then it ran
			fakeTime.onNow = func(location *time.Location) time.Time {
				return nextMonth
			}
			sched.runAndReschedule(j)

			// expected 2 months from start at midnight
			assert.Equal(t, j.nextRun, sched.roundToMidnight(startRunningTime).AddDate(0, 2, 0))
		})
		t.Run("5 months from now at midnight", func(t *testing.T) {
			j, err := sched.Every(5).Months(1).Do(func() {})
			assert.Nil(t, err)

			// scheduler started
			startRunnningTime := time.Date(2020, time.August, 1, 15, 15, 15, 0, time.UTC)
			fakeTime.onNow = func(location *time.Location) time.Time {
				return startRunnningTime
			}
			sched.scheduleAllJobs()

			// expected to run in 5 months at midnight
			fiveMonthsFromStart := sched.roundToMidnight(startRunnningTime.AddDate(0, 5, 0))
			assert.Equal(t, j.nextRun, fiveMonthsFromStart)

			// then it ran
			fakeTime.onNow = func(location *time.Location) time.Time {
				return fiveMonthsFromStart
			}
			sched.runAndReschedule(j)

			// expected 10 months from start
			tenMonthsFromStart := sched.roundToMidnight(startRunnningTime.AddDate(0, 10, 0))
			assert.Equal(t, j.nextRun, tenMonthsFromStart)
		})

		t.Run("5 months from now at time", func(t *testing.T) {
			j, err := sched.Every(5).Months(1).At("15:15:15").Do(func() {})
			assert.Nil(t, err)

			// scheduler started
			startRunnningTime := time.Date(2020, time.August, 1, 15, 15, 15, 0, time.UTC)
			fakeTime.onNow = func(location *time.Location) time.Time {
				return startRunnningTime
			}
			sched.scheduleAllJobs()

			// expected to run in 5 months at midnight
			fiveMonthsFromStart := startRunnningTime.AddDate(0, 5, 0)
			assert.Equal(t, j.nextRun, fiveMonthsFromStart)

			// then it ran
			fakeTime.onNow = func(location *time.Location) time.Time {
				return fiveMonthsFromStart
			}
			sched.runAndReschedule(j)

			// expected 10 months from start
			tenMonthsFromStart := startRunnningTime.AddDate(0, 10, 0)
			assert.Equal(t, j.nextRun, tenMonthsFromStart)
		})
	})
}

func TestScheduler_ScheduleWeekdays(t *testing.T) {
	sched := NewScheduler(time.UTC)
	fakeTime := fakeTime{}
	sched.time = &fakeTime

	t.Run("every weekday starting with .At() that already passed should run next week", func(t *testing.T) {
		j, err := sched.Every(1).Saturday().At("15:15:00").Do(func() {})
		assert.Nil(t, err)

		// scheduler started
		startingSaturday := time.Date(2020, time.Month(8), 29, 15, 15, 15, 0, time.UTC)
		fakeTime.onNow = func(location *time.Location) time.Time {
			return startingSaturday
		}
		sched.scheduleAllJobs()

		// expected to run next weekday
		nextSaturday := startingSaturday.AddDate(0, 0, 7)
		expectedSaturdayAtTime := time.Date(nextSaturday.Year(), nextSaturday.Month(), nextSaturday.Day(),
			15, 15, 0, 0, time.UTC)
		assert.Equal(t, expectedSaturdayAtTime, j.nextRun)

		// then it ran
		fakeTime.onNow = func(location *time.Location) time.Time {
			return expectedSaturdayAtTime
		}
		sched.runAndReschedule(j)

		// expected to run on next weekday
		expectedNextRun := expectedSaturdayAtTime.AddDate(0, 0, 7)
		assert.Equal(t, expectedNextRun, j.nextRun)
	})

	t.Run("every weekday starting same weekday at time that still should run at same day", func(t *testing.T) {
		startingSaturday := time.Date(2020, time.Month(8), 29, 15, 15, 15, 0, time.UTC)

		j, err := sched.Every(1).Saturday().At("15:15:20").Do(func() {})
		assert.Nil(t, err)

		// scheduler started
		fakeTime.onNow = func(location *time.Location) time.Time {
			return startingSaturday
		}
		sched.scheduleAllJobs()

		// expected to run this weekday
		expectedSaturdayAtTime := time.Date(startingSaturday.Year(), startingSaturday.Month(), startingSaturday.Day(),
			15, 15, 20, 0, time.UTC)
		assert.Equal(t, expectedSaturdayAtTime, j.nextRun)

		// then it ran
		fakeTime.onNow = func(location *time.Location) time.Time {
			return expectedSaturdayAtTime
		}
		sched.runAndReschedule(j)

		// expected to run on next weekday
		expectedNextRun := expectedSaturdayAtTime.AddDate(0, 0, 7)
		assert.Equal(t, expectedNextRun, j.nextRun)
	})

	t.Run("every weekday starting same weekday at midnight", func(t *testing.T) {
		startingSaturday := time.Date(2020, time.Month(8), 29, 15, 15, 15, 0, time.UTC)

		j, err := sched.Every(1).Saturday().Do(func() {})
		assert.Nil(t, err)

		// scheduler started
		fakeTime.onNow = func(location *time.Location) time.Time {
			return startingSaturday
		}
		sched.scheduleAllJobs()

		// expected to run next weekday since this weekday's midnight already passed
		nextSaturday := sched.roundToMidnight(startingSaturday.AddDate(0, 0, 7))
		assert.Equal(t, nextSaturday, j.nextRun)

		// then it ran
		fakeTime.onNow = func(location *time.Location) time.Time {
			return nextSaturday
		}
		sched.runAndReschedule(j)

		// expected to run on next saturday
		expectedNextRun := nextSaturday.AddDate(0, 0, 7)
		assert.Equal(t, expectedNextRun, j.nextRun)
	})

	t.Run("every tuesday starting on a monday at midnight", func(t *testing.T) {
		startingMonday := time.Date(2020, time.Month(8), 24, 15, 15, 15, 0, time.UTC)
		startingTuesday := startingMonday.AddDate(0, 0, 1)

		j, err := sched.Every(1).Tuesday().Do(func() {})
		assert.Nil(t, err)

		// scheduler started
		fakeTime.onNow = func(location *time.Location) time.Time {
			return startingMonday
		}
		sched.scheduleAllJobs()

		// expected to run this tuesday
		assert.Equal(t, sched.roundToMidnight(startingTuesday), j.nextRun)

		// then it ran
		fakeTime.onNow = func(location *time.Location) time.Time {
			return startingTuesday
		}
		sched.runAndReschedule(j)

		// expected to run on next tuesday
		expectedNextRun := sched.roundToMidnight(startingTuesday.AddDate(0, 0, 7))
		assert.Equal(t, expectedNextRun, j.nextRun)
	})

	t.Run("every tuesday starting immediately at a monday and rescheduling for tuesday's midnight", func(t *testing.T) {
		startingMonday := time.Date(2020, time.Month(8), 24, 15, 15, 15, 0, time.UTC)
		fakeTime.onNow = func(location *time.Location) time.Time {
			return startingMonday
		}
		j, err := sched.Every(1).Tuesday().StartImmediately().Do(func() {})
		assert.Nil(t, err)

		// scheduler started
		sched.scheduleAllJobs()

		// expected to run immediately
		assert.Equal(t, startingMonday, j.nextRun)

		// then it ran
		fakeTime.onNow = func(location *time.Location) time.Time {
			return startingMonday
		}
		sched.runAndReschedule(j)

		// expected to run on next tuesday
		expectedNextRun := sched.roundToMidnight(startingMonday.AddDate(0, 0, 1))
		assert.Equal(t, expectedNextRun, j.nextRun)

		// then it ran
		fakeTime.onNow = func(location *time.Location) time.Time {
			return expectedNextRun
		}
		sched.runAndReschedule(j)

		// expected to run on next tuesday
		expectedNextRun = sched.roundToMidnight(expectedNextRun.AddDate(0, 0, 7))
		assert.Equal(t, expectedNextRun, j.nextRun)
	})

	t.Run("every tuesday starting on a wednesday at midnight", func(t *testing.T) {
		startingMonday := time.Date(2020, time.Month(8), 24, 15, 15, 15, 0, time.UTC)
		startingTuesday := startingMonday.AddDate(0, 0, 1)
		startingWednesday := startingMonday.AddDate(0, 0, 2)

		j, err := sched.Every(1).Tuesday().Do(func() {})
		assert.Nil(t, err)

		// scheduler started
		fakeTime.onNow = func(location *time.Location) time.Time {
			return startingWednesday
		}
		sched.scheduleAllJobs()

		// expected to run next tuesday
		firstRun := sched.roundToMidnight(startingTuesday.AddDate(0, 0, 7))
		assert.Equal(t, firstRun, j.nextRun)

		// then it ran
		fakeTime.onNow = func(location *time.Location) time.Time {
			return firstRun
		}
		sched.runAndReschedule(j)

		// expected to run next tuesday
		assert.Equal(t, sched.roundToMidnight(startingTuesday.AddDate(0, 0, 14)), j.nextRun)
	})

	t.Run("every tuesday starting on a wednesday at time", func(t *testing.T) {
		startingMonday := time.Date(2020, time.Month(8), 24, 15, 15, 15, 0, time.UTC)
		startingTuesday := startingMonday.AddDate(0, 0, 1)
		startingWednesday := startingMonday.AddDate(0, 0, 2)

		j, err := sched.Every(1).Tuesday().At("15:15:15").Do(func() {})
		assert.Nil(t, err)

		// scheduler started
		fakeTime.onNow = func(location *time.Location) time.Time {
			return startingWednesday
		}
		sched.scheduleAllJobs()

		// expected to run next tuesday
		firstRun := startingTuesday.AddDate(0, 0, 7)
		assert.Equal(t, firstRun, j.nextRun)

		// then it ran
		fakeTime.onNow = func(location *time.Location) time.Time {
			return firstRun
		}
		sched.runAndReschedule(j)

		// expected to run next tuesday
		assert.Equal(t, startingTuesday.AddDate(0, 0, 14), j.nextRun)
	})

	t.Run("every 5 tuesdays at midnight starting on a monday", func(t *testing.T) {
		startingMonday := time.Date(2020, time.Month(8), 24, 15, 15, 15, 0, time.UTC)
		startingTuesday := startingMonday.AddDate(0, 0, 1)

		j, err := sched.Every(5).Tuesday().Do(func() {})
		assert.Nil(t, err)

		// scheduler started
		fakeTime.onNow = func(location *time.Location) time.Time {
			return startingMonday
		}
		sched.scheduleAllJobs()

		// expected to run 5 tuesdays ahead
		firstRun := sched.roundToMidnight(startingTuesday.AddDate(0, 0, 4*7)) // month 9 day 22, 4 weeks since started on monday
		assert.Equal(t, firstRun, j.nextRun)

		// then it ran
		fakeTime.onNow = func(location *time.Location) time.Time {
			return firstRun
		}
		sched.runAndReschedule(j)

		// expected to run 10 tuesdays from first run
		tenTuesdaysFromStart := sched.roundToMidnight(startingTuesday.AddDate(0, 0, 9*7)) // month 10 day 27, 9 weeks seince started on monday
		assert.Equal(t, tenTuesdaysFromStart, j.nextRun)
	})
}

func TestScheduler_ScheduleDuration(t *testing.T) {
	tick := func(date time.Time, duration time.Duration) func(location *time.Location) time.Time {
		return func(location *time.Location) time.Time {
			return date.Add(duration)
		}
	}
	getJob := func(fn func(interface{}, ...interface{}) (*Job, error)) *Job {
		j, _ := fn(func() {})
		return j
	}

	fakeTime := fakeTime{}
	sched := NewScheduler(time.UTC)
	sched.time = &fakeTime
	startRunningTime := time.Date(2020, time.August, 27, 15, 15, 15, 0, time.UTC)
	s := startRunningTime // small alias :)
	var durationTests = []struct {
		description       string
		durationType      time.Duration
		job               *Job
		wantNextSchedules []time.Time
	}{
		{
			description:       "1 second test",
			job:               getJob(sched.Every(1).Second().Do),
			durationType:      time.Second,
			wantNextSchedules: []time.Time{s.Add(time.Second), s.Add(2 * time.Second), s.Add(3 * time.Second)},
		},
		{
			description:       "2 seconds test",
			job:               getJob(sched.Every(2).Seconds().Do),
			durationType:      time.Second,
			wantNextSchedules: []time.Time{s.Add(2 * time.Second), s.Add(4 * time.Second), s.Add(6 * time.Second)},
		},
		{
			description:       "62 seconds test",
			job:               getJob(sched.Every(62).Seconds().Do),
			durationType:      time.Second,
			wantNextSchedules: []time.Time{s.Add(62 * time.Second), s.Add(62 * 2 * time.Second), s.Add(62 * 3 * time.Second)},
		},
		{
			description:       "1 minute test",
			job:               getJob(sched.Every(1).Minute().Do),
			durationType:      time.Minute,
			wantNextSchedules: []time.Time{s.Add(1 * time.Minute), s.Add(2 * time.Minute), s.Add(3 * time.Minute)},
		},
		{
			description:       "2 minutes test",
			job:               getJob(sched.Every(2).Minutes().Do),
			durationType:      time.Minute,
			wantNextSchedules: []time.Time{s.Add(2 * time.Minute), s.Add(4 * time.Minute), s.Add(6 * time.Minute)},
		},
		{
			description:       "62 minutes test",
			job:               getJob(sched.Every(62).Minutes().Do),
			durationType:      time.Minute,
			wantNextSchedules: []time.Time{s.Add(62 * time.Minute), s.Add(62 * 2 * time.Minute), s.Add(62 * 3 * time.Minute)},
		},
		{
			description:       "1 hour test",
			job:               getJob(sched.Every(1).Hour().Do),
			durationType:      time.Hour,
			wantNextSchedules: []time.Time{s.Add(1 * time.Hour), s.Add(2 * time.Hour), s.Add(3 * time.Hour)},
		},
		{
			description:       "2 hours test",
			job:               getJob(sched.Every(2).Hours().Do),
			durationType:      time.Hour,
			wantNextSchedules: []time.Time{s.Add(2 * time.Hour), s.Add(4 * time.Hour), s.Add(6 * time.Hour)},
		},
		{
			description:       "25 hours test",
			job:               getJob(sched.Every(25).Hours().Do),
			durationType:      time.Hour,
			wantNextSchedules: []time.Time{s.Add(25 * time.Hour), s.Add(25 * 2 * time.Hour), s.Add(25 * 3 * time.Hour)},
		},
	}
	for _, tt := range durationTests {
		job := tt.job

		fakeTime.onNow = func(location *time.Location) time.Time {
			return startRunningTime
		}
		sched.scheduleAllJobs()
		for j, want := range tt.wantNextSchedules {
			assert.Equal(t, want, job.nextRun, tt.description)

			// time passes, schedules rerun and reschedule job
			fakeTime.onNow = tick(startRunningTime, tt.durationType*time.Duration(job.interval)*time.Duration(j+1))
			sched.runAndReschedule(job)
		}
	}
}

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
