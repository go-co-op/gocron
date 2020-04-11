package gocron

import (
	"fmt"
	"reflect"
	"sort"
	"time"
)

// Scheduler struct stores a list of Jobs and the location of time Scheduler
// Scheduler implements the sort.Interface{} for sorting Jobs, by the time of nextRun
type Scheduler struct {
	jobs []*Job
	loc  *time.Location
	time timeHelper // an instance of timeHelper to interact with the time package
}

// NewScheduler creates a new Scheduler
func NewScheduler(loc *time.Location) *Scheduler {
	return &Scheduler{
		jobs: newEmptyJobSlice(),
		loc:  loc,
		time: newTimeHelper(),
	}
}

// StartBlocking starts all the pending jobs using a second-long ticker and blocks the current thread
func (s *Scheduler) StartBlocking() {
	<-s.StartAsync()
}

// StartAsync starts a goroutine that runs all the pending using a second-long ticker
func (s *Scheduler) StartAsync() chan struct{} {
	stopped := make(chan struct{})
	ticker := s.time.NewTicker(1 * time.Second)

	go func() {
		for {
			select {
			case <-ticker.C:
				s.RunPending()
			case <-stopped:
				ticker.Stop()
				return
			}
		}
	}()

	return stopped
}

// Jobs returns the list of Jobs from the Scheduler
func (s *Scheduler) Jobs() []*Job {
	return s.jobs
}

// Len returns the number of Jobs in the Scheduler
func (s *Scheduler) Len() int {
	return len(s.jobs)
}

// Swap
func (s *Scheduler) Swap(i, j int) {
	s.jobs[i], s.jobs[j] = s.jobs[j], s.jobs[i]
}

func (s *Scheduler) Less(i, j int) bool {
	return s.jobs[j].nextRun.Unix() >= s.jobs[i].nextRun.Unix()
}

// ChangeLocation changes the default time location
func (s *Scheduler) ChangeLocation(newLocation *time.Location) {
	s.loc = newLocation
}

// scheduleNextRun Compute the instant when this Job should run next
func (s *Scheduler) scheduleNextRun(j *Job) error {
	now := s.time.Now(s.loc)

	switch j.unit {
	case seconds, minutes, hours:
		j.nextRun = j.lastRun.Add(j.periodDuration)
	case days:
		j.nextRun = s.roundToMidnight(j.lastRun)
		j.nextRun = j.nextRun.Add(j.atTime).Add(j.periodDuration)
	case weeks:
		j.nextRun = s.roundToMidnight(j.lastRun)
		dayDiff := int(j.startDay)
		dayDiff -= int(j.nextRun.Weekday())
		if dayDiff != 0 {
			j.nextRun = j.nextRun.Add(time.Duration(dayDiff) * 24 * time.Hour)
		}
		j.nextRun = j.nextRun.Add(j.atTime)
	}

	// advance to next possible Schedule
	for j.nextRun.Before(now) || j.nextRun.Before(j.lastRun) {
		j.nextRun = j.nextRun.Add(j.periodDuration)
	}

	return nil
}

// roundToMidnight truncate time to midnight
func (s *Scheduler) roundToMidnight(t time.Time) time.Time {
	return s.time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, s.loc)
}

// Get the current runnable Jobs, which shouldRun is True
func (s *Scheduler) runnableJobs() []*Job {
	var runnableJobs []*Job
	sort.Sort(s)
	for _, job := range s.jobs {
		if s.shouldRun(job) {
			runnableJobs = append(runnableJobs, job)
		} else {
			break
		}
	}
	return runnableJobs
}

// NextRun datetime when the next Job should run.
func (s *Scheduler) NextRun() (*Job, time.Time) {
	if len(s.jobs) <= 0 {
		return nil, s.time.Now(s.loc)
	}
	sort.Sort(s)
	return s.jobs[0], s.jobs[0].nextRun
}

// Every schedules a new periodic Job with interval
func (s *Scheduler) Every(interval uint64) *Scheduler {
	job := NewJob(interval)
	s.jobs = append(s.jobs, job)
	return s
}

