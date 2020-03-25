package gocron

import (
	"fmt"
	"sync"
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

func TestSeconds(t *testing.T) {
	sched := NewScheduler(time.UTC)
	jobDone := make(chan bool)
	executionTimes := make([]int64, 0, 2)
	numberOfIterations := 2

	sched.Every(2).Seconds().Do(func() {
		executionTimes = append(executionTimes, time.Now().Unix())
		if len(executionTimes) >= numberOfIterations {
			jobDone <- true
		}
	})

	stop := sched.Start()
	<-jobDone // Wait job done
	close(stop)

	assert.Equal(t, numberOfIterations, len(executionTimes), "did not run expected number of times")

	for i := 1; i < numberOfIterations; i++ {
		durationBetweenExecutions := executionTimes[i] - executionTimes[i-1]
		assert.Equal(t, int64(2), durationBetweenExecutions, "Duration between tasks does not correspond to expectations")
	}
}

func TestExecution(t *testing.T) {
	sched := NewScheduler(time.UTC)
	success := false
	sched.Every(1).Second().Do(func(mutableValue *bool) {
		*mutableValue = !*mutableValue
	}, &success)
	sched.RunAllWithDelay(1)
	assert.Equal(t, true, success, "Task did not get called")
}

func TestScheduled(t *testing.T) {
	n := NewScheduler(time.UTC)
	n.Every(1).Second().Do(task)
	if !n.Scheduled(task) {
		t.Fatal("Task was scheduled but function couldn't find it")
	}
}

// This is a basic test for the issue described here: https://github.com/jasonlvhit/gocron/issues/23
func TestScheduler_Weekdays(t *testing.T) {
	scheduler := NewScheduler(time.UTC)

	job1, _ := scheduler.Every(1).Monday().At("23:59").Do(task)
	job2, _ := scheduler.Every(1).Wednesday().At("23:59").Do(task)
	t.Logf("job1 scheduled for %s", job1.NextScheduledTime())
	t.Logf("job2 scheduled for %s", job2.NextScheduledTime())
	assert.NotEqual(t, job1.NextScheduledTime(), job2.NextScheduledTime(), "Two jobs scheduled at the same time on two different weekdays should never run at the same time")
}

// This ensures that if you schedule a job for today's weekday, but the time is already passed, it will be scheduled for
// next week at the requested time.
func TestScheduler_WeekdaysTodayAfter(t *testing.T) {
	scheduler := NewScheduler(time.UTC)
	now := time.Now().UTC()
	timeToSchedule := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute()-1, 0, 0, time.UTC)

	runTime := fmt.Sprintf("%02d:%02d", timeToSchedule.Hour(), timeToSchedule.Minute())
	job, _ := callTodaysWeekday(scheduler.Every(1), time.UTC).At(runTime).Do(task)
	t.Logf("job is scheduled for %s", job.NextScheduledTime())
	if job.NextScheduledTime().Weekday() != timeToSchedule.Weekday() {
		t.Errorf("Job scheduled for current weekday for earlier time, should still be scheduled for current weekday (but next week)")
	}
	nextWeek := time.Date(now.Year(), now.Month(), now.Day()+7, now.Hour(), now.Minute()-1, 0, 0, time.UTC)
	if !job.NextScheduledTime().Equal(nextWeek) {
		t.Errorf("Job should be scheduled for the correct time next week.\nGot %+v, expected %+v", job.NextScheduledTime(), nextWeek)
	}
}

func TestScheduleNextRunFromNow(t *testing.T) {
	sched := NewScheduler(time.UTC)
	now := time.Now().UTC()

	job, _ := sched.Every(1).Hour().StartImmediately().Do(task)
	next := job.NextScheduledTime()

	nextRounded := time.Date(next.Year(), next.Month(), next.Day(), next.Hour(), next.Minute(), next.Second(), 0, time.UTC)
	expected := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), 0, time.UTC)

	assert.Exactly(t, expected, nextRounded)
}

