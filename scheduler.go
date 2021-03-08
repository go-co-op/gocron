package gocron

import (
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
)

type limitMode int8

// Scheduler struct stores a list of Jobs and the location of time Scheduler
// Scheduler implements the sort.Interface{} for sorting Jobs, by the time of nextRun
type Scheduler struct {
	jobsMutex sync.RWMutex
	jobs      []*Job

	locationMutex sync.RWMutex
	location      *time.Location
	runningMutex  sync.RWMutex
	running       bool // represents if the scheduler is running at the moment or not

	time     timeWrapper // wrapper around time.Time
	executor *executor   // executes jobs passed via chan

	tags map[string]struct{} // for storing tags when unique tags is set
}

// NewScheduler creates a new Scheduler
func NewScheduler(loc *time.Location) *Scheduler {
	executor := newExecutor()

	return &Scheduler{
		jobs:     make([]*Job, 0),
		location: loc,
		running:  false,
		time:     &trueTime{},
		executor: &executor,
	}
}

// SetMaxConcurrentJobs limits how many jobs can be running at the same time.
// This is useful when running resource intensive jobs and a precise start time is not critical.
func (s *Scheduler) SetMaxConcurrentJobs(n int, mode limitMode) {
	s.executor.maxRunningJobs = semaphore.NewWeighted(int64(n))
	s.executor.limitMode = mode
}

// StartBlocking starts all jobs and blocks the current thread
func (s *Scheduler) StartBlocking() {
	s.StartAsync()
	<-make(chan bool)
}

// StartAsync starts all jobs without blocking the current thread
func (s *Scheduler) StartAsync() {
	if !s.IsRunning() {
		s.start()
	}
}

//start starts the scheduler, scheduling and running jobs
func (s *Scheduler) start() {
	go s.executor.start()
	s.setRunning(true)
	s.runJobs(s.Jobs())
}

func (s *Scheduler) runJobs(jobs []*Job) {
	for _, job := range jobs {
		s.scheduleNextRun(job)
	}
}

func (s *Scheduler) setRunning(b bool) {
	s.runningMutex.Lock()
	defer s.runningMutex.Unlock()
	s.running = b
}

// IsRunning returns true if the scheduler is running
func (s *Scheduler) IsRunning() bool {
	s.runningMutex.RLock()
	defer s.runningMutex.RUnlock()
	return s.running
}

// Jobs returns the list of Jobs from the Scheduler
func (s *Scheduler) Jobs() []*Job {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	return s.jobs
}

func (s *Scheduler) setJobs(jobs []*Job) {
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()
	s.jobs = jobs
}

// Len returns the number of Jobs in the Scheduler - implemented for sort
func (s *Scheduler) Len() int {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	return len(s.jobs)
}

// Swap places each job into the other job's position given
// the provided job indexes.
func (s *Scheduler) Swap(i, job int) {
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()
	s.jobs[i], s.jobs[job] = s.jobs[job], s.jobs[i]
}

// Less compares the next run of jobs based on their index.
// Returns true if the second job is after the first.
func (s *Scheduler) Less(first, second int) bool {
	return s.Jobs()[second].NextRun().Unix() >= s.Jobs()[first].NextRun().Unix()
}

// ChangeLocation changes the default time location
func (s *Scheduler) ChangeLocation(newLocation *time.Location) {
	s.locationMutex.Lock()
	defer s.locationMutex.Unlock()
	s.location = newLocation
}

// Location provides the current location set on the scheduler
func (s *Scheduler) Location() *time.Location {
	s.locationMutex.RLock()
	defer s.locationMutex.RUnlock()
	return s.location
}

// scheduleNextRun Compute the instant when this Job should run next
func (s *Scheduler) scheduleNextRun(job *Job) {
	now := s.now()
	lastRun := job.LastRun()

	if !s.jobPresent(job) {
		return
	}

	if job.getStartsImmediately() {
		s.run(job)
		job.setStartsImmediately(false)
	}

	if job.neverRan() {
		// Increment startAtTime until it is in the future
		for job.startAtTime.Before(now) && !job.startAtTime.IsZero() {
			job.startAtTime = job.startAtTime.Add(s.durationToNextRun(job.startAtTime, job))
		}
		lastRun = now
	}

	if !job.shouldRun() {
		s.RemoveByReference(job)
		return
	}

	durationToNextRun := s.durationToNextRun(lastRun, job)
	job.setNextRun(lastRun.Add(durationToNextRun))
	job.setTimer(time.AfterFunc(durationToNextRun, func() {
		s.run(job)
		s.scheduleNextRun(job)
	}))
}

