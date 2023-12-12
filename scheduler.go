package gocron

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"go.uber.org/atomic"
)

type limitMode int8

// Scheduler struct stores a list of Jobs and the location of time used by the Scheduler
type Scheduler struct {
	jobsMutex sync.RWMutex
	jobs      map[uuid.UUID]*Job

	locationMutex sync.RWMutex
	location      *time.Location
	running       *atomic.Bool // represents if the scheduler is running at the moment or not

	time     TimeWrapper // wrapper around time.Time
	timer    func(d time.Duration, f func()) *time.Timer
	executor *executor // executes jobs passed via chan

	tags sync.Map // for storing tags when unique tags is set

	tagsUnique      bool // defines whether tags should be unique
	updateJob       bool // so the scheduler knows to create a new job or update the current
	waitForInterval bool // defaults jobs to waiting for first interval to start
	singletonMode   bool // defaults all jobs to use SingletonMode()

	startBlockingStopChanMutex sync.Mutex
	startBlockingStopChan      chan struct{} // stops the scheduler

	// tracks whether we're in a chain of scheduling methods for a job
	// a chain is started with any of the scheduler methods that operate
	// upon a job and are ended with one of [ Do(), Update() ] - note that
	// Update() calls Do(), so really they all end with Do().
	// This allows the caller to begin with any job related scheduler method
	// and only with one of [ Every(), EveryRandom(), Cron(), CronWithSeconds(), MonthFirstWeekday() ]
	inScheduleChain *uuid.UUID
}

// days in a week
const allWeekDays = 7

// NewScheduler creates a new Scheduler
func NewScheduler(loc *time.Location) *Scheduler {
	executor := newExecutor()

	s := &Scheduler{
		location:   loc,
		running:    atomic.NewBool(false),
		time:       &trueTime{},
		executor:   &executor,
		tagsUnique: false,
		timer:      afterFunc,
	}
	s.jobsMutex.Lock()
	s.jobs = map[uuid.UUID]*Job{}
	s.jobsMutex.Unlock()
	return s
}

// SetMaxConcurrentJobs limits how many jobs can be running at the same time.
// This is useful when running resource intensive jobs and a precise start time is not critical.
//
// Note: WaitMode and RescheduleMode provide details on usage and potential risks.
func (s *Scheduler) SetMaxConcurrentJobs(n int, mode limitMode) {
	s.executor.limitModeMaxRunningJobs = n
	s.executor.limitMode = mode
}

// StartBlocking starts all jobs and blocks the current thread.
// This blocking method can be stopped with Stop() from a separate goroutine.
func (s *Scheduler) StartBlocking() {
	s.StartAsync()
	s.startBlockingStopChanMutex.Lock()
	s.startBlockingStopChan = make(chan struct{}, 1)
	s.startBlockingStopChanMutex.Unlock()

	<-s.startBlockingStopChan

	s.startBlockingStopChanMutex.Lock()
	s.startBlockingStopChan = nil
	s.startBlockingStopChanMutex.Unlock()
}

// StartAsync starts all jobs without blocking the current thread
func (s *Scheduler) StartAsync() {
	if !s.IsRunning() {
		s.start()
	}
}

// start starts the scheduler, scheduling and running jobs
func (s *Scheduler) start() {
	s.executor.start()
	s.setRunning(true)
	s.runJobs()
}

func (s *Scheduler) runJobs() {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	for _, job := range s.jobs {
		ctx, cancel := context.WithCancel(context.Background())
		job.mu.Lock()
		job.ctx = ctx
		job.cancel = cancel
		job.mu.Unlock()
		s.runContinuous(job)
	}
}

func (s *Scheduler) setRunning(b bool) {
	s.running.Store(b)
}

// IsRunning returns true if the scheduler is running
func (s *Scheduler) IsRunning() bool {
	return s.running.Load()
}

// Jobs returns the list of Jobs from the scheduler
func (s *Scheduler) Jobs() []*Job {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	jobs := make([]*Job, len(s.jobs))
	var counter int
	for _, job := range s.jobs {
		jobs[counter] = job
		counter++
	}
	return jobs
}

// JobsMap returns a map of job uuid to job
func (s *Scheduler) JobsMap() map[uuid.UUID]*Job {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	jobs := make(map[uuid.UUID]*Job, len(s.jobs))
	for id, job := range s.jobs {
		jobs[id] = job
	}
	return jobs
}

// Name sets the name of the current job.
//
// If the scheduler is running using WithDistributedLocker(), the job name is used
// as the distributed lock key. If the job name is not set, the function name is used as the distributed lock key.
func (s *Scheduler) Name(name string) *Scheduler {
	job := s.getCurrentJob()
	job.jobName = name
	return s
}