// This is to ensure that if you schedule a job for today's weekday, and the time hasn't yet passed, the next run time
// will be scheduled for today.
func TestScheduler_WeekdaysTodayBefore(t *testing.T) {
	scheduler := NewScheduler(time.UTC)

	now := time.Now().UTC()
	timeToSchedule := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute()+1, 0, 0, time.UTC)

	runTime := fmt.Sprintf("%02d:%02d", timeToSchedule.Hour(), timeToSchedule.Minute())
	job, _ := callTodaysWeekday(scheduler.Every(1), time.UTC).At(runTime).Do(task)
	t.Logf("job is scheduled for %s", job.NextScheduledTime())
	if !job.NextScheduledTime().Equal(timeToSchedule) {
		t.Error("Job should be run today, at the set time.")
	}
}

func Test_formatTime(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		wantHour int
		wantMin  int
		wantSec  int
		wantErr  bool
	}{
		{
			name:     "normal",
			args:     "16:18",
			wantHour: 16,
			wantMin:  18,
			wantErr:  false,
		},
		{
			name:     "normal_with_second",
			args:     "6:18:01",
			wantHour: 6,
			wantMin:  18,
			wantSec:  1,
			wantErr:  false,
		},
		{
			name:     "not_a_number",
			args:     "e:18",
			wantHour: 0,
			wantMin:  0,
			wantErr:  true,
		},
		{
			name:     "out_of_range_hour",
			args:     "25:18",
			wantHour: 0,
			wantMin:  0,
			wantErr:  true,
		},
		{
			name:     "out_of_range_minute",
			args:     "23:60",
			wantHour: 0,
			wantMin:  0,
			wantErr:  true,
		},
		{
			name:     "wrong_format",
			args:     "19:18:17:17",
			wantHour: 0,
			wantMin:  0,
			wantErr:  true,
		},
		{
			name:     "wrong_minute",
			args:     "19:1e",
			wantHour: 19,
			wantMin:  0,
			wantErr:  true,
		},
		{
			name:     "wrong_hour",
			args:     "1e:10",
			wantHour: 11,
			wantMin:  0,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHour, gotMin, gotSec, err := formatTime(tt.args)
			if tt.wantErr {
				assert.NotEqual(t, nil, err, tt.args)
				return
			}
			assert.Equal(t, tt.wantHour, gotHour, tt.args)
			assert.Equal(t, tt.wantMin, gotMin, tt.args)
			if tt.wantSec != 0 {
				assert.Equal(t, tt.wantSec, gotSec)
			} else {
				assert.Zero(t, gotSec)
			}
		})
	}
}

// utility function for testing the weekday functions *on* the current weekday.
func callTodaysWeekday(s *Scheduler, loc *time.Location) *Scheduler {
	switch time.Now().In(loc).Weekday() {
	case 0:
		s.Sunday()
	case 1:
		s.Monday()
	case 2:
		s.Tuesday()
	case 3:
		s.Wednesday()
	case 4:
		s.Thursday()
	case 5:
		s.Friday()
	case 6:
		s.Saturday()
	}
	return s
}

func TestScheduler_Remove(t *testing.T) {
	scheduler := NewScheduler(time.UTC)
	scheduler.Every(1).Minute().Do(task)
	scheduler.Every(1).Minute().Do(taskWithParams, 1, "hello")

	assert.Equal(t, 2, scheduler.Len(), "Incorrect number of jobs")

	scheduler.Remove(task)
	assert.Equal(t, 1, scheduler.Len(), "Incorrect number of jobs after removing 1 job")

	scheduler.Remove(task)
	assert.Equal(t, 1, scheduler.Len(), "Incorrect number of jobs after removing non-existent job")
}

func TestScheduler_RemoveByRef(t *testing.T) {
	scheduler := NewScheduler(time.UTC)
	job1, _ := scheduler.Every(1).Minute().Do(task)
	job2, _ := scheduler.Every(1).Minute().Do(taskWithParams, 1, "hello")

	assert.Equal(t, 2, scheduler.Len(), "Incorrect number of jobs")

	scheduler.RemoveByRef(job1)
	assert.ElementsMatch(t, []*Job{job2}, scheduler.Jobs())
}