func (s *Scheduler) durationToNextRun(lastRun time.Time, job *Job) time.Duration {
	// job can be scheduled with .StartAt()
	if job.getStartAtTime().After(lastRun) {
		return job.getStartAtTime().Sub(s.now())
	}

	var d time.Duration
	switch job.unit {
	case milliseconds, seconds, minutes, hours:
		d = s.calculateDuration(job)
	case days:
		d = s.calculateDays(job, lastRun)
	case weeks:
		if job.scheduledWeekday != nil { // weekday selected, Every().Monday(), for example
			d = s.calculateWeekday(job, lastRun)
		} else {
			d = s.calculateWeeks(job, lastRun)
		}
	case months:
		d = s.calculateMonths(job, lastRun)
	case duration:
		d = job.duration
	}
	return d
}

func (s *Scheduler) calculateMonths(job *Job, lastRun time.Time) time.Duration {
	lastRunRoundedMidnight := s.roundToMidnight(lastRun)

	if job.dayOfTheMonth > 0 { // calculate days to job.dayOfTheMonth
		jobDay := time.Date(lastRun.Year(), lastRun.Month(), job.dayOfTheMonth, 0, 0, 0, 0, s.Location()).Add(job.getAtTime())
		daysDifference := int(math.Abs(lastRun.Sub(jobDay).Hours()) / 24)
		nextRun := s.roundToMidnight(lastRun).Add(job.getAtTime())
		if jobDay.Before(lastRun) { // shouldn't run this month; schedule for next interval minus day difference
			nextRun = nextRun.AddDate(0, int(job.interval), -daysDifference)
		} else {
			if job.interval == 1 { // every month counts current month
				nextRun = nextRun.AddDate(0, int(job.interval)-1, daysDifference)
			} else { // should run next month interval
				nextRun = nextRun.AddDate(0, int(job.interval), daysDifference)
			}
		}
		return s.until(lastRun, nextRun)
	}
	nextRun := lastRunRoundedMidnight.Add(job.getAtTime()).AddDate(0, int(job.interval), 0)
	return s.until(lastRunRoundedMidnight, nextRun)
}

func (s *Scheduler) calculateWeekday(job *Job, lastRun time.Time) time.Duration {
	daysToWeekday := remainingDaysToWeekday(lastRun.Weekday(), *job.scheduledWeekday)
	totalDaysDifference := s.calculateTotalDaysDifference(lastRun, daysToWeekday, job)
	nextRun := s.roundToMidnight(lastRun).Add(job.getAtTime()).AddDate(0, 0, totalDaysDifference)
	return s.until(lastRun, nextRun)
}

func (s *Scheduler) calculateWeeks(job *Job, lastRun time.Time) time.Duration {
	totalDaysDifference := int(job.interval) * 7
	nextRun := s.roundToMidnight(lastRun).Add(job.getAtTime()).AddDate(0, 0, totalDaysDifference)
	return s.until(lastRun, nextRun)
}

func (s *Scheduler) calculateTotalDaysDifference(lastRun time.Time, daysToWeekday int, job *Job) int {
	if job.interval > 1 { // every N weeks counts rest of this week and full N-1 weeks
		return daysToWeekday + int(job.interval-1)*7
	}

	if daysToWeekday == 0 { // today, at future time or already passed
		lastRunAtTime := time.Date(lastRun.Year(), lastRun.Month(), lastRun.Day(), 0, 0, 0, 0, s.Location()).Add(job.getAtTime())
		if lastRun.Before(lastRunAtTime) || lastRun.Equal(lastRunAtTime) {
			return 0
		}
		return 7
	}

	return daysToWeekday
}

