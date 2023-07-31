package gocron

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/atomic"
)

// Job struct stores the information necessary to run a Job
type Job struct {
	mu *jobMutex
	jobFunction
	interval          int             // interval * unit between runs
	random                            // details for randomness
	duration          time.Duration   // time duration between runs
	unit              schedulingUnit  // time units, e.g. 'minutes', 'hours'...
	startsImmediately bool            // if the Job should run upon scheduler start
	atTimes           []time.Duration // optional time(s) at which this Job runs when interval is day
	startAtTime       time.Time       // optional time at which the Job starts
	error             error           // error related to Job

	scheduledWeekdays []time.Weekday // Specific days of the week to start on
	daysOfTheMonth    []int          // Specific days of the month to run the job
	tags              []string       // allow the user to tag Jobs with certain labels
	timer             *time.Timer    // handles running tasks at specific time
	cronSchedule      cron.Schedule  // stores the schedule when a task uses cron
	runWithDetails    bool           // when true the job is passed as the last arg of the jobFunc
}

type jobRunTimes struct {
	jobRunTimesMu *sync.Mutex
	previousRun   time.Time // datetime of the run before last run
	lastRun       time.Time // datetime of last run
	nextRun       time.Time // datetime of next run
}

type random struct {
	rand                *rand.Rand
	randomizeInterval   bool   // whether the interval is random
	randomIntervalRange [2]int // random interval range
}

type jobFunction struct {
	*jobRunTimes                         // tracking all the markers for job run times
	eventListeners                       // additional functions to allow run 'em during job performing
	function          interface{}        // task's function
	parameters        []interface{}      // task's function parameters
	parametersLen     int                // length of the passed parameters
	jobName           string             // key of the distributed lock
	funcName          string             // the name of the function - e.g. main.func1
	runConfig         runConfig          // configuration for how many times to run the job
	singletonQueueMu  *sync.Mutex        // mutex for singletonQueue
	singletonQueue    chan struct{}      // queues jobs for the singleton runner to handle
	singletonRunnerOn *atomic.Bool       // whether the runner function for singleton is running
	ctx               context.Context    // for cancellation
	cancel            context.CancelFunc // for cancellation
	isRunning         *atomic.Bool       // whether the job func is currently being run
	runStartCount     *atomic.Int64      // number of times the job was started
	runFinishCount    *atomic.Int64      // number of times the job was finished
	singletonWg       *sync.WaitGroup    // used by singleton runner
	singletonWgMu     *sync.Mutex        // use to protect the singletonWg
	stopped           *atomic.Bool       // tracks whether the job is currently stopped
	jobFuncNextRun    time.Time          // the next time the job is scheduled to run
}

type eventListeners struct {
	onAfterJobExecution  interface{}                     // deprecated
	onBeforeJobExecution interface{}                     // deprecated
	beforeJobRuns        func(jobName string)            // called before the job executes
	afterJobRuns         func(jobName string)            // called after the job executes
	onError              func(jobName string, err error) // called when the job returns an error
	noError              func(jobName string)            // called when no error is returned
}

type jobMutex struct {
	sync.RWMutex
}

func (jf *jobFunction) copy() jobFunction {
	cp := jobFunction{
		jobRunTimes:       jf.jobRunTimes,
		eventListeners:    jf.eventListeners,
		function:          jf.function,
		parameters:        nil,
		parametersLen:     jf.parametersLen,
		funcName:          jf.funcName,
		jobName:           jf.jobName,
		runConfig:         jf.runConfig,
		singletonQueue:    jf.singletonQueue,
		singletonQueueMu:  jf.singletonQueueMu,
		ctx:               jf.ctx,
		cancel:            jf.cancel,
		isRunning:         jf.isRunning,
		runStartCount:     jf.runStartCount,
		runFinishCount:    jf.runFinishCount,
		singletonWg:       jf.singletonWg,
		singletonWgMu:     jf.singletonWgMu,
		singletonRunnerOn: jf.singletonRunnerOn,
		stopped:           jf.stopped,
		jobFuncNextRun:    jf.jobFuncNextRun,
	}
	cp.parameters = append(cp.parameters, jf.parameters...)
	return cp
}

func (jf *jobFunction) getName() string {
	if jf.jobName != "" {
		return jf.jobName
	}
	return jf.funcName
}

type runConfig struct {
	finiteRuns bool
	maxRuns    int
	mode       mode
}

