package gocron

import (
	"fmt"
	"reflect"
	"sort"
	"time"
)

// Scheduler struct, the only data member is the list of jobs.
// - implements the sort.Interface{} for sorting jobs, by the time nextRun
type Scheduler struct {
	jobs []*Job
	loc  *time.Location
}

// NewScheduler creates a new scheduler
func NewScheduler(loc *time.Location) *Scheduler {
	return &Scheduler{
		jobs: newEmptyJobSlice(),
		loc:  loc,
	}
}

// Start all the pending jobs
// Add seconds ticker
func (s *Scheduler) Start() chan bool {
	stopped := make(chan bool)
	ticker := time.NewTicker(1 * time.Second)

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

func (s *Scheduler) Len() int {
	return len(s.jobs)
}

func (s *Scheduler) Swap(i, j int) {
	s.jobs[i], s.jobs[j] = s.jobs[j], s.jobs[i]
}

func (s *Scheduler) Less(i, j int) bool {
	return s.jobs[j].nextRun.Unix() >= s.jobs[i].nextRun.Unix()
}

// ChangeLoc changes the default time location
func (s *Scheduler) ChangeLoc(newLocation *time.Location) {
	s.loc = newLocation
}

// scheduleNextRun Compute the instant when this job should run next
func (s *Scheduler) scheduleNextRun(j *Job) error {
	now := time.Now().In(s.loc)
	if j.lastRun == time.Unix(0, 0) {
		j.lastRun = now
	}

	if j.nextRun.After(now) {
		return nil
	}

	periodDuration, err := j.periodDuration()
	if err != nil {
		return err
	}

	switch j.unit {
	case seconds, minutes, hours:
		j.nextRun = j.lastRun.Add(periodDuration)
	case days:
		j.nextRun = s.roundToMidnight(j.lastRun)
		j.nextRun = j.nextRun.Add(j.atTime)
	case weeks:
		j.nextRun = s.roundToMidnight(j.lastRun)
		dayDiff := int(j.startDay)
		dayDiff -= int(j.nextRun.Weekday())
		if dayDiff != 0 {
			j.nextRun = j.nextRun.Add(time.Duration(dayDiff) * 24 * time.Hour)
		}
		j.nextRun = j.nextRun.Add(j.atTime)
	}

	// advance to next possible schedule
	for j.nextRun.Before(now) || j.nextRun.Before(j.lastRun) {
		j.nextRun = j.nextRun.Add(periodDuration)
	}

	return nil
}

// roundToMidnight truncate time to midnight
func (s *Scheduler) roundToMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, s.loc)
}

// Get the current runnable jobs, which shouldRun is True
func (s *Scheduler) getRunnableJobs() []*Job {
	var runnableJobs []*Job
	sort.Sort(s)
	for _, job := range s.jobs {
		if job.shouldRun() {
			runnableJobs = append(runnableJobs, job)
		} else {
			break
		}
	}
	return runnableJobs
}

// NextRun datetime when the next job should run.
func (s *Scheduler) NextRun() (*Job, time.Time) {
	if len(s.jobs) <= 0 {
		return nil, time.Now()
	}
	sort.Sort(s)
	return s.jobs[0], s.jobs[0].nextRun
}

// Every schedule a new periodic job with interval
func (s *Scheduler) Every(interval uint64) *Scheduler {
	job := NewJob(interval)
	s.jobs = append(s.jobs, job)
	return s
}

// RunPending runs all the jobs that are scheduled to run.
func (s *Scheduler) RunPending() {
	runnableJobs := s.getRunnableJobs()
	for _, job := range runnableJobs {
		s.runJob(job)
	}
}

func (s *Scheduler) runJob(job *Job) error {
	if job.lock {
		if locker == nil {
			return fmt.Errorf("trying to lock %s with nil locker", job.jobFunc)
		}
		key := getFunctionKey(job.jobFunc)

		locker.Lock(key)
		defer locker.Unlock(key)
	}
	job.run()
	err := s.scheduleNextRun(job)
	if err != nil {
		return err
	}
	return nil
}

// RunAll run all jobs regardless if they are scheduled to run or not
func (s *Scheduler) RunAll() {
	s.RunAllWithDelay(0)
}

// RunAllwithDelay runs all jobs with delay seconds
func (s *Scheduler) RunAllWithDelay(d int) {
	for _, job := range s.jobs {
		err := s.runJob(job)
		if err != nil {
			continue
		}
		time.Sleep(time.Duration(d) * time.Second)
	}
}

// Remove specific job j by function
func (s *Scheduler) Remove(j interface{}) {
	s.removeByCondition(func(someJob *Job) bool {
		return someJob.jobFunc == getFunctionName(j)
	})
}