func (s *Scheduler) calculateDays(job *Job, lastRun time.Time) time.Duration {
	if job.interval == 1 {
		lastRunDayPlusJobAtTime := time.Date(lastRun.Year(), lastRun.Month(), lastRun.Day(), 0, 0, 0, 0, s.Location()).Add(job.getAtTime())
		if shouldRunToday(lastRun, lastRunDayPlusJobAtTime) {
			return s.until(lastRun, s.roundToMidnight(lastRun).Add(job.getAtTime()))
		}
	}

	nextRunAtTime := s.roundToMidnight(lastRun).Add(job.getAtTime()).AddDate(0, 0, int(job.interval)).In(s.Location())
	return s.until(lastRun, nextRunAtTime)
}

func (s *Scheduler) until(from time.Time, until time.Time) time.Duration {
	return until.Sub(from)
}

func shouldRunToday(lastRun time.Time, atTime time.Time) bool {
	return lastRun.Before(atTime)
}

func (s *Scheduler) calculateDuration(job *Job) time.Duration {
	lastRun := job.LastRun()
	if job.neverRan() && shouldRunAtSpecificTime(job) { // ugly. in order to avoid this we could prohibit setting .At() and allowing only .StartAt() when dealing with Duration types
		atTime := time.Date(lastRun.Year(), lastRun.Month(), lastRun.Day(), 0, 0, 0, 0, s.Location()).Add(job.getAtTime())
		if lastRun.Before(atTime) || lastRun.Equal(atTime) {
			return time.Until(s.roundToMidnight(lastRun).Add(job.getAtTime()))
		}
	}

	interval := job.interval
	switch job.unit {
	case milliseconds:
		return time.Duration(interval) * time.Millisecond
	case seconds:
		return time.Duration(interval) * time.Second
	case minutes:
		return time.Duration(interval) * time.Minute
	default:
		return time.Duration(interval) * time.Hour
	}
}

func shouldRunAtSpecificTime(job *Job) bool {
	return job.getAtTime() != 0
}

func remainingDaysToWeekday(from time.Weekday, to time.Weekday) int {
	daysUntilScheduledDay := int(to) - int(from)
	if daysUntilScheduledDay < 0 {
		daysUntilScheduledDay += 7
	}
	return daysUntilScheduledDay
}

// roundToMidnight truncates time to midnight
func (s *Scheduler) roundToMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, s.Location())
}

// NextRun datetime when the next Job should run.
func (s *Scheduler) NextRun() (*Job, time.Time) {
	if len(s.Jobs()) <= 0 {
		return nil, s.now()
	}

	sort.Sort(s)

	return s.Jobs()[0], s.Jobs()[0].NextRun()
}

// Every schedules a new periodic Job with an interval.
// Interval can be an int, time.Duration or a string that
// parses with time.ParseDuration().
// Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
func (s *Scheduler) Every(interval interface{}) *Scheduler {
	switch interval := interval.(type) {
	case int:
		job := NewJob(interval)
		if interval <= 0 {
			job.error = wrapOrError(job.error, ErrInvalidInterval)
		}
		s.setJobs(append(s.Jobs(), job))
	case time.Duration:
		job := NewJob(0)
		job.duration = interval
		job.unit = duration
		s.setJobs(append(s.Jobs(), job))
	case string:
		job := NewJob(0)
		d, err := time.ParseDuration(interval)
		if err != nil {
			job.error = wrapOrError(job.error, err)
		}
		job.duration = d
		job.unit = duration
		s.setJobs(append(s.Jobs(), job))
	default:
		job := NewJob(0)
		job.error = wrapOrError(job.error, ErrInvalidIntervalType)
		s.setJobs(append(s.Jobs(), job))
	}
	return s
}

func (s *Scheduler) run(job *Job) {
	if !s.IsRunning() {
		return
	}

	job.Lock()
	defer job.Unlock()
	job.setLastRun(s.now())
	job.runCount++
	s.executor.jobFunctions <- job.jobFunction
}

// RunAll run all Jobs regardless if they are scheduled to run or not
func (s *Scheduler) RunAll() {
	s.RunAllWithDelay(0)
}