func TestTaskAt(t *testing.T) {
	s := NewScheduler(time.UTC)

	// Schedule to run in next minute
	now := time.Now().UTC()
	dayJobDone := make(chan bool, 1)

	// Schedule every day At
	startAt := fmt.Sprintf("%02d:%02d", now.Hour(), now.Add(time.Minute).Minute())
	dayJob, _ := s.Every(1).Day().At(startAt).Do(func() {
		dayJobDone <- true
	})

	// Expected start time
	expectedStartTime := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Add(time.Minute).Minute(), 0, 0, time.UTC)
	nextRun := dayJob.NextScheduledTime()
	assert.Equal(t, expectedStartTime, nextRun)

	sStop := s.Start()
	<-dayJobDone // Wait job done
	close(sStop)
	time.Sleep(time.Second) // wait for scheduler to reschedule job

	// Expected next start time 1 day after
	expectedNextRun := expectedStartTime.AddDate(0, 0, 1)
	nextRun = dayJob.NextScheduledTime()
	assert.Equal(t, expectedNextRun, nextRun)
}

func TestTaskAtFuture(t *testing.T) {
	// Create new scheduler to have clean test env
	s := NewScheduler(time.UTC)

	now := time.Now().UTC()

	// Schedule to run in next minute
	nextMinuteTime := now.Add(time.Duration(1 * time.Minute))
	startAt := fmt.Sprintf("%02d:%02d", nextMinuteTime.Hour(), nextMinuteTime.Minute())
	shouldBeFalse := false
	dayJob, _ := s.Every(1).Day().At(startAt).Do(func() {
		shouldBeFalse = true
	})

	// Check first run
	expectedStartTime := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Add(time.Minute).Minute(), 0, 0, time.UTC)
	nextRun := dayJob.NextScheduledTime()
	assert.Equal(t, expectedStartTime, nextRun)

	s.RunPending()

	// Check next run's scheduled time
	nextRun = dayJob.NextScheduledTime()
	assert.Equal(t, expectedStartTime, nextRun)
	assert.Equal(t, false, shouldBeFalse, "Day job was not expected to run as it was in the future")
}

func TestDaily(t *testing.T) {
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

func TestWeekdayAfterToday(t *testing.T) {
	now := time.Now().UTC()

	// Create new scheduler to have clean test env
	s := NewScheduler(time.UTC)

	// Schedule job at next week day
	switch now.Weekday() {
	case time.Monday:
		s = s.Every(1).Tuesday()
	case time.Tuesday:
		s = s.Every(1).Wednesday()
	case time.Wednesday:
		s = s.Every(1).Thursday()
	case time.Thursday:
		s = s.Every(1).Friday()
	case time.Friday:
		s = s.Every(1).Saturday()
	case time.Saturday:
		s = s.Every(1).Sunday()
	case time.Sunday:
		s = s.Every(1).Monday()
	}

	weekJob, _ := s.Do(task)

	// First run
	s.scheduleNextRun(weekJob)
	exp := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, exp, weekJob.nextRun)

	// Simulate job run 7 days before
	weekJob.lastRun = weekJob.nextRun.AddDate(0, 0, -7)
	// Next run
	s.scheduleNextRun(weekJob)
	exp = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, exp, weekJob.nextRun)
}

func TestWeekdayBeforeToday(t *testing.T) {
	now := time.Now().In(time.UTC)

	// Create new scheduler to have clean test env
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

	// Simulate job run 7 days before
	weekJob.lastRun = weekJob.nextRun.AddDate(0, 0, -7)
	// Next run
	s.scheduleNextRun(weekJob)
	exp = time.Date(sixDaysFromNow.Year(), sixDaysFromNow.Month(), sixDaysFromNow.Day(), 0, 0, 0, 0, time.UTC)
	assert.Equal(t, exp, weekJob.nextRun)
}