// mode is the Job's running mode
type mode int8

const (
	// defaultMode disable any mode
	defaultMode mode = iota

	// singletonMode switch to single job mode
	singletonMode
)

// newJob creates a new Job with the provided interval
func newJob(interval int, startImmediately bool, singletonMode bool) *Job {
	ctx, cancel := context.WithCancel(context.Background())
	job := &Job{
		mu:       &jobMutex{},
		interval: interval,
		unit:     seconds,
		jobFunction: jobFunction{
			jobRunTimes: &jobRunTimes{
				jobRunTimesMu: &sync.Mutex{},
				lastRun:       time.Time{},
				nextRun:       time.Time{},
			},
			ctx:               ctx,
			cancel:            cancel,
			isRunning:         atomic.NewBool(false),
			runStartCount:     atomic.NewInt64(0),
			runFinishCount:    atomic.NewInt64(0),
			singletonRunnerOn: atomic.NewBool(false),
			stopped:           atomic.NewBool(false),
		},
		tags:              []string{},
		startsImmediately: startImmediately,
	}
	if singletonMode {
		job.SingletonMode()
	}
	return job
}

// Name sets the name of the current job.
//
// If the scheduler is running using WithDistributedLocker(),
// the job name is used as the distributed lock key.
func (j *Job) Name(name string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.jobName = name
}

func (j *Job) setRandomInterval(a, b int) {
	j.random.rand = rand.New(rand.NewSource(time.Now().UnixNano())) // nolint

	j.random.randomizeInterval = true
	if a < b {
		j.random.randomIntervalRange[0] = a
		j.random.randomIntervalRange[1] = b + 1
	} else {
		j.random.randomIntervalRange[0] = b
		j.random.randomIntervalRange[1] = a + 1
	}
}

func (j *Job) getRandomInterval() int {
	randNum := j.rand.Intn(j.randomIntervalRange[1] - j.randomIntervalRange[0])
	return j.randomIntervalRange[0] + randNum
}

func (j *Job) getInterval() int {
	if j.randomizeInterval {
		return j.getRandomInterval()
	}
	return j.interval
}

func (j *Job) neverRan() bool {
	jobLastRun := j.LastRun()
	return jobLastRun.IsZero()
}

func (j *Job) getStartsImmediately() bool {
	return j.startsImmediately
}

func (j *Job) setStartsImmediately(b bool) {
	j.startsImmediately = b
}

func (j *Job) setTimer(t *time.Timer) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.timer = t
}

func (j *Job) getFirstAtTime() time.Duration {
	var t time.Duration
	if len(j.atTimes) > 0 {
		t = j.atTimes[0]
	}

	return t
}

func (j *Job) getAtTime(lastRun time.Time) time.Duration {
	var r time.Duration
	if len(j.atTimes) == 0 {
		return r
	}

	if len(j.atTimes) == 1 {
		return j.atTimes[0]
	}

	if lastRun.IsZero() {
		r = j.atTimes[0]
	} else {
		for _, d := range j.atTimes {
			nt := time.Date(lastRun.Year(), lastRun.Month(), lastRun.Day(), 0, 0, 0, 0, lastRun.Location()).Add(d)
			if nt.After(lastRun) {
				r = d
				break
			}
		}
	}

	return r
}

func (j *Job) addAtTime(t time.Duration) {
	if len(j.atTimes) == 0 {
		j.atTimes = append(j.atTimes, t)
		return
	}
	exist := false
	index := sort.Search(len(j.atTimes), func(i int) bool {
		atTime := j.atTimes[i]
		b := atTime >= t
		if b {
			exist = atTime == t
		}
		return b
	})

	// ignore if present
	if exist {
		return
	}

	j.atTimes = append(j.atTimes, time.Duration(0))
	copy(j.atTimes[index+1:], j.atTimes[index:])
	j.atTimes[index] = t
}

func (j *Job) getStartAtTime() time.Time {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.startAtTime
}

func (j *Job) setStartAtTime(t time.Time) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.startAtTime = t
}

func (j *Job) getUnit() schedulingUnit {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.unit
}

func (j *Job) setUnit(t schedulingUnit) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.unit = t
}

func (j *Job) getDuration() time.Duration {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.duration
}

func (j *Job) setDuration(t time.Duration) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.duration = t
}

func (j *Job) setInterval(i int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.interval = i
}