// RunAllWithDelay runs all jobs with the provided delay in between each job
func (s *Scheduler) RunAllWithDelay(d time.Duration) {
	for _, job := range s.Jobs() {
		s.run(job)
		s.time.Sleep(d)
	}
}

// Remove specific Job job by function
//
// Removing a job stops that job's timer. However, if a job has already
// been started by by the job's timer before being removed, there is no way to stop
// it through gocron as https://pkg.go.dev/time#Timer.Stop explains.
// The job function would need to have implemented a means of
// stopping, e.g. using a context.WithCancel().
func (s *Scheduler) Remove(job interface{}) {
	s.removeByCondition(func(someJob *Job) bool {
		return someJob.name == getFunctionName(job)
	})
}

// RemoveByReference removes specific Job job by reference
func (s *Scheduler) RemoveByReference(job *Job) {
	s.removeByCondition(func(someJob *Job) bool {
		job.RLock()
		defer job.RUnlock()
		return someJob == job
	})
}

func (s *Scheduler) removeByCondition(shouldRemove func(*Job) bool) {
	retainedJobs := make([]*Job, 0)
	for _, job := range s.Jobs() {
		if !shouldRemove(job) {
			retainedJobs = append(retainedJobs, job)
		} else {
			job.stopTimer()
			job.cancel()
		}
	}
	s.setJobs(retainedJobs)
}

// RemoveByTag will remove a job by a given tag.
func (s *Scheduler) RemoveByTag(tag string) error {
	index, err := s.findJobsIndexByTag(tag)
	if err != nil {
		return err
	}
	// Remove job if job index is valid
	s.jobs[index].stopTimer()
	s.jobs[index].cancel()
	s.setJobs(removeAtIndex(s.jobs, index))
	return nil
}

// Find first job index by given string
func (s *Scheduler) findJobsIndexByTag(tag string) (int, error) {
	for i, job := range s.Jobs() {
		if strings.Contains(strings.Join(job.Tags(), " "), tag) {
			return i, nil
		}
	}
	return -1, ErrJobNotFoundWithTag
}

func removeAtIndex(jobs []*Job, i int) []*Job {
	if i == len(jobs)-1 {
		return jobs[:i]
	}
	jobs = append(jobs[:i], jobs[i+1:]...)
	return jobs
}

// LimitRunsTo limits the number of executions of this job to n.
// Upon reaching the limit, the job is removed from the scheduler.
func (s *Scheduler) LimitRunsTo(i int) *Scheduler {
	job := s.getCurrentJob()
	job.LimitRunsTo(i)
	return s
}

// SingletonMode prevents a new job from starting if the prior job has not yet
// completed it's run
func (s *Scheduler) SingletonMode() *Scheduler {
	job := s.getCurrentJob()
	job.SingletonMode()
	return s
}

// TaskPresent checks if specific job's function was added to the scheduler.
func (s *Scheduler) TaskPresent(j interface{}) bool {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	for _, job := range s.Jobs() {
		if job.name == getFunctionName(j) {
			return true
		}
	}
	return false
}

func (s *Scheduler) jobPresent(j *Job) bool {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	for _, job := range s.Jobs() {
		if job == j {
			return true
		}
	}
	return false
}

// Clear clear all Jobs from this scheduler
func (s *Scheduler) Clear() {
	for _, j := range s.Jobs() {
		j.stopTimer()
	}
	s.setJobs(make([]*Job, 0))
}

// Stop stops the scheduler. This is a no-op if the scheduler is already stopped .
func (s *Scheduler) Stop() {
	if s.IsRunning() {
		s.stop()
	}
}

func (s *Scheduler) stop() {
	s.setRunning(false)
	s.executor.stop <- struct{}{}
}

