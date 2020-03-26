package gocron

import (
	"errors"
	"time"
)

// Error declarations for gocron
var (
	ErrTimeFormat           = errors.New("time format error")
	ErrParamsNotAdapted     = errors.New("the number of params is not adapted")
	ErrNotAFunction         = errors.New("only functions can be schedule into the job queue")
	ErrPeriodNotSpecified   = errors.New("unspecified job period")
	ErrParameterCannotBeNil = errors.New("nil paramaters cannot be used with reflection")
)

// Job struct keeping information about job
type Job struct {
	interval          uint64                   // pause interval * unit between runs
	startsImmediately bool                     // if the job should run upon scheduler start
	jobFunc           string                   // the job jobFunc to run, func[jobFunc]
	unit              timeUnit                 // time units, ,e.g. 'minutes', 'hours'...
	atTime            time.Duration            // optional time at which this job runs
	err               error                    // error related to job
	lastRun           time.Time                // datetime of last run
	nextRun           time.Time                // datetime of next run
	startDay          time.Weekday             // Specific day of the week to start on
	funcs             map[string]interface{}   // Map for the function task store
	fparams           map[string][]interface{} // Map for function and  params of function
	lock              bool                     // lock the job from running at same time form multiple instances
	tags              []string                 // allow the user to tag jobs with certain labels
}

// NewJob creates a new job with the time interval.
func NewJob(interval uint64) *Job {
	return &Job{
		interval: interval,
		lastRun:  time.Unix(0, 0),
		nextRun:  time.Unix(0, 0),
		startDay: time.Sunday,
		funcs:    make(map[string]interface{}),
		fparams:  make(map[string][]interface{}),
		tags:     []string{},
	}
}

// True if the job should be run now
func (j *Job) shouldRun() bool {
	return time.Now().Unix() >= j.nextRun.Unix()
}

//Run the job and immediately reschedule it
func (j *Job) run() {
	j.lastRun = time.Now()
	go callJobFuncWithParams(j.funcs[j.jobFunc], j.fparams[j.jobFunc])
}

// Err should be checked to ensure an error didn't occur creating the job
func (j *Job) Err() error {
	return j.err
}

// Tag allows you to add labels to a job
// they don't impact the functionality of the job.
func (j *Job) Tag(t string, others ...string) {
	j.tags = append(j.tags, t)
	for _, tag := range others {
		j.tags = append(j.tags, tag)
	}
}

// Untag removes a tag from a job
func (j *Job) Untag(t string) {
	newTags := []string{}
	for _, tag := range j.tags {
		if t != tag {
			newTags = append(newTags, tag)
		}
	}

	j.tags = newTags
}

// Tags returns the tags attached to the job
func (j *Job) Tags() []string {
	return j.tags
}

func (j *Job) periodDuration() (time.Duration, error) {
	interval := time.Duration(j.interval)
	var periodDuration time.Duration

	switch j.unit {
	case seconds:
		periodDuration = interval * time.Second
	case minutes:
		periodDuration = interval * time.Minute
	case hours:
		periodDuration = interval * time.Hour
	case days:
		periodDuration = interval * time.Hour * 24
	case weeks:
		periodDuration = interval * time.Hour * 24 * 7
	default:
		return 0, ErrPeriodNotSpecified
	}
	return periodDuration, nil
}

// NextScheduledTime returns the time of when this job is to run next
func (j *Job) NextScheduledTime() time.Time {
	return j.nextRun
}

// GetWeekday returns which day of the week the job will run on
// This should only be used when .Weekday(...) was called on the job.
func (j *Job) GetWeekday() time.Weekday {
	return j.startDay
}