// Len returns the number of Jobs in the Scheduler
func (s *Scheduler) Len() int {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	return len(s.jobs)
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

type nextRun struct {
	duration time.Duration
	dateTime time.Time
}

// scheduleNextRun Compute the instant when this Job should run next
func (s *Scheduler) scheduleNextRun(job *Job) (bool, nextRun) {
	now := s.now()
	if !s.jobPresent(job) {
		return false, nextRun{}
	}

	lastRun := now

	if job.neverRan() {
		// Increment startAtTime to the future
		if !job.startAtTime.IsZero() && job.startAtTime.Before(now) {
			dur := s.durationToNextRun(job.startAtTime, job).duration
			job.setStartAtTime(job.startAtTime.Add(dur))
			if job.startAtTime.Before(now) {
				diff := now.Sub(job.startAtTime)
				dur := s.durationToNextRun(job.startAtTime, job).duration
				var count time.Duration
				if dur != 0 {
					count = diff / dur
					if diff%dur != 0 {
						count++
					}
				}
				job.setStartAtTime(job.startAtTime.Add(dur * count))
			}
		}
	} else {
		lastRun = job.NextRun()
	}

	if !job.shouldRun() {
		_ = s.RemoveByID(job)
		return false, nextRun{}
	}

	next := s.durationToNextRun(lastRun, job)

	jobNextRun := job.NextRun()
	if jobNextRun.After(now) {
		job.setLastRun(now)
	} else {
		job.setLastRun(jobNextRun)
	}

	if next.dateTime.IsZero() {
		next.dateTime = lastRun.Add(next.duration)
		job.setNextRun(next.dateTime)
	} else {
		job.setNextRun(next.dateTime)
	}
	return true, next
}

// durationToNextRun calculate how much time to the next run, depending on unit
func (s *Scheduler) durationToNextRun(lastRun time.Time, job *Job) nextRun {
	// job can be scheduled with .StartAt()
	if job.getFirstAtTime() == 0 && job.getStartAtTime().After(lastRun) {
		sa := job.getStartAtTime()
		if job.unit == days || job.unit == weeks || job.unit == months {
			job.addAtTime(
				time.Duration(sa.Hour())*time.Hour +
					time.Duration(sa.Minute())*time.Minute +
					time.Duration(sa.Second())*time.Second,
			)
		}
		return nextRun{duration: sa.Sub(s.now()), dateTime: sa}
	}

	var next nextRun
	switch job.getUnit() {
	case milliseconds, seconds, minutes, hours:
		next.duration = s.calculateDuration(job)
	case days:
		next = s.calculateDays(job, lastRun)
	case weeks:
		if len(job.scheduledWeekdays) != 0 { // weekday selected, Every().Monday(), for example
			next = s.calculateWeekday(job, lastRun)
		} else {
			next = s.calculateWeeks(job, lastRun)
		}
		if next.dateTime.Before(job.getStartAtTime()) {
			return s.durationToNextRun(job.getStartAtTime(), job)
		}
	case months:
		next = s.calculateMonths(job, lastRun)
	case duration:
		next.duration = job.getDuration()
	case crontab:
		next.dateTime = job.cronSchedule.Next(lastRun)
		next.duration = next.dateTime.Sub(lastRun)
	}
	return next
}

func (s *Scheduler) calculateMonths(job *Job, lastRun time.Time) nextRun {
	// Special case: negative days from the end of the month
	if len(job.daysOfTheMonth) == 1 && job.daysOfTheMonth[0] < 0 {
		return calculateNextRunForLastDayOfMonth(s, job, lastRun, job.daysOfTheMonth[0])
	}

	if len(job.daysOfTheMonth) != 0 { // calculate days to job.daysOfTheMonth
		nextRunDateMap := make(map[int]nextRun)
		for _, day := range job.daysOfTheMonth {
			nextRunDateMap[day] = calculateNextRunForMonth(s, job, lastRun, day)
		}

		nextRunResult := nextRun{}
		for _, val := range nextRunDateMap {
			if nextRunResult.dateTime.IsZero() {
				nextRunResult = val
			} else if nextRunResult.dateTime.Sub(val.dateTime).Milliseconds() > 0 {
				nextRunResult = val
			}
		}

		return nextRunResult
	}
	next := s.roundToMidnightAndAddDSTAware(lastRun, job.getFirstAtTime()).AddDate(0, job.getInterval(), 0)
	return nextRun{duration: until(lastRun, next), dateTime: next}
}

func calculateNextRunForLastDayOfMonth(s *Scheduler, job *Job, lastRun time.Time, dayBeforeLastOfMonth int) nextRun {
	// Calculate the last day of the next month, by adding job.interval+1 months (i.e. the
	// first day of the month after the next month), and subtracting one day, unless the
	// last run occurred before the end of the month.
	addMonth := job.getInterval()
	atTime := job.getAtTime(lastRun)
	if testDate := lastRun.AddDate(0, 0, -dayBeforeLastOfMonth); testDate.Month() != lastRun.Month() &&
		!s.roundToMidnightAndAddDSTAware(lastRun, atTime).After(lastRun) {
		// Our last run was on the last day of this month.
		addMonth++
		atTime = job.getFirstAtTime()
	}

	next := time.Date(lastRun.Year(), lastRun.Month(), 1, 0, 0, 0, 0, s.Location()).
		Add(atTime).
		AddDate(0, addMonth, 0).
		AddDate(0, 0, dayBeforeLastOfMonth)
	return nextRun{duration: until(lastRun, next), dateTime: next}
}

func calculateNextRunForMonth(s *Scheduler, job *Job, lastRun time.Time, dayOfMonth int) nextRun {
	atTime := job.getAtTime(lastRun)
	natTime := atTime

	hours, minutes, seconds := s.deconstructDuration(atTime)
	jobDay := time.Date(lastRun.Year(), lastRun.Month(), dayOfMonth, hours, minutes, seconds, 0, s.Location())

	difference := absDuration(lastRun.Sub(jobDay))
	next := lastRun
	if jobDay.Before(lastRun) { // shouldn't run this month; schedule for next interval minus day difference
		next = next.AddDate(0, job.getInterval(), -0)
		next = next.Add(-difference)
		natTime = job.getFirstAtTime()
	} else {
		if job.getInterval() == 1 && !jobDay.Equal(lastRun) { // every month counts current month
			next = next.AddDate(0, job.getInterval()-1, 0)
		} else { // should run next month interval
			next = next.AddDate(0, job.getInterval(), 0)
			natTime = job.getFirstAtTime()
		}
		next = next.Add(difference)
	}
	if atTime != natTime {
		next = next.Add(-atTime).Add(natTime)
	}
	return nextRun{duration: until(lastRun, next), dateTime: next}
}

func (s *Scheduler) calculateWeekday(job *Job, lastRun time.Time) nextRun {
	daysToWeekday := s.remainingDaysToWeekday(lastRun, job)
	totalDaysDifference := s.calculateTotalDaysDifference(lastRun, daysToWeekday, job)
	acTime := job.getAtTime(lastRun)
	if totalDaysDifference > 0 {
		acTime = job.getFirstAtTime()
	}
	next := s.roundToMidnightAndAddDSTAware(lastRun, acTime).AddDate(0, 0, totalDaysDifference)
	return nextRun{duration: until(lastRun, next), dateTime: next}
}

func (s *Scheduler) calculateWeeks(job *Job, lastRun time.Time) nextRun {
	totalDaysDifference := int(job.getInterval()) * 7

	var next time.Time

	atTimes := job.atTimes
	for _, at := range atTimes {
		n := s.roundToMidnightAndAddDSTAware(lastRun, at)
		if n.After(s.now()) {
			next = n
			break
		}
	}

	if next.IsZero() {
		next = s.roundToMidnightAndAddDSTAware(lastRun, job.getFirstAtTime()).AddDate(0, 0, totalDaysDifference)
	}

	return nextRun{duration: until(lastRun, next), dateTime: next}
}

func (s *Scheduler) calculateTotalDaysDifference(lastRun time.Time, daysToWeekday int, job *Job) int {
	if job.getInterval() > 1 {
		weekDays := job.Weekdays()
		if job.lastRun.Weekday() != weekDays[len(weekDays)-1] {
			return daysToWeekday
		}
		if daysToWeekday > 0 {
			return int(job.getInterval())*7 - (allWeekDays - daysToWeekday)
		}
		return int(job.getInterval()) * 7
	}

	if daysToWeekday == 0 { // today, at future time or already passed
		lastRunAtTime := time.Date(lastRun.Year(), lastRun.Month(), lastRun.Day(), 0, 0, 0, 0, s.Location()).Add(job.getAtTime(lastRun))
		if lastRun.Before(lastRunAtTime) {
			return 0
		}
		return 7
	}
	return daysToWeekday
}

func (s *Scheduler) calculateDays(job *Job, lastRun time.Time) nextRun {
	nextRunAtTime := s.roundToMidnightAndAddDSTAware(lastRun, job.getAtTime(lastRun)).In(s.Location())
	if s.now().After(nextRunAtTime) || s.now() == nextRunAtTime {
		nextRunAtTime = nextRunAtTime.AddDate(0, 0, job.getInterval())
	}
	return nextRun{duration: until(lastRun, nextRunAtTime), dateTime: nextRunAtTime}
}

func until(from time.Time, until time.Time) time.Duration {
	return until.Sub(from)
}

func in(scheduleWeekdays []time.Weekday, weekday time.Weekday) bool {
	in := false

	for _, weekdayInSchedule := range scheduleWeekdays {
		if int(weekdayInSchedule) == int(weekday) {
			in = true
			break
		}
	}
	return in
}

func (s *Scheduler) calculateDuration(job *Job) time.Duration {
	interval := job.getInterval()
	switch job.getUnit() {
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

func (s *Scheduler) remainingDaysToWeekday(lastRun time.Time, job *Job) int {
	weekDays := job.Weekdays()
	sort.Slice(weekDays, func(i, j int) bool {
		return weekDays[i] < weekDays[j]
	})

	equals := false
	lastRunWeekday := lastRun.Weekday()
	index := sort.Search(len(weekDays), func(i int) bool {
		b := weekDays[i] >= lastRunWeekday
		if b {
			equals = weekDays[i] == lastRunWeekday
		}
		return b
	})
	// check atTime
	if equals {
		if s.roundToMidnightAndAddDSTAware(lastRun, job.getAtTime(lastRun)).After(lastRun) {
			return 0
		}
		index++
	}

	if index < len(weekDays) {
		return int(weekDays[index] - lastRunWeekday)
	}

	return int(weekDays[0]) + allWeekDays - int(lastRunWeekday)
}

// absDuration returns the abs time difference
func absDuration(a time.Duration) time.Duration {
	if a >= 0 {
		return a
	}
	return -a
}

func (s *Scheduler) deconstructDuration(d time.Duration) (hours int, minutes int, seconds int) {
	hours = int(d.Seconds()) / int(time.Hour/time.Second)
	minutes = (int(d.Seconds()) % int(time.Hour/time.Second)) / int(time.Minute/time.Second)
	seconds = int(d.Seconds()) % int(time.Minute/time.Second)
	return
}

// roundToMidnightAndAddDSTAware truncates time to midnight and "adds" duration in a DST aware manner
func (s *Scheduler) roundToMidnightAndAddDSTAware(t time.Time, d time.Duration) time.Time {
	hours, minutes, seconds := s.deconstructDuration(d)
	return time.Date(t.Year(), t.Month(), t.Day(), hours, minutes, seconds, 0, s.Location())
}

// NextRun datetime when the next Job should run.
func (s *Scheduler) NextRun() (*Job, time.Time) {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	if len(s.jobs) <= 0 {
		return nil, time.Time{}
	}

	var jobID uuid.UUID
	var nearestRun time.Time
	for _, job := range s.jobs {
		nr := job.NextRun()
		if (nr.Before(nearestRun) || nearestRun.IsZero()) && s.now().Before(nr) {
			nearestRun = nr
			jobID = job.id
		}
	}

	return s.jobs[jobID], nearestRun
}

// EveryRandom schedules a new period Job that runs at random intervals
// between the provided lower (inclusive) and upper (inclusive) bounds.
// The default unit is Seconds(). Call a different unit in the chain
// if you would like to change that. For example, Minutes(), Hours(), etc.
func (s *Scheduler) EveryRandom(lower, upper int) *Scheduler {
	job := s.getCurrentJob()

	job.setRandomInterval(lower, upper)
	return s
}

// Every schedules a new periodic Job with an interval.
// Interval can be an int, time.Duration or a string that
// parses with time.ParseDuration().
// Negative intervals will return an error.
// Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
//
// The job is run immediately, unless:
// * StartAt or At is set on the job,
// * WaitForSchedule is set on the job,
// * or WaitForScheduleAll is set on the scheduler.
func (s *Scheduler) Every(interval interface{}) *Scheduler {
	job := s.getCurrentJob()

	switch interval := interval.(type) {
	case int:
		job.interval = interval
		if interval <= 0 {
			job.error = wrapOrError(job.error, ErrInvalidInterval)
		}
	case time.Duration:
		if interval <= 0 {
			job.error = wrapOrError(job.error, ErrInvalidInterval)
		}
		job.setInterval(0)
		job.setDuration(interval)
		job.setUnit(duration)
	case string:
		d, err := time.ParseDuration(interval)
		if err != nil {
			job.error = wrapOrError(job.error, err)
		}
		if d <= 0 {
			job.error = wrapOrError(job.error, ErrInvalidInterval)
		}
		job.setDuration(d)
		job.setUnit(duration)
	default:
		job.error = wrapOrError(job.error, ErrInvalidIntervalType)
	}

	return s
}

func (s *Scheduler) run(job *Job) {
	if !s.IsRunning() {
		return
	}

	job.mu.Lock()

	if job.function == nil {
		job.mu.Unlock()
		s.Remove(job)
		return
	}

	defer job.mu.Unlock()

	if job.runWithDetails {
		switch len(job.parameters) {
		case job.parametersLen:
			job.parameters = append(job.parameters, job.copy())
		case job.parametersLen + 1:
			job.parameters[job.parametersLen] = job.copy()
		default:
			// something is really wrong and we should never get here
			job.error = wrapOrError(job.error, ErrInvalidFunctionParameters)
			return
		}
	}

	s.executor.jobFunctions <- job.jobFunction.copy()
}

func (s *Scheduler) runContinuous(job *Job) {
	shouldRun, next := s.scheduleNextRun(job)
	if !shouldRun {
		return
	}

	if !job.getStartsImmediately() {
		job.setStartsImmediately(true)
	} else {
		s.run(job)
	}
	nr := next.dateTime.Sub(s.now())
	if nr < 0 {
		job.setLastRun(s.now())
		shouldRun, next := s.scheduleNextRun(job)
		if !shouldRun {
			return
		}
		nr = next.dateTime.Sub(s.now())
	}

	job.setTimer(s.timer(nr, func() {
		if !next.dateTime.IsZero() {
			for {
				n := s.now().UnixNano() - next.dateTime.UnixNano()
				if n >= 0 {
					break
				}
				select {
				case <-s.executor.ctx.Done():
				case <-time.After(time.Duration(n)):
				}
			}
		}
		s.runContinuous(job)
	}))
}

// RunAll run all Jobs regardless if they are scheduled to run or not
func (s *Scheduler) RunAll() {
	s.RunAllWithDelay(0)
}

// RunAllWithDelay runs all Jobs with the provided delay in between each Job
func (s *Scheduler) RunAllWithDelay(d time.Duration) {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	for _, job := range s.jobs {
		s.run(job)
		s.time.Sleep(d)
	}
}

// RunByTag runs all the Jobs containing a specific tag
// regardless of whether they are scheduled to run or not
func (s *Scheduler) RunByTag(tag string) error {
	return s.RunByTagWithDelay(tag, 0)
}

// RunByTagWithDelay is same as RunByTag but introduces a delay between
// each Job execution
func (s *Scheduler) RunByTagWithDelay(tag string, d time.Duration) error {
	jobs, err := s.FindJobsByTag(tag)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		s.run(job)
		s.time.Sleep(d)
	}
	return nil
}

// Remove specific Job by function
//
// Removing a job stops that job's timer. However, if a job has already
// been started by the job's timer before being removed, the only way to stop
// it through gocron is to use DoWithJobDetails and access the job's Context which
// informs you when the job has been canceled.
//
// Alternatively, the job function would need to have implemented a means of
// stopping, e.g. using a context.WithCancel() passed as params to Do method.
//
// The above are based on what the underlying library suggests https://pkg.go.dev/time#Timer.Stop.
func (s *Scheduler) Remove(job interface{}) {
	fName := getFunctionName(job)
	j := s.findJobByTaskName(fName)
	s.removeJobsUniqueTags(j)
	s.removeByCondition(func(someJob *Job) bool {
		return someJob.funcName == fName
	})
}

// RemoveByReference removes specific Job by reference
func (s *Scheduler) RemoveByReference(job *Job) {
	_ = s.RemoveByID(job)
}

func (s *Scheduler) findJobByTaskName(name string) *Job {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	for _, job := range s.jobs {
		if job.funcName == name {
			return job
		}
	}
	return nil
}

func (s *Scheduler) removeJobsUniqueTags(job *Job) {
	if job == nil {
		return
	}
	if s.tagsUnique && len(job.tags) > 0 {
		for _, tag := range job.tags {
			s.tags.Delete(tag)
		}
	}
}

func (s *Scheduler) removeByCondition(shouldRemove func(*Job) bool) {
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()
	for _, job := range s.jobs {
		if shouldRemove(job) {
			s.stopJob(job)
			delete(s.jobs, job.id)
		}
	}
}

func (s *Scheduler) stopJob(job *Job) {
	job.mu.Lock()
	if job.runConfig.mode == singletonMode {
		s.executor.singletonWgs.Delete(job.singletonWg)
	}
	job.mu.Unlock()
	job.stop()
}

// RemoveByTag will remove jobs that match the given tag.
func (s *Scheduler) RemoveByTag(tag string) error {
	return s.RemoveByTags(tag)
}

// RemoveByTags will remove jobs that match all given tags.
func (s *Scheduler) RemoveByTags(tags ...string) error {
	jobs, err := s.FindJobsByTag(tags...)
	if err != nil {
		return err
	}

	for _, job := range jobs {
		_ = s.RemoveByID(job)
	}
	return nil
}

// RemoveByTagsAny will remove jobs that match any one of the given tags.
func (s *Scheduler) RemoveByTagsAny(tags ...string) error {
	var errs error
	mJob := make(map[*Job]struct{})
	for _, tag := range tags {
		jobs, err := s.FindJobsByTag(tag)
		if err != nil {
			errs = wrapOrError(errs, fmt.Errorf("%s: %s", err.Error(), tag))
		}
		for _, job := range jobs {
			mJob[job] = struct{}{}
		}
	}

	for job := range mJob {
		_ = s.RemoveByID(job)
	}

	return errs
}

// RemoveByID removes the job from the scheduler looking up by id
func (s *Scheduler) RemoveByID(job *Job) error {
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()
	if _, ok := s.jobs[job.id]; ok {
		s.removeJobsUniqueTags(job)
		s.stopJob(job)
		delete(s.jobs, job.id)
		return nil
	}
	return ErrJobNotFound
}

// FindJobsByTag will return a slice of jobs that match all given tags
func (s *Scheduler) FindJobsByTag(tags ...string) ([]*Job, error) {
	var jobs []*Job

	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
Jobs:
	for _, job := range s.jobs {
		if job.hasTags(tags...) {
			jobs = append(jobs, job)
			continue Jobs
		}
	}

	if len(jobs) > 0 {
		return jobs, nil
	}
	return nil, ErrJobNotFoundWithTag
}

// MonthFirstWeekday sets the job to run the first specified weekday of the month
func (s *Scheduler) MonthFirstWeekday(weekday time.Weekday) *Scheduler {
	_, month, day := s.time.Now(time.UTC).Date()

	if day < 7 {
		return s.Cron(fmt.Sprintf("0 0 %d %d %d", day, month, weekday))
	}

	return s.Cron(fmt.Sprintf("0 0 %d %d %d", day, (month+1)%12, weekday))
}

// LimitRunsTo limits the number of executions of this job to n.
// Upon reaching the limit, the job is removed from the scheduler.
func (s *Scheduler) LimitRunsTo(i int) *Scheduler {
	job := s.getCurrentJob()
	job.LimitRunsTo(i)
	return s
}

// SingletonMode prevents a new job from starting if the prior job has not yet
// completed its run
//
// Warning: do not use this mode if your jobs will continue to stack
// up beyond the ability of the limit workers to keep up. An example of
// what NOT to do:
//
//	 s.Every("1s").SingletonMode().Do(func() {
//	     // this will result in an ever-growing number of goroutines
//		   // blocked trying to send to the buffered channel
//	     time.Sleep(10 * time.Minute)
//	 })
func (s *Scheduler) SingletonMode() *Scheduler {
	job := s.getCurrentJob()
	job.SingletonMode()
	return s
}

// SingletonModeAll prevents new jobs from starting if the prior instance of the
// particular job has not yet completed its run
//
// Warning: do not use this mode if your jobs will continue to stack
// up beyond the ability of the limit workers to keep up. An example of
// what NOT to do:
//
//	 s := gocron.NewScheduler(time.UTC)
//	 s.SingletonModeAll()
//
//	 s.Every("1s").Do(func() {
//	     // this will result in an ever-growing number of goroutines
//		   // blocked trying to send to the buffered channel
//	     time.Sleep(10 * time.Minute)
//	 })
func (s *Scheduler) SingletonModeAll() {
	s.singletonMode = true
}

// TaskPresent checks if specific job's function was added to the scheduler.
func (s *Scheduler) TaskPresent(j interface{}) bool {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	for _, job := range s.jobs {
		if job.funcName == getFunctionName(j) {
			return true
		}
	}
	return false
}

func (s *Scheduler) jobPresent(j *Job) bool {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	if _, ok := s.jobs[j.id]; ok {
		return true
	}
	return false
}

// Clear clears all Jobs from this scheduler
func (s *Scheduler) Clear() {
	s.stopJobs()
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()
	s.jobs = make(map[uuid.UUID]*Job)
	// If unique tags was enabled, delete all the tags loaded in the tags sync.Map
	if s.tagsUnique {
		s.tags.Range(func(key interface{}, value interface{}) bool {
			s.tags.Delete(key)
			return true
		})
	}
}

// Stop stops the scheduler. This is a no-op if the scheduler is already stopped.
// It waits for all running jobs to finish before returning, so it is safe to assume that running jobs will finish when calling this.
func (s *Scheduler) Stop() {
	if s.IsRunning() {
		s.stop()
	}
}

func (s *Scheduler) stop() {
	s.stopJobs()
	s.executor.stop()
	s.StopBlockingChan()
	s.setRunning(false)
}

func (s *Scheduler) stopJobs() {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	for _, job := range s.jobs {
		job.stop()
	}
}

func (s *Scheduler) doCommon(jobFun interface{}, params ...interface{}) (*Job, error) {
	job := s.getCurrentJob()
	s.inScheduleChain = nil

	jobUnit := job.getUnit()
	jobLastRun := job.LastRun()
	if job.getAtTime(jobLastRun) != 0 && (jobUnit <= hours || jobUnit >= duration) {
		job.error = wrapOrError(job.error, ErrAtTimeNotSupported)
	}

	if len(job.scheduledWeekdays) != 0 && jobUnit != weeks {
		job.error = wrapOrError(job.error, ErrWeekdayNotSupported)
	}

	if job.unit != crontab && job.getInterval() == 0 {
		if job.unit != duration {
			job.error = wrapOrError(job.error, ErrInvalidInterval)
		}
	}

	if job.error != nil {
		// delete the job from the scheduler as this job
		// cannot be executed
		_ = s.RemoveByID(job)
		return nil, job.error
	}

	val := reflect.ValueOf(jobFun)
	for val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Func {
		// delete the job for the same reason as above
		_ = s.RemoveByID(job)
		return nil, ErrNotAFunction
	}

	var fname string
	if val == reflect.ValueOf(jobFun) {
		fname = getFunctionName(jobFun)
	} else {
		fname = getFunctionNameOfPointer(jobFun)
	}

	if job.funcName != fname {
		job.function = jobFun
		if val != reflect.ValueOf(jobFun) {
			job.function = val.Interface()
		}

		job.parameters = params
		job.funcName = fname
	}

	expectedParamLength := val.Type().NumIn()
	if job.runWithDetails {
		expectedParamLength--
	}

	if len(params) != expectedParamLength {
		_ = s.RemoveByID(job)
		job.error = wrapOrError(job.error, ErrWrongParams)
		return nil, job.error
	}

	if job.runWithDetails && val.Type().In(len(params)).Kind() != reflect.ValueOf(*job).Kind() {
		_ = s.RemoveByID(job)
		job.error = wrapOrError(job.error, ErrDoWithJobDetails)
		return nil, job.error
	}

	// we should not schedule if not running since we can't foresee how long it will take for the scheduler to start
	if s.IsRunning() {
		s.runContinuous(job)
	}

	return job, nil
}

// Do specifies the jobFunc that should be called every time the Job runs
func (s *Scheduler) Do(jobFun interface{}, params ...interface{}) (*Job, error) {
	return s.doCommon(jobFun, params...)
}

// DoWithJobDetails specifies the jobFunc that should be called every time the Job runs
// and additionally passes the details of the current job to the jobFunc.
// The last argument of the function must be a gocron.Job that will be passed by
// the scheduler when the function is called.
func (s *Scheduler) DoWithJobDetails(jobFun interface{}, params ...interface{}) (*Job, error) {
	job := s.getCurrentJob()
	job.runWithDetails = true
	job.parametersLen = len(params)
	return s.doCommon(jobFun, params...)
}

// At schedules the Job at a specific time of day in the form "HH:MM:SS" or "HH:MM"
// or time.Time (note that only the hours, minutes, seconds and nanos are used).
// When the At time(s) occur on the same day on which the scheduler is started
// the Job will be run at the first available At time.
// For example: a schedule for every 2 days at 9am and 11am
// - currently 7am -> Job runs at 9am and 11am on the day the scheduler was started
// - currently 12 noon -> Job runs at 9am and 11am two days after the scheduler started
func (s *Scheduler) At(i interface{}) *Scheduler {
	job := s.getCurrentJob()

	switch t := i.(type) {
	case string:
		for _, tt := range strings.Split(t, ";") {
			hour, min, sec, err := parseTime(tt)
			if err != nil {
				job.error = wrapOrError(job.error, err)
				return s
			}
			// save atTime start as duration from midnight
			job.addAtTime(time.Duration(hour)*time.Hour + time.Duration(min)*time.Minute + time.Duration(sec)*time.Second)
		}
	case time.Time:
		job.addAtTime(time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute + time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())*time.Nanosecond)
	default:
		job.error = wrapOrError(job.error, ErrUnsupportedTimeFormat)
	}
	job.startsImmediately = false
	return s
}

// Tag will add a tag when creating a job.
func (s *Scheduler) Tag(t ...string) *Scheduler {
	job := s.getCurrentJob()

	if s.tagsUnique {
		for _, tag := range t {
			if _, ok := s.tags.Load(tag); ok {
				job.error = wrapOrError(job.error, ErrTagsUnique(tag))
				return s
			}
			s.tags.Store(tag, struct{}{})
		}
	}

	job.tags = append(job.tags, t...)
	return s
}

// GetAllTags returns all tags.
func (s *Scheduler) GetAllTags() []string {
	var tags []string
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	for _, job := range s.jobs {
		tags = append(tags, job.Tags()...)
	}
	return tags
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
func (s *Scheduler) setUnit(unit schedulingUnit) {
	job := s.getCurrentJob()
	currentUnit := job.getUnit()
	if currentUnit == duration || currentUnit == crontab {
		job.error = wrapOrError(job.error, ErrInvalidIntervalUnitsSelection)
		return
	}
	job.setUnit(unit)
}

// Millisecond sets the unit with milliseconds
func (s *Scheduler) Millisecond() *Scheduler {
	return s.Milliseconds()
}

// Milliseconds sets the unit with milliseconds
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
	return s.Days()
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
// Note: Only days 1 through 28 are allowed for monthly schedules
// Note: Multiple of the same day of month is not allowed
// Note: Negative numbers are special values and can only occur as single argument
// and count backwards from the end of the month -1 == last day of the month, -2 == penultimate day of the month
func (s *Scheduler) Month(daysOfMonth ...int) *Scheduler {
	return s.Months(daysOfMonth...)
}

// MonthLastDay sets the unit with months at every last day of the month
// The optional parameter is a negative integer denoting days previous to the
// last day of the month. E.g. -1 == the penultimate day of the month,
// -2 == two days for the last day of the month
func (s *Scheduler) MonthLastDay(dayCountBeforeLastDayOfMonth ...int) *Scheduler {
	job := s.getCurrentJob()

	switch l := len(dayCountBeforeLastDayOfMonth); l {
	case 0:
		return s.Months(-1)
	case 1:
		count := dayCountBeforeLastDayOfMonth[0]
		if count >= 0 {
			job.error = wrapOrError(job.error, ErrInvalidMonthLastDayEntry)
			return s
		}
		return s.Months(count - 1)
	default:
		job.error = wrapOrError(job.error, ErrInvalidMonthLastDayEntry)
		return s
	}
}

// Months sets the unit with months
// Note: Only days 1 through 28 are allowed for monthly schedules
// Note: Multiple of the same day of month is not allowed
// Note: Negative numbers are special values and can only occur as single argument
// and count backwards from the end of the month -1 == last day of the month, -2 == penultimate day of the month
func (s *Scheduler) Months(daysOfTheMonth ...int) *Scheduler {
	job := s.getCurrentJob()

	if len(daysOfTheMonth) == 0 {
		job.error = wrapOrError(job.error, ErrInvalidDayOfMonthEntry)
	} else if len(daysOfTheMonth) == 1 {
		dayOfMonth := daysOfTheMonth[0]
		if dayOfMonth < -28 || dayOfMonth == 0 || dayOfMonth > 28 {
			job.error = wrapOrError(job.error, ErrInvalidDayOfMonthEntry)
		}
	} else {
		repeatMap := make(map[int]int)
		for _, dayOfMonth := range daysOfTheMonth {
			if dayOfMonth < 1 || dayOfMonth > 28 {
				job.error = wrapOrError(job.error, ErrInvalidDayOfMonthEntry)
				break
			}

			for _, dayOfMonthInJob := range job.daysOfTheMonth {
				if dayOfMonthInJob == dayOfMonth {
					job.error = wrapOrError(job.error, ErrInvalidDaysOfMonthDuplicateValue)
					break
				}
			}

			if _, ok := repeatMap[dayOfMonth]; ok {
				job.error = wrapOrError(job.error, ErrInvalidDaysOfMonthDuplicateValue)
				break
			}
			repeatMap[dayOfMonth]++
		}
	}
	if job.daysOfTheMonth == nil {
		job.daysOfTheMonth = make([]int, 0)
	}
	job.daysOfTheMonth = append(job.daysOfTheMonth, daysOfTheMonth...)
	job.startsImmediately = false
	s.setUnit(months)
	return s
}

// NOTE: If the dayOfTheMonth for the above two functions is
// more than the number of days in that month, the extra day(s)
// spill over to the next month. Similarly, if it's less than 0,
// it will go back to the month before

// Weekday sets the scheduledWeekdays with a specifics weekdays
func (s *Scheduler) Weekday(weekDay time.Weekday) *Scheduler {
	job := s.getCurrentJob()

	if in := in(job.scheduledWeekdays, weekDay); !in {
		job.scheduledWeekdays = append(job.scheduledWeekdays, weekDay)
	}

	job.startsImmediately = false
	s.setUnit(weeks)
	return s
}

func (s *Scheduler) Midday() *Scheduler {
	return s.At("12:00")
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
	if s.inScheduleChain == nil {
		s.jobsMutex.Lock()
		j := s.newJob(0)
		s.jobs[j.id] = j
		s.jobsMutex.Unlock()
		s.inScheduleChain = &j.id
		return j
	}

	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()

	return s.jobs[*s.inScheduleChain]
}

func (s *Scheduler) now() time.Time {
	return s.time.Now(s.Location())
}

// TagsUnique forces job tags to be unique across the scheduler
// when adding tags with (s *Scheduler) Tag().
// This does not enforce uniqueness on tags added via
// (j *Job) Tag()
func (s *Scheduler) TagsUnique() {
	s.tagsUnique = true
}

// Job puts the provided job in focus for the purpose
// of making changes to the job with the scheduler chain
// and finalized by calling Update()
func (s *Scheduler) Job(j *Job) *Scheduler {
	if job, ok := s.JobsMap()[j.id]; !ok {
		return s
	} else if job != j {
		return s
	}
	s.inScheduleChain = &j.id
	s.updateJob = true
	return s
}

// Update stops the job (if running) and starts it with any updates
// that were made to the job in the scheduler chain. Job() must be
// called first to put the given job in focus.
func (s *Scheduler) Update() (*Job, error) {
	job := s.getCurrentJob()

	if !s.updateJob {
		return job, wrapOrError(job.error, ErrUpdateCalledWithoutJob)
	}
	s.updateJob = false
	job.stop()
	job.setStartsImmediately(false)

	if job.runWithDetails {
		params := job.parameters
		if len(params) > 0 {
			params = job.parameters[:len(job.parameters)-1]
		}
		return s.DoWithJobDetails(job.function, params...)
	}

	if job.runConfig.mode == singletonMode {
		job.SingletonMode()
	}

	return s.Do(job.function, job.parameters...)
}

func (s *Scheduler) Cron(cronExpression string) *Scheduler {
	return s.cron(cronExpression, false)
}

func (s *Scheduler) CronWithSeconds(cronExpression string) *Scheduler {
	return s.cron(cronExpression, true)
}

func (s *Scheduler) cron(cronExpression string, withSeconds bool) *Scheduler {
	job := s.getCurrentJob()

	var withLocation string
	if strings.HasPrefix(cronExpression, "TZ=") || strings.HasPrefix(cronExpression, "CRON_TZ=") {
		withLocation = cronExpression
	} else {
		withLocation = fmt.Sprintf("CRON_TZ=%s %s", s.location.String(), cronExpression)
	}

	var (
		cronSchedule cron.Schedule
		err          error
	)

	if withSeconds {
		p := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		cronSchedule, err = p.Parse(withLocation)
	} else {
		cronSchedule, err = cron.ParseStandard(withLocation)
	}

	if err != nil {
		job.error = wrapOrError(err, ErrCronParseFailure)
	}

	job.cronSchedule = cronSchedule
	job.setUnit(crontab)
	job.startsImmediately = false

	return s
}

func (s *Scheduler) newJob(interval int) *Job {
	return newJob(interval, !s.waitForInterval, s.singletonMode)
}

// WaitForScheduleAll defaults the scheduler to create all
// new jobs with the WaitForSchedule option as true.
// The jobs will not start immediately but rather will
// wait until their first scheduled interval.
func (s *Scheduler) WaitForScheduleAll() {
	s.waitForInterval = true
}

// WaitForSchedule sets the job to not start immediately
// but rather wait until the first scheduled interval.
func (s *Scheduler) WaitForSchedule() *Scheduler {
	job := s.getCurrentJob()
	job.startsImmediately = false
	return s
}

// StartImmediately sets the job to run immediately upon
// starting the scheduler or adding the job to a running
// scheduler. This overrides the jobs start status of any
// previously called methods in the chain.
//
// Note: This is the default behavior of the scheduler
// for most jobs, but is useful for overriding the default
// behavior of Cron scheduled jobs which default to
// WaitForSchedule.
func (s *Scheduler) StartImmediately() *Scheduler {
	job := s.getCurrentJob()
	job.startsImmediately = true
	return s
}

// CustomTime takes an in a struct that implements the TimeWrapper interface
// allowing the caller to mock the time used by the scheduler. This is useful
// for tests relying on gocron.
func (s *Scheduler) CustomTime(customTimeWrapper TimeWrapper) {
	s.time = customTimeWrapper
}

// CustomTimer takes in a function that mirrors the time.AfterFunc
// This is used to mock the time.AfterFunc function used by the scheduler
// for testing long intervals in a short amount of time.
func (s *Scheduler) CustomTimer(customTimer func(d time.Duration, f func()) *time.Timer) {
	s.timer = customTimer
}

func (s *Scheduler) StopBlockingChan() {
	s.startBlockingStopChanMutex.Lock()
	if s.IsRunning() && s.startBlockingStopChan != nil {
		close(s.startBlockingStopChan)
	}
	s.startBlockingStopChanMutex.Unlock()
}

// WithDistributedLocker prevents the same job from being run more than once
// when multiple schedulers are trying to schedule the same job.
//
// One strategy to reduce splay in the job execution times when using
// intervals (e.g. 1s, 1m, 1h), on each scheduler instance, is to use
// StartAt with time.Now().Round(interval) to start the job at the
// next interval boundary.
//
// Another strategy is to use the Cron or CronWithSeconds methods as they
// use the same behavior described above using StartAt.
//
// NOTE - the Locker will NOT lock jobs using the singleton options:
// SingletonMode, or SingletonModeAll
//
// NOTE - beware of potential race conditions when running the Locker
// with SetMaxConcurrentJobs and WaitMode as jobs are not guaranteed
// to be locked when each scheduler's is below its limit and able
// to run the job.
func (s *Scheduler) WithDistributedLocker(l Locker) {
	s.executor.distributedLocker = l
}

// WithDistributedElector prevents the same job from being run more than once
// when multiple schedulers are trying to schedule the same job, by allowing only
// the leader to run jobs. Non-leaders wait until the leader instance goes down
// and then a new leader is elected.
//
// Compared with the distributed lock, the election is the same as leader/follower framework.
// All jobs are only scheduled and execute on the leader scheduler instance. Only when the leader scheduler goes down
// and one of the scheduler instances is successfully elected, then the new leader scheduler instance can schedule jobs.
func (s *Scheduler) WithDistributedElector(e Elector) {
	s.executor.distributedElector = e
}

// RegisterEventListeners accepts EventListeners and registers them for all jobs
// in the scheduler at the time this function is called.
// The event listeners are then called at the times described by each listener.
// If a new job is added, an additional call to this method, or the job specific
// version must be executed in order for the new job to trigger event listeners.
func (s *Scheduler) RegisterEventListeners(eventListeners ...EventListener) {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	for _, job := range s.jobs {
		job.RegisterEventListeners(eventListeners...)
	}
}

// BeforeJobRuns registers an event listener that is called before a job runs.
func (s *Scheduler) BeforeJobRuns(eventListenerFunc func(jobName string)) *Scheduler {
	job := s.getCurrentJob()
	job.mu.Lock()
	defer job.mu.Unlock()
	job.eventListeners.beforeJobRuns = eventListenerFunc

	return s
}

// AfterJobRuns registers an event listener that is called after a job runs.
func (s *Scheduler) AfterJobRuns(eventListenerFunc func(jobName string)) *Scheduler {
	job := s.getCurrentJob()
	job.mu.Lock()
	defer job.mu.Unlock()
	job.eventListeners.afterJobRuns = eventListenerFunc

	return s
}

// WhenJobStarts registers an event listener that is called when a job starts.
func (s *Scheduler) WhenJobReturnsError(eventListenerFunc func(jobName string, err error)) *Scheduler {
	job := s.getCurrentJob()
	job.mu.Lock()
	defer job.mu.Unlock()
	job.eventListeners.onError = eventListenerFunc

	return s
}

// WhenJobStarts registers an event listener that is called when a job starts.
func (s *Scheduler) WhenJobReturnsNoError(eventListenerFunc func(jobName string)) *Scheduler {
	job := s.getCurrentJob()
	job.mu.Lock()
	defer job.mu.Unlock()
	job.eventListeners.noError = eventListenerFunc

	return s
}

func (s *Scheduler) PauseJobExecution(shouldPause bool) {
	s.executor.skipExecution.Store(shouldPause)
}
