package gocron

import (
	"fmt"
	"time"
)

// Job struct stores the information necessary to run a Job
type Job struct {
	interval          uint64                   // pause interval * unit between runs
	startsImmediately bool                     // if the Job should run upon scheduler start
	jobFunc           string                   // the Job jobFunc to run, func[jobFunc]
	unit              timeUnit                 // time units, ,e.g. 'minutes', 'hours'...
	atTime            time.Duration            // optional time at which this Job runs
	err               error                    // error related to Job
	lastRun           time.Time                // datetime of last run
	nextRun           time.Time                // datetime of next run
	startDay          time.Weekday             // Specific day of the week to start on
	funcs             map[string]interface{}   // Map for the function task store
	fparams           map[string][]interface{} // Map for function and  params of function
	lock              bool                     // lock the Job from running at same time form multiple instances
	tags              []string                 // allow the user to tag Jobs with certain labels
	time              timeHelper               // an instance of timeHelper to interact with the time package
}

// NewJob creates a new Job with the provided interval
func NewJob(interval uint64) *Job {
	th := newTimeHelper()
	return &Job{
		interval: interval,
		lastRun:  th.Unix(0, 0),
		nextRun:  th.Unix(0, 0),
		startDay: time.Sunday,
		funcs:    make(map[string]interface{}),
		fparams:  make(map[string][]interface{}),
		tags:     []string{},
		time:     th,
	}
}

// shouldRun returns true if the Job should be run now
func (j *Job) shouldRun() bool {
	return j.time.Now().Unix() >= j.nextRun.Unix()
}

// Run the Job and immediately reschedule it
func (j *Job) run() {
	j.lastRun = j.time.Now()
	go callJobFuncWithParams(j.funcs[j.jobFunc], j.fparams[j.jobFunc])
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

// NextScheduledTime returns the time of the Job's next scheduled run
func (j *Job) NextScheduledTime() time.Time {
	return j.nextRun
}

// GetScheduledTime returns the specific time of day the Job will run at
func (j *Job) GetScheduledTime() string {
	return fmt.Sprintf("%d:%d", j.atTime/time.Hour, (j.atTime%time.Hour)/time.Minute)
}

// GetWeekday returns which day of the week the Job will run on and
// will return an error if the Job is not scheduled weekly
func (j *Job) GetWeekday() (time.Weekday, error) {
	if j.unit == weeks {
		return j.startDay, nil
	}
	return time.Sunday, ErrNotScheduledWeekday
}