// hasTags returns true if all tags are matched on this Job
func (j *Job) hasTags(tags ...string) bool {
	// Build map of all Job tags for easy comparison
	jobTags := map[string]int{}
	for _, tag := range j.tags {
		jobTags[tag] = 0
	}

	// Loop through required tags and if one doesn't exist, return false
	for _, tag := range tags {
		_, ok := jobTags[tag]
		if !ok {
			return false
		}
	}
	return true
}

// Error returns an error if one occurred while creating the Job.
// If multiple errors occurred, they will be wrapped and can be
// checked using the standard unwrap options.
func (j *Job) Error() error {
	return j.error
}

// Context returns the job's context. The context controls cancellation.
func (j *Job) Context() context.Context {
	return j.ctx
}

// Tag allows you to add arbitrary labels to a Job that do not
// impact the functionality of the Job
func (j *Job) Tag(tags ...string) {
	j.tags = append(j.tags, tags...)
}

// Untag removes a tag from a Job
func (j *Job) Untag(t string) {
	var newTags []string
	for _, tag := range j.tags {
		if t != tag {
			newTags = append(newTags, tag)
		}
	}

	j.tags = newTags
}

// Tags returns the tags attached to the Job
func (j *Job) Tags() []string {
	return j.tags
}

// EventListener functions utilize the job's name and are triggered
// by or in the condition that the name suggests
type EventListener func(j *Job)

// BeforeJobRuns is called before the job is run
func BeforeJobRuns(eventListenerFunc func(jobName string)) EventListener {
	return func(j *Job) {
		j.mu.Lock()
		defer j.mu.Unlock()
		j.eventListeners.beforeJobRuns = eventListenerFunc
	}
}

// AfterJobRuns is called after the job is run
// This is called even when an error is returned
func AfterJobRuns(eventListenerFunc func(jobName string)) EventListener {
	return func(j *Job) {
		j.mu.Lock()
		defer j.mu.Unlock()
		j.eventListeners.afterJobRuns = eventListenerFunc
	}
}

// WhenJobReturnsError is called when the job returns an error
func WhenJobReturnsError(eventListenerFunc func(jobName string, err error)) EventListener {
	return func(j *Job) {
		j.mu.Lock()
		defer j.mu.Unlock()
		j.eventListeners.onError = eventListenerFunc
	}
}

// WhenJobReturnsNoError is called when the job does not return an error
// the function must accept a single parameter, which is an error
func WhenJobReturnsNoError(eventListenerFunc func(jobName string)) EventListener {
	return func(j *Job) {
		j.mu.Lock()
		defer j.mu.Unlock()
		j.eventListeners.noError = eventListenerFunc
	}
}

// RegisterEventListeners accepts EventListeners and registers them for the job
// The event listeners are then called at the times described by each listener.
func (j *Job) RegisterEventListeners(eventListeners ...EventListener) {
	for _, el := range eventListeners {
		el(j)
	}
}

// Deprecated: SetEventListeners accepts two functions that will be called, one before and one after the job is run
func (j *Job) SetEventListeners(onBeforeJobExecution interface{}, onAfterJobExecution interface{}) {
	j.eventListeners = eventListeners{
		onBeforeJobExecution: onBeforeJobExecution,
		onAfterJobExecution:  onAfterJobExecution,
	}
}

// ScheduledTime returns the time of the Job's next scheduled run
func (j *Job) ScheduledTime() time.Time {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.nextRun
}

// ScheduledAtTime returns the specific time of day the Job will run at.
// If multiple times are set, the earliest time will be returned.
func (j *Job) ScheduledAtTime() string {
	if len(j.atTimes) == 0 {
		return "00:00"
	}

	return fmt.Sprintf("%02d:%02d", j.getFirstAtTime()/time.Hour, (j.getFirstAtTime()%time.Hour)/time.Minute)
}

// ScheduledAtTimes returns the specific times of day the Job will run at
func (j *Job) ScheduledAtTimes() []string {
	r := make([]string, len(j.atTimes))
	for i, t := range j.atTimes {
		r[i] = fmt.Sprintf("%02d:%02d", t/time.Hour, (t%time.Hour)/time.Minute)
	}

	return r
}

// Weekday returns which day of the week the Job will run on and
// will return an error if the Job is not scheduled weekly
func (j *Job) Weekday() (time.Weekday, error) {
	if len(j.scheduledWeekdays) == 0 {
		return time.Sunday, ErrNotScheduledWeekday
	}
	return j.scheduledWeekdays[0], nil
}