func TestWeekdayAt(t *testing.T) {
	now := time.Now()

	hour := now.Hour()
	minute := now.Minute()
	startAt := fmt.Sprintf("%02d:%02d", hour, minute)

	// Create new scheduler to have clean test env
	s := NewScheduler(time.UTC)

	// Schedule job at next week day
	switch now.Weekday() {
	case time.Monday:
		s = s.Every(1).Tuesday().At(startAt)
	case time.Tuesday:
		s = s.Every(1).Wednesday().At(startAt)
	case time.Wednesday:
		s = s.Every(1).Thursday().At(startAt)
	case time.Thursday:
		s = s.Every(1).Friday().At(startAt)
	case time.Friday:
		s = s.Every(1).Saturday().At(startAt)
	case time.Saturday:
		s = s.Every(1).Sunday().At(startAt)
	case time.Sunday:
		s = s.Every(1).Monday().At(startAt)
	}

	weekJob, _ := s.Do(task)

	// First run
	s.scheduleNextRun(weekJob)
	exp := time.Date(now.Year(), now.Month(), now.AddDate(0, 0, 1).Day(), hour, minute, 0, 0, time.UTC)
	assert.Equal(t, exp, weekJob.nextRun)

	// Simulate job run 7 days before
	weekJob.lastRun = weekJob.nextRun.AddDate(0, 0, -7)
	// Next run
	s.scheduleNextRun(weekJob)
	exp = time.Date(now.Year(), now.Month(), now.AddDate(0, 0, 1).Day(), hour, minute, 0, 0, time.UTC)
	assert.Equal(t, exp, weekJob.nextRun)
}

type lockerMock struct {
	cache map[string]struct{}
	l     sync.Mutex
}

func (l *lockerMock) Lock(key string) (bool, error) {
	l.l.Lock()
	defer l.l.Unlock()
	if _, ok := l.cache[key]; ok {
		return false, nil
	}
	l.cache[key] = struct{}{}
	return true, nil
}

func (l *lockerMock) Unlock(key string) error {
	l.l.Lock()
	defer l.l.Unlock()
	delete(l.cache, key)
	return nil
}

func TestSetLocker(t *testing.T) {
	if locker != nil {
		t.Fail()
		t.Log("Expected locker to not be set by default")
	}

	SetLocker(&lockerMock{})

	if locker == nil {
		t.Fail()
		t.Log("Expected locker to be set")
	}
}

type lockerResult struct {
	key   string
	cycle int
	s, e  time.Time
}

func TestLocker(t *testing.T) {
	l := sync.Mutex{}

	result := make([]lockerResult, 0)
	task := func(key string, i int) {
		s := time.Now()
		time.Sleep(time.Millisecond * 100)
		e := time.Now()
		l.Lock()
		result = append(result, lockerResult{
			key:   key,
			cycle: i,
			s:     s,
			e:     e,
		})
		l.Unlock()
	}

	SetLocker(&lockerMock{
		make(map[string]struct{}),
		sync.Mutex{},
	})

	for i := 0; i < 5; i++ {
		s1 := NewScheduler(time.UTC)
		s1.Every(1).Seconds().Lock().Do(task, "A", i)

		s2 := NewScheduler(time.UTC)
		s2.Every(1).Seconds().Lock().Do(task, "B", i)

		s3 := NewScheduler(time.UTC)
		s3.Every(1).Seconds().Lock().Do(task, "C", i)

		stop1 := s1.Start()
		stop2 := s2.Start()
		stop3 := s3.Start()

		time.Sleep(time.Millisecond * 100)

		close(stop1)
		close(stop2)
		close(stop3)

		for i := 0; i < len(result)-1; i++ {
			for j := i + 1; j < len(result); j++ {
				iBefJ := result[i].s.Before(result[j].s) && result[i].e.Before(result[j].s)
				jBefI := result[j].s.Before(result[i].s) && result[j].e.Before(result[i].s)
				if !iBefJ && !jBefI {
					t.Fatalf("\n2 operations ran concurrently:\n%s\n%d\n%s\n%s\n**********\n%s\n%d\n%s\n%s\n",
						result[i].key, result[i].cycle, result[i].s, result[i].e,
						result[j].key, result[j].cycle, result[j].s, result[j].e)
				}
			}
		}
	}
}

func TestGetAllJobs(t *testing.T) {
	s := NewScheduler(time.UTC)
	s.Every(1).Minute().Do(task)
	s.Every(2).Minutes().Do(task)
	s.Every(3).Minutes().Do(task)
	s.Every(4).Minutes().Do(task)
	js := s.Jobs()
	assert.Len(t, js, 4)
}

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

func TestGetAt(t *testing.T) {
	j, _ := NewScheduler(time.UTC).Every(1).Minute().At("10:30").Do(task)
	assert.Equal(t, "10:30", j.GetAt())
}

func TestGetWeekday(t *testing.T) {
	j, _ := NewScheduler(time.UTC).Every(1).Weekday(time.Wednesday).Do(task)
	assert.Equal(t, time.Wednesday, j.GetWeekday())
}