// Do specifies the jobFunc that should be called every time the Job runs
func (s *Scheduler) Do(jobFun interface{}, params ...interface{}) (*Job, error) {
	job := s.getCurrentJob()

	if job.atTime != 0 && job.unit <= hours {
		job.error = wrapOrError(job.error, ErrAtTimeNotSupported)
	}

	if job.scheduledWeekday != nil && job.unit != weeks {
		job.error = wrapOrError(job.error, ErrWeekdayNotSupported)
	}

	if job.error != nil {
		// delete the job from the scheduler as this job
		// cannot be executed
		s.RemoveByReference(job)
		return nil, job.error
	}

	typ := reflect.TypeOf(jobFun)
	if typ.Kind() != reflect.Func {
		// delete the job for the same reason as above
		s.RemoveByReference(job)
		return nil, ErrNotAFunction
	}

	fname := getFunctionName(jobFun)
	job.functions[fname] = jobFun
	job.params[fname] = params
	job.name = fname

	// we should not schedule if not running since we cant foresee how long it will take for the scheduler to start
	if s.IsRunning() {
		s.scheduleNextRun(job)
	}

	return job, nil
}

// At schedules the Job at a specific time of day in the form "HH:MM:SS" or "HH:MM"
// or time.Time (note that only the hours, minutes, seconds and nanos are used).
func (s *Scheduler) At(i interface{}) *Scheduler {
	job := s.getCurrentJob()

	switch t := i.(type) {
	case string:
		hour, min, sec, err := parseTime(t)
		if err != nil {
			job.error = wrapOrError(job.error, err)
			return s
		}
		// save atTime start as duration from midnight
		job.setAtTime(time.Duration(hour)*time.Hour + time.Duration(min)*time.Minute + time.Duration(sec)*time.Second)
	case time.Time:
		job.setAtTime(time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute + time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())*time.Nanosecond)
	default:
		job.error = wrapOrError(job.error, ErrUnsupportedTimeFormat)
	}
	job.startsImmediately = false
	return s
}

// Tag will add a tag when creating a job.
func (s *Scheduler) Tag(t ...string) *Scheduler {
	job := s.getCurrentJob()

	if s.tags != nil {
		for _, tag := range t {
			if _, ok := s.tags[tag]; ok {
				job.error = wrapOrError(job.error, ErrTagsUnique(tag))
				return s
			}
			s.tags[tag] = struct{}{}
		}
	}

	job.tags = t
	return s
}

// StartAt schedules the next run of the Job. If this time is in the past, the configured interval will be used
// to calculate the next future time
func (s *Scheduler) StartAt(t time.Time) *Scheduler {
	job := s.getCurrentJob()
	job.setStartAtTime(t)
	job.startsImmediately = false
	return s
}

// setUnit sets the unit type
func (s *Scheduler) setUnit(unit timeUnit) {
	job := s.getCurrentJob()
	if job.unit == duration {
		job.error = wrapOrError(job.error, ErrInvalidIntervalUnitsSelection)
	}
	job.unit = unit
}

// Second sets the unit with seconds
func (s *Scheduler) Millisecond() *Scheduler {
	return s.Milliseconds()
}

// Seconds sets the unit with seconds
func (s *Scheduler) Milliseconds() *Scheduler {
	s.setUnit(milliseconds)
	return s
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

// Month sets the unit with months
func (s *Scheduler) Month(dayOfTheMonth int) *Scheduler {
	return s.Months(dayOfTheMonth)
}

// Months sets the unit with months
func (s *Scheduler) Months(dayOfTheMonth int) *Scheduler {
	job := s.getCurrentJob()
	job.dayOfTheMonth = dayOfTheMonth
	job.startsImmediately = false
	s.setUnit(months)
	return s
}

// NOTE: If the dayOfTheMonth for the above two functions is
// more than the number of days in that month, the extra day(s)
// spill over to the next month. Similarly, if it's less than 0,
// it will go back to the month before

// Weekday sets the start with a specific weekday weekday
func (s *Scheduler) Weekday(startDay time.Weekday) *Scheduler {
	job := s.getCurrentJob()
	job.scheduledWeekday = &startDay
	job.startsImmediately = false
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
	return s.Jobs()[len(s.jobs)-1]
}

func (s *Scheduler) now() time.Time {
	return s.time.Now(s.Location())
}

// TagsUnique forces job tags to be unique across the scheduler
// when adding tags with (s *Scheduler) Tag().
// This does not enforce uniqueness on tags added via
// (j *Job) Tag()
func (s *Scheduler) TagsUnique() {
	s.tags = make(map[string]struct{})
}