// Weekdays returns a slice of time.Weekday that the Job will run in a week and
// will return an error if the Job is not scheduled weekly
func (j *Job) Weekdays() []time.Weekday {
	// appending on j.scheduledWeekdays may cause a side effect
	if len(j.scheduledWeekdays) == 0 {
		return []time.Weekday{time.Sunday}
	}

	return j.scheduledWeekdays
}

// LimitRunsTo limits the number of executions of this job to n.
// Upon reaching the limit, the job is removed from the scheduler.
//
// Note: If a job is added to a running scheduler and this method is then used
// you may see the job run more than the set limit as job is scheduled immediately
// by default upon being added to the scheduler. It is recommended to use the
// LimitRunsTo() func on the scheduler chain when scheduling the job.
// For example: scheduler.LimitRunsTo(1).Do()
func (j *Job) LimitRunsTo(n int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.runConfig.finiteRuns = true
	j.runConfig.maxRuns = n
}

// SingletonMode prevents a new job from starting if the prior job has not yet
// completed it's run
// Note: If a job is added to a running scheduler and this method is then used
// you may see the job run overrun itself as job is scheduled immediately
// by default upon being added to the scheduler. It is recommended to use the
// SingletonMode() func on the scheduler chain when scheduling the job.
func (j *Job) SingletonMode() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.runConfig.mode = singletonMode

	j.jobFunction.singletonWgMu = &sync.Mutex{}
	j.jobFunction.singletonWgMu.Lock()
	j.jobFunction.singletonWg = &sync.WaitGroup{}
	j.jobFunction.singletonWgMu.Unlock()

	j.jobFunction.singletonQueueMu = &sync.Mutex{}
	j.jobFunction.singletonQueueMu.Lock()
	j.jobFunction.singletonQueue = make(chan struct{}, 100)
	j.jobFunction.singletonQueueMu.Unlock()
}

// shouldRun evaluates if this job should run again
// based on the runConfig
func (j *Job) shouldRun() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return !j.runConfig.finiteRuns || j.runStartCount.Load() < int64(j.runConfig.maxRuns)
}

// LastRun returns the time the job was run last
func (j *Job) LastRun() time.Time {
	j.jobRunTimesMu.Lock()
	defer j.jobRunTimesMu.Unlock()
	return j.lastRun
}

func (j *Job) setLastRun(t time.Time) {
	j.previousRun = j.lastRun
	j.lastRun = t
}

// NextRun returns the time the job will run next
func (j *Job) NextRun() time.Time {
	j.jobRunTimesMu.Lock()
	defer j.jobRunTimesMu.Unlock()
	return j.nextRun
}

func (j *Job) setNextRun(t time.Time) {
	j.jobRunTimesMu.Lock()
	defer j.jobRunTimesMu.Unlock()
	j.nextRun = t
	j.jobFunction.jobFuncNextRun = t
}

// PreviousRun returns the job run time previous to LastRun
func (j *Job) PreviousRun() time.Time {
	j.jobRunTimesMu.Lock()
	defer j.jobRunTimesMu.Unlock()
	return j.previousRun
}

// RunCount returns the number of times the job has been started
func (j *Job) RunCount() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return int(j.runStartCount.Load())
}

// FinishedRunCount returns the number of times the job has finished running
func (j *Job) FinishedRunCount() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return int(j.runFinishCount.Load())
}

func (j *Job) stop() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.timer != nil {
		j.timer.Stop()
	}
	if j.cancel != nil {
		j.cancel()
		j.ctx, j.cancel = context.WithCancel(context.Background())
	}
	j.stopped.Store(true)
}

// IsRunning reports whether any instances of the job function are currently running
func (j *Job) IsRunning() bool {
	return j.isRunning.Load()
}

// you must Lock the job before calling copy
func (j *Job) copy() Job {
	return Job{
		mu:                &jobMutex{},
		jobFunction:       j.jobFunction,
		interval:          j.interval,
		duration:          j.duration,
		unit:              j.unit,
		startsImmediately: j.startsImmediately,
		atTimes:           j.atTimes,
		startAtTime:       j.startAtTime,
		error:             j.error,
		scheduledWeekdays: j.scheduledWeekdays,
		daysOfTheMonth:    j.daysOfTheMonth,
		tags:              j.tags,
		timer:             j.timer,
		cronSchedule:      j.cronSchedule,
		runWithDetails:    j.runWithDetails,
	}
}
