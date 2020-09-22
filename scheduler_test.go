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
	assert.Equal(t, now.Add(time.Second*5), nextRun)
	scheduler.Remove(job)

	// Without StartAt
	job, _ = scheduler.Every(3).Seconds().Do(func() {})
	scheduler.scheduleNextRun(job)
	_, nextRun = scheduler.NextRun()
	assert.Equal(t, now.Add(time.Second*3).Second(), nextRun.Second())
}

func TestScheduler_FirstSchedule(t *testing.T) {
	day := time.Hour * 24
	janFirst2020 := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	monday := time.Date(2020, time.January, 6, 0, 0, 0, 0, time.UTC)
	tuesday := monday.AddDate(0, 0, 1)
	wednesday := monday.AddDate(0, 0, 2)

	fakeTime := fakeTime{}
	sched := NewScheduler(time.UTC)
	sched.time = &fakeTime
	var tests = []struct {
		name             string
		job              *Job
		startRunningTime time.Time
		wantNextSchedule time.Time
	}{
		// SECONDS
		{
			name:             "every second test",
			job:              getJob(sched.Every(1).Second().Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: janFirst2020.Add(time.Second),
		},
		{
			name:             "every 62 seconds test",
			job:              getJob(sched.Every(62).Seconds().Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: janFirst2020.Add(62 * time.Second),
		},
		// MINUTES
		{
			name:             "every minute test",
			job:              getJob(sched.Every(1).Minute().Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: janFirst2020.Add(1 * time.Minute),
		},
		{
			name:             "every 62 minutes test",
			job:              getJob(sched.Every(62).Minutes().Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: janFirst2020.Add(62 * time.Minute),
		},
		// HOURS
		{
			name:             "every hour test",
			job:              getJob(sched.Every(1).Hour().Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: janFirst2020.Add(1 * time.Hour),
		},
		{
			name:             "every 25 hours test",
			job:              getJob(sched.Every(25).Hours().Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: janFirst2020.Add(25 * time.Hour),
		},
		// DAYS
		{
			name:             "every day at midnight",
			job:              getJob(sched.Every(1).Day().Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: janFirst2020.Add(1 * day),
		},
		{
			name:             "every day at 09:30AM with scheduler starting before 09:30AM should run at same day at time",
			job:              getJob(sched.Every(1).Day().At("09:30").Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: time.Date(2020, time.January, 1, 9, 30, 0, 0, time.UTC),
		},
		{
			name:             "every day at 09:30AM with scheduler starting after 09:30AM should run tomorrow at time",
			job:              getJob(sched.Every(1).Day().At("09:30").Do),
			startRunningTime: janFirst2020.Add(10 * time.Hour),
			wantNextSchedule: time.Date(2020, time.January, 2, 9, 30, 0, 0, time.UTC),
		},
		{
			name:             "every 31 days at midnight should run 31 days later",
			job:              getJob(sched.Every(31).Days().Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: time.Date(2020, time.February, 1, 0, 0, 0, 0, time.UTC),
		},
		// WEEKS
		{
			name:             "every week should run in 7 days",
			job:              getJob(sched.Every(1).Week().Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: time.Date(2020, time.January, 8, 0, 0, 0, 0, time.UTC),
		},
		{
			name:             "every week at 09:30AM should run in 7 days at 09:30AM",
			job:              getJob(sched.Every(1).Week().At("09:30").Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: time.Date(2020, time.January, 8, 9, 30, 0, 0, time.UTC),
		},
		{
			name:             "every two weeks at 09:30AM should run in 14 days at 09:30AM",
			job:              getJob(sched.Every(2).Weeks().At("09:30").Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: time.Date(2020, time.January, 15, 9, 30, 0, 0, time.UTC),
		},
		{
			name:             "every 31 weeks at midnight should run in 217 days (2020 was a leap year)",
			job:              getJob(sched.Every(31).Weeks().Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: time.Date(2020, time.August, 5, 0, 0, 0, 0, time.UTC),
		},
		// MONTHS
		{
			name:             "every month at first day starting at first day should run at same day",
			job:              getJob(sched.Every(1).Month(1).Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:             "every month at first day at time, but started late, should run next month",
			job:              getJob(sched.Every(1).Month(1).At("09:30").Do),
			startRunningTime: janFirst2020.Add(10 * time.Hour),
			wantNextSchedule: time.Date(2020, time.February, 1, 9, 30, 0, 0, time.UTC),
		},
		{
			name:             "every month at day 2, and started in day 1, day should run day 2 same month",
			job:              getJob(sched.Every(1).Month(2).Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: time.Date(2020, time.January, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name:             "every month at day 1, but started on the day 8, should run next month at day 1",
			job:              getJob(sched.Every(1).Month(1).Do),
			startRunningTime: time.Date(2020, time.January, 8, 0, 0, 0, 0, time.UTC),
			wantNextSchedule: time.Date(2020, time.February, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:             "every 2 months at day 1, starting at day 1, should run in 2 months",
			job:              getJob(sched.Every(2).Months(1).Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: time.Date(2020, time.March, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:             "every 2 months at day 2, starting at day 1, should run in 2 months",
			job:              getJob(sched.Every(2).Months(2).Do),
			startRunningTime: janFirst2020,
			wantNextSchedule: time.Date(2020, time.March, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name:             "every 2 months at day 1, starting at day 2, should run 2 months later at day 1",
			job:              getJob(sched.Every(2).Months(1).Do),
			startRunningTime: time.Date(2020, time.January, 2, 0, 0, 0, 0, time.UTC),
			wantNextSchedule: time.Date(2020, time.March, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:             "every 13 months at day 1, starting at day 2 run in 13 months",
			job:              getJob(sched.Every(13).Months(1).Do),
			startRunningTime: time.Date(2020, time.January, 2, 0, 0, 0, 0, time.UTC),
			wantNextSchedule: time.Date(2021, time.February, 1, 0, 0, 0, 0, time.UTC),
		},
		// WEEKDAYS
		{
			name:             "every tuesday starting on a monday should run this tuesday",
			job:              getJob(sched.Every(1).Tuesday().Do),
			startRunningTime: monday,
			wantNextSchedule: tuesday,
		},
		{
			name:             "every tuesday starting on tuesday a should run on same tuesday",
			job:              getJob(sched.Every(1).Tuesday().Do),
			startRunningTime: tuesday,
			wantNextSchedule: tuesday,
		},
		{
			name:             "every 2 tuesdays starting on a tuesday should run next tuesday",
			job:              getJob(sched.Every(2).Tuesday().Do),
			startRunningTime: tuesday,
			wantNextSchedule: tuesday.AddDate(0, 0, 7),
		},
		{
			name:             "every tuesday starting on a wednesday should run next tuesday",
			job:              getJob(sched.Every(1).Tuesday().Do),
			startRunningTime: wednesday,
			wantNextSchedule: tuesday.AddDate(0, 0, 7),
		},
		{
			name:             "starting on a monday, every monday at time to happen should run at same day at time",
			job:              getJob(sched.Every(1).Monday().At("09:00").Do),
			startRunningTime: monday,
			wantNextSchedule: monday.Add(9 * time.Hour),
		},
		{
			name:             "starting on a monday, every monday at time that already passed should run at next week at time",
			job:              getJob(sched.Every(1).Monday().At("09:00").Do),
			startRunningTime: monday.Add(10 * time.Hour),
			wantNextSchedule: monday.AddDate(0, 0, 7).Add(9 * time.Hour),
		},
	}

	for i, tt := range tests {
		fakeTime.onNow = func(location *time.Location) time.Time { // scheduler started
			return tests[i].startRunningTime
		}

		job := tt.job
		sched.scheduleNextRun(job)
		assert.Equal(t, tt.wantNextSchedule, job.nextRun, tt.name)
	}
}

func getJob(fn func(interface{}, ...interface{}) (*Job, error)) *Job {
	j, _ := fn(func() {})
	return j
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

func TestScheduler_Do(t *testing.T) {
	t.Run("adding a new job before scheduler starts does not schedule job", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		s.running = false
		job, err := s.Every(1).Second().Do(func() {})
		assert.Equal(t, nil, err)
		assert.True(t, job.nextRun.IsZero())
	})

	t.Run("adding a new job when scheduler is running schedules job", func(t *testing.T) {
		s := NewScheduler(time.UTC)
		s.running = true
		job, err := s.Every(1).Second().Do(func() {})
		assert.Equal(t, nil, err)
		assert.False(t, job.nextRun.IsZero())
	})
}