// RemoveByRef removes specific job j by reference
func (s *Scheduler) RemoveByRef(j *Job) {
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

// Scheduled checks if specific job j was already added
func (s *Scheduler) Scheduled(j interface{}) bool {
	for _, job := range s.jobs {
		if job.jobFunc == getFunctionName(j) {
			return true
		}
	}
	return false
}

// Clear delete all scheduled jobs
func (s *Scheduler) Clear() {
	s.jobs = newEmptyJobSlice()
}

func newEmptyJobSlice() []*Job {
	const initialCapacity = 256
	return make([]*Job, 0, initialCapacity)
}

// Do specifies the jobFunc that should be called every time the job runs
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

	if !j.startsImmediately {
		if err := s.scheduleNextRun(j); err != nil {
			return nil, err
		}
	}

	return j, nil
}

// At schedules job at specific time of day
// s.Every(1).Day().At("10:30:01").Do(task)
// s.Every(1).Monday().At("10:30:01").Do(task)
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

// GetAt returns the specific time of day the job will run at
//	s.Every(1).Day().At("10:30").GetAt() == "10:30"
func (j *Job) GetAt() string {
	return fmt.Sprintf("%d:%d", j.atTime/time.Hour, (j.atTime%time.Hour)/time.Minute)
}

// StartAt schedules the next run of the job
func (s *Scheduler) StartAt(t time.Time) *Scheduler {
	s.getCurrentJob().nextRun = t
	return s
}

// StartImmediately sets the jobs next run as soon as the scheduler starts
func (s *Scheduler) StartImmediately() *Scheduler {
	job := s.getCurrentJob()
	job.nextRun = time.Now().In(s.loc)
	job.startsImmediately = true
	return s
}

// setUnit sets unit type
func (s *Scheduler) setUnit(unit timeUnit) {
	currentJob := s.getCurrentJob()
	currentJob.unit = unit
}

// Seconds set the unit with seconds
func (s *Scheduler) Seconds() *Scheduler {
	s.setUnit(seconds)
	return s
}

// Minutes set the unit with minute
func (s *Scheduler) Minutes() *Scheduler {
	s.setUnit(minutes)
	return s
}

// Hours set the unit with hours
func (s *Scheduler) Hours() *Scheduler {
	s.setUnit(hours)
	return s
}

// Second sets the unit with second
func (s *Scheduler) Second() *Scheduler {
	s.mustInterval(1)
	return s.Seconds()
}

// Minute sets the unit  with minute, which interval is 1
func (s *Scheduler) Minute() *Scheduler {
	s.mustInterval(1)
	return s.Minutes()
}

// Hour sets the unit with hour, which interval is 1
func (s *Scheduler) Hour() *Scheduler {
	s.mustInterval(1)
	return s.Hours()
}

// Day sets the job's unit with day, which interval is 1
func (s *Scheduler) Day() *Scheduler {
	s.mustInterval(1)
	s.setUnit(days)
	return s
}

// Days set the job's unit with days
func (s *Scheduler) Days() *Scheduler {
	s.setUnit(days)
	return s
}

// Week sets the job's unit with week, which interval is 1
func (s *Scheduler) Week() *Scheduler {
	s.mustInterval(1)
	s.setUnit(weeks)
	return s
}

// Weeks sets the job's unit with weeks
func (s *Scheduler) Weeks() *Scheduler {
	s.setUnit(weeks)
	return s
}

// Weekday start job on specific Weekday
func (s *Scheduler) Weekday(startDay time.Weekday) *Scheduler {
	s.mustInterval(1)
	s.getCurrentJob().startDay = startDay
	s.setUnit(weeks)
	return s
}

// Monday set the start day with Monday
// - s.Every(1).Monday().Do(task)
func (s *Scheduler) Monday() *Scheduler {
	return s.Weekday(time.Monday)
}

// Tuesday sets the job start day Tuesday
func (s *Scheduler) Tuesday() *Scheduler {
	return s.Weekday(time.Tuesday)
}

// Wednesday sets the job start day Wednesday
func (s *Scheduler) Wednesday() *Scheduler {
	return s.Weekday(time.Wednesday)
}

// Thursday sets the job start day Thursday
func (s *Scheduler) Thursday() *Scheduler {
	return s.Weekday(time.Thursday)
}

// Friday sets the job start day Friday
func (s *Scheduler) Friday() *Scheduler {
	return s.Weekday(time.Friday)
}

// Saturday sets the job start day Saturday
func (s *Scheduler) Saturday() *Scheduler {
	return s.Weekday(time.Saturday)
}

// Sunday sets the job start day Sunday
func (s *Scheduler) Sunday() *Scheduler {
	return s.Weekday(time.Sunday)
}

func (s *Scheduler) getCurrentJob() *Job {
	return s.jobs[len(s.jobs)-1]
}

// Lock prevents job to run from multiple instances of gocron
func (s *Scheduler) Lock() *Scheduler {
	s.getCurrentJob().lock = true
	return s
}

// set the job's unit with seconds,minutes,hours...
func (s *Scheduler) mustInterval(i uint64) error {
	if s.getCurrentJob().interval != i {
		return fmt.Errorf("interval must be %d", i)
	}
	return nil
}
