package gocron

import (
	"fmt"
	"time"
)

// Job struct stores the information necessary to run a Job
type Job struct {
	interval          uint64                   // pause interval * unit between runs
	unit              timeUnit                 // time units, ,e.g. 'minutes', 'hours'...
	periodDuration    time.Duration            // interval * unit
	startsImmediately bool                     // if the Job should run upon scheduler start
	jobFunc           string                   // the Job jobFunc to run, func[jobFunc]
	atTime            time.Duration            // optional time at which this Job runs
	err               error                    // error related to Job
	lastRun           time.Time                // datetime of last run
	nextRun           time.Time                // datetime of next run
	startDay          time.Weekday             // Specific day of the week to start on
	dayOfTheMonth     int                      // Specific day of the month to run the job
	funcs             map[string]interface{}   // Map for the function task store
	fparams           map[string][]interface{} // Map for function and  params of function
	lock              bool                     // lock the Job from running at same time form multiple instances
	tags              []string                 // allow the user to tag Jobs with certain labels
}

// NewJob creates a new Job with the provided interval
func NewJob(interval uint64) *Job {
	th := newTimeWrapper()
	return &Job{
		interval: interval,
		lastRun:  th.Unix(0, 0),
		nextRun:  th.Unix(0, 0),
		startDay: time.Sunday,
		funcs:    make(map[string]interface{}),
		fparams:  make(map[string][]interface{}),
		tags:     []string{},
	}
}

// Run the Job and immediately reschedule it
func (j *Job) run() {
	callJobFuncWithParams(j.funcs[j.jobFunc], j.fparams[j.jobFunc])
}

// Err returns an error if one ocurred while creating the Job
func (j *Job) Err() error {
	return j.err
}

// Tag allows you to add arbitrary labels to a Job that do not
// impact the functionality of the Job
func (j *Job) Tag(t string, others ...string) {
	j.tags = append(j.tags, t)
	for _, tag := range others {
		j.tags = append(j.tags, tag)
	}
}

// Untag removes a tag from a Job
func (j *Job) Untag(t string) {
	newTags := []string{}
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

func (j *Job) setPeriodDuration() error {
	interval := time.Duration(j.interval)

	switch j.unit {
	case seconds:
		j.periodDuration = interval * time.Second
	case minutes:
		j.periodDuration = interval * time.Minute
	case hours:
		j.periodDuration = interval * time.Hour
	case days:
		j.periodDuration = interval * time.Hour * 24
	case weeks:
		j.periodDuration = interval * time.Hour * 24 * 7
	case months:
		// periodDuration doesn't apply here
	default:
		return ErrPeriodNotSpecified
	}
	return nil
}

// ScheduledTime returns the time of the Job's next scheduled run
func (j *Job) ScheduledTime() time.Time {
	return j.nextRun
}

// ScheduledAtTime returns the specific time of day the Job will run at
func (j *Job) ScheduledAtTime() string {
	return fmt.Sprintf("%d:%d", j.atTime/time.Hour, (j.atTime%time.Hour)/time.Minute)
}

// Weekday returns which day of the week the Job will run on and
// will return an error if the Job is not scheduled weekly
func (j *Job) Weekday() (time.Weekday, error) {
	if j.unit == weeks {
		return j.startDay, nil
	}
	return time.Sunday, ErrNotScheduledWeekday
}
