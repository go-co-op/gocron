package gocron

import (
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

// Scheduler struct stores a list of Jobs and the location of time Scheduler
// Scheduler implements the sort.Interface{} for sorting Jobs, by the time of nextRun
type Scheduler struct {
	jobsMutex sync.RWMutex
	jobs      []*Job

	locationMutex sync.RWMutex
	location      *time.Location

	runningMutex sync.RWMutex
	running      bool          // represents if the scheduler is running at the moment or not
	stopChan     chan struct{} // signal to stop scheduling

	time timeWrapper // wrapper around time.Time
}

// NewScheduler creates a new Scheduler
func NewScheduler(loc *time.Location) *Scheduler {
	return &Scheduler{
		jobs:     make([]*Job, 0),
		location: loc,
		running:  false,
		stopChan: make(chan struct{}, 1),
		time:     &trueTime{},
	}
}

// StartBlocking starts all the pending jobs using a second-long ticker and blocks the current thread
func (s *Scheduler) StartBlocking() {
	<-s.StartAsync()
}

// StartAsync starts a goroutine that runs all the pending using a second-long ticker
func (s *Scheduler) StartAsync() chan struct{} {
	if s.IsRunning() {
		return s.stopChan
	}
	s.start()
	return s.stopChan
}

//start runs each job and schedules it's next run
func (s *Scheduler) start() {
	s.setRunning(true)
	s.runJobs()
}

func (s *Scheduler) runJobs() {
	for _, j := range s.Jobs() {
		if j.getStartsImmediately() {
			s.run(j)
			j.setStartsImmediately(false)
		}
		if !j.shouldRun() {
			if j.getRemoveAfterLastRun() { // TODO: this method seems unnecessary as we could always remove after the run cout has expired. Maybe remove this in the future?
				s.RemoveByReference(j)
			}
			continue
		}
		s.scheduleNextRun(j)
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

// Swap
func (s *Scheduler) Swap(i, j int) {
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()
	s.jobs[i], s.jobs[j] = s.jobs[j], s.jobs[i]
}

func (s *Scheduler) Less(i, j int) bool {
	return s.Jobs()[j].NextRun().Unix() >= s.Jobs()[i].NextRun().Unix()
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
	lastRun := job.lastRun

	// job can be scheduled with .StartAt()
	if job.neverRan() {
		if !job.NextRun().IsZero() {
			return // scheduled for future run and should skip scheduling
		}
		lastRun = now
	}

	durationToNextRun := s.durationToNextRun(lastRun, job)
	job.setNextRun(lastRun.Add(durationToNextRun))
	job.timer = time.AfterFunc(durationToNextRun, func() {
		s.run(job)
		s.scheduleNextRun(job)
	})
}

func (s *Scheduler) durationToNextRun(t time.Time, job *Job) time.Duration {
	var duration time.Duration
	switch job.unit {
	case seconds, minutes, hours:
		duration = s.calculateDuration(job)
	case days:
		duration = s.calculateDays(job, t)
	case weeks:
		if job.scheduledWeekday != nil { // weekday selected, Every().Monday(), for example
			duration = s.calculateWeekday(job, t)
		} else {
			duration = s.calculateWeeks(job, t)
		}
	case months:
		duration = s.calculateMonths(job, t)
	}
	return duration
}

func (s *Scheduler) getJobLastRun(job *Job) time.Time {
	if job.neverRan() {
		return s.time.Now(s.Location())
	}
	return job.LastRun()
}

func (s *Scheduler) calculateMonths(job *Job, lastRun time.Time) time.Duration {
	lastRunRoundedMidnight := s.roundToMidnight(lastRun)

	if job.dayOfTheMonth > 0 { // calculate days to j.dayOfTheMonth
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
		return s.until(lastRunRoundedMidnight, nextRun)
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
		return nil, s.time.Now(s.Location())
	}

	sort.Sort(s)

	return s.Jobs()[0], s.Jobs()[0].NextRun()
}

// Every schedules a new periodic Job with interval
func (s *Scheduler) Every(interval uint64) *Scheduler {
	job := NewJob(interval)
	s.setJobs(append(s.Jobs(), job))
	return s
}

func (s *Scheduler) run(job *Job) {
	if !s.running {
		return
	}
	job.setLastRun(s.now())
	job.run()
}

// RunAll run all Jobs regardless if they are scheduled to run or not
func (s *Scheduler) RunAll() {
	s.RunAllWithDelay(0)
}

// RunAllWithDelay runs all Jobs with delay seconds
func (s *Scheduler) RunAllWithDelay(d int) {
	for _, job := range s.Jobs() {
		s.run(job)
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
	retainedJobs := make([]*Job, 0)
	for _, job := range s.Jobs() {
		if !shouldRemove(job) {
			retainedJobs = append(retainedJobs, job)
		}
	}
	s.setJobs(retainedJobs)
}

// RemoveJobByTag will Remove Jobs by Tag
func (s *Scheduler) RemoveJobByTag(tag string) error {
	jobindex, err := s.findJobsIndexByTag(tag)
	if err != nil {
		return err
	}
	// Remove job if jobindex is valid
	s.setJobs(removeAtIndex(s.jobs, jobindex))
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

// Scheduled checks if specific Job j was already added
func (s *Scheduler) Scheduled(j interface{}) bool {
	for _, job := range s.Jobs() {
		if job.jobFunc == getFunctionName(j) {
			return true
		}
	}
	return false
}

// Clear clear all Jobs from this scheduler
func (s *Scheduler) Clear() {
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
	s.stopChan <- struct{}{}
}

// Do specifies the jobFunc that should be called every time the Job runs
func (s *Scheduler) Do(jobFun interface{}, params ...interface{}) (*Job, error) {
	j := s.getCurrentJob()
	if j.err != nil {
		// delete the job from the scheduler as this job
		// cannot be executed
		s.RemoveByReference(j)
		return nil, j.err
	}

	typ := reflect.TypeOf(jobFun)
	if typ.Kind() != reflect.Func {
		// delete the job for the same reason as above
		s.RemoveByReference(j)
		return nil, ErrNotAFunction
	}

	fname := getFunctionName(jobFun)
	j.funcs[fname] = jobFun
	j.fparams[fname] = params
	j.jobFunc = fname

	// we should not schedule if not running since we cant foresee how long it will take for the scheduler to start
	if s.IsRunning() {
		s.scheduleNextRun(j)
	}

	return j, nil
}

// At schedules the Job at a specific time of day in the form "HH:MM:SS" or "HH:MM"
func (s *Scheduler) At(t string) *Scheduler {
	j := s.getCurrentJob()
	hour, min, sec, err := parseTime(t)
	if err != nil {
		j.err = ErrTimeFormat
		return s
	}
	// save atTime start as duration from midnight
	j.setAtTime(time.Duration(hour)*time.Hour + time.Duration(min)*time.Minute + time.Duration(sec)*time.Second)
	j.startsImmediately = false
	return s
}

// SetTag will add tag when creating a job
func (s *Scheduler) SetTag(t []string) *Scheduler {
	job := s.getCurrentJob()
	job.tags = t
	return s
}

// StartAt schedules the next run of the Job
func (s *Scheduler) StartAt(t time.Time) *Scheduler {
	job := s.getCurrentJob()
	job.setNextRun(t)
	job.startsImmediately = false
	return s
}

// shouldRun returns true if the Job should be run now
func (s *Scheduler) shouldRun(j *Job) bool {

	// option remove the job's in the scheduler after its last execution
	if j.getRemoveAfterLastRun() && (j.getMaxRuns()-j.RunCount()) == 1 {
		s.RemoveByReference(j)
	}

	return j.shouldRun() && s.time.Now(s.Location()).Unix() >= j.NextRun().Unix()
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