// RunPending runs all the Jobs that are scheduled to run.
func (s *Scheduler) RunPending() {
	runnableJobs := s.runnableJobs()
	for _, job := range runnableJobs {
		s.runAndReschedule(job) // we should handle this error somehow
	}
}

func (s *Scheduler) runAndReschedule(job *Job) error {
	if err := s.run(job); err != nil {
		return err
	}
	if err := s.scheduleNextRun(job); err != nil {
		return err
	}
	return nil
}

func (s *Scheduler) run(job *Job) error {
	if job.lock {
		if locker == nil {
			return fmt.Errorf("trying to lock %s with nil locker", job.jobFunc)
		}
		key := getFunctionKey(job.jobFunc)

		locker.Lock(key)
		defer locker.Unlock(key)
	}

	job.lastRun = s.time.Now(s.loc)
	go job.run()

	return nil
}

// RunAll run all Jobs regardless if they are scheduled to run or not
func (s *Scheduler) RunAll() {
	s.RunAllWithDelay(0)
}

// RunAllWithDelay runs all Jobs with delay seconds
func (s *Scheduler) RunAllWithDelay(d int) {
	for _, job := range s.jobs {
		err := s.run(job)
		if err != nil {
			continue
		}
		s.time.Sleep(time.Duration(d) * time.Second)
	}
}

// Remove specific Job j by function
func (s *Scheduler) Remove(j interface{}) {
	s.removeByCondition(func(someJob *Job) bool {
		return someJob.jobFunc == getFunctionName(j)
	})
}

// RemoveByReference removes specific Job j by reference
func (s *Scheduler) RemoveByReference(j *Job) {
	s.removeByCondition(func(someJob *Job) bool {
		return someJob == j
	})
}

func (s *Scheduler) removeByCondition(shouldRemove func(*Job) bool) {
	for i, job := range s.jobs {
		if shouldRemove(job) {
			s.jobs = removeAtIndex(s.jobs, i)
		}
	}
}

func removeAtIndex(jobs []*Job, i int) []*Job {
	if i == len(jobs)-1 {
		return jobs[:i]
	}
	jobs = append(jobs[:i], jobs[i+1:]...)
	return jobs
}

// Scheduled checks if specific Job j was already added
func (s *Scheduler) Scheduled(j interface{}) bool {
	for _, job := range s.jobs {
		if job.jobFunc == getFunctionName(j) {
			return true
		}
	}
	return false
}

// Clear delete all scheduled Jobs
func (s *Scheduler) Clear() {
	s.jobs = newEmptyJobSlice()
}

func newEmptyJobSlice() []*Job {
	const initialCapacity = 256
	return make([]*Job, 0, initialCapacity)
}

// Do specifies the jobFunc that should be called every time the Job runs
func (s *Scheduler) Do(jobFun interface{}, params ...interface{}) (*Job, error) {
	j := s.getCurrentJob()
	if j.err != nil {
		return nil, j.err
	}

	typ := reflect.TypeOf(jobFun)
	if typ.Kind() != reflect.Func {
		return nil, ErrNotAFunction
	}

	fname := getFunctionName(jobFun)
	j.funcs[fname] = jobFun
	j.fparams[fname] = params
	j.jobFunc = fname

	if j.periodDuration == 0 {
		err := j.setPeriodDuration()
		if err != nil {
			return nil, err
		}
	}

	if !j.startsImmediately {

		if j.lastRun == s.time.Unix(0, 0) {
			j.lastRun = s.time.Now(s.loc)

			if j.atTime != 0 {
				j.lastRun = j.lastRun.Add(-j.periodDuration)
			}
		}

		if err := s.scheduleNextRun(j); err != nil {
			return nil, err
		}
	}

	return j, nil
}

// At schedules the Job at a specific time of day in the form "HH:MM:SS" or "HH:MM"
func (s *Scheduler) At(t string) *Scheduler {
	j := s.getCurrentJob()
	hour, min, sec, err := formatTime(t)
	if err != nil {
		j.err = ErrTimeFormat
		return s
	}
	// save atTime start as duration from midnight
	j.atTime = time.Duration(hour)*time.Hour + time.Duration(min)*time.Minute + time.Duration(sec)*time.Second
	return s
}

// StartAt schedules the next run of the Job
func (s *Scheduler) StartAt(t time.Time) *Scheduler {
	s.getCurrentJob().nextRun = t
	return s
}

// StartImmediately sets the Jobs next run as soon as the scheduler starts
func (s *Scheduler) StartImmediately() *Scheduler {
	job := s.getCurrentJob()
	job.nextRun = s.time.Now(s.loc)
	job.startsImmediately = true
	return s
}

// shouldRun returns true if the Job should be run now
func (s *Scheduler) shouldRun(j *Job) bool {
	return s.time.Now(s.loc).Unix() >= j.nextRun.Unix()
}

// setUnit sets the unit type
func (s *Scheduler) setUnit(unit timeUnit) {
	currentJob := s.getCurrentJob()
	currentJob.unit = unit
}

// Second sets the unit with seconds
func (s *Scheduler) Second() *Scheduler {
	return s.Seconds()
}

// Seconds sets the unit with seconds
func (s *Scheduler) Seconds() *Scheduler {
	s.setUnit(seconds)
	return s
}

// Minute sets the unit with minutes
func (s *Scheduler) Minute() *Scheduler {
	return s.Minutes()
}

// Minutes sets the unit with minutes
func (s *Scheduler) Minutes() *Scheduler {
	s.setUnit(minutes)
	return s
}

// Hour sets the unit with hours
func (s *Scheduler) Hour() *Scheduler {
	return s.Hours()
}

// Hours sets the unit with hours
func (s *Scheduler) Hours() *Scheduler {
	s.setUnit(hours)
	return s
}

// Day sets the unit with days
func (s *Scheduler) Day() *Scheduler {
	s.setUnit(days)
	return s
}

// Days set the unit with days
func (s *Scheduler) Days() *Scheduler {
	s.setUnit(days)
	return s
}

// Week sets the unit with weeks
func (s *Scheduler) Week() *Scheduler {
	s.setUnit(weeks)
	return s
}

// Weeks sets the unit with weeks
func (s *Scheduler) Weeks() *Scheduler {
	s.setUnit(weeks)
	return s
}

// Weekday sets the start with a specific weekday weekday
func (s *Scheduler) Weekday(startDay time.Weekday) *Scheduler {
	s.getCurrentJob().startDay = startDay
	s.setUnit(weeks)
	return s
}

// Monday sets the start day as Monday
func (s *Scheduler) Monday() *Scheduler {
	return s.Weekday(time.Monday)
}

// Tuesday sets the start day as Tuesday
func (s *Scheduler) Tuesday() *Scheduler {
	return s.Weekday(time.Tuesday)
}

// Wednesday sets the start day as Wednesday
func (s *Scheduler) Wednesday() *Scheduler {
	return s.Weekday(time.Wednesday)
}

// Thursday sets the start day as Thursday
func (s *Scheduler) Thursday() *Scheduler {
	return s.Weekday(time.Thursday)
}

// Friday sets the start day as Friday
func (s *Scheduler) Friday() *Scheduler {
	return s.Weekday(time.Friday)
}

// Saturday sets the start day as Saturday
func (s *Scheduler) Saturday() *Scheduler {
	return s.Weekday(time.Saturday)
}

// Sunday sets the start day as Sunday
func (s *Scheduler) Sunday() *Scheduler {
	return s.Weekday(time.Sunday)
}

func (s *Scheduler) getCurrentJob() *Job {
	return s.jobs[len(s.jobs)-1]
}

// Lock prevents Job to run from multiple instances of gocron
func (s *Scheduler) Lock() *Scheduler {
	s.getCurrentJob().lock = true
	return s
}
