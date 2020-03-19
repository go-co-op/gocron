package gocron

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"time"
)

var (
	ErrTimeFormat           = errors.New("time format error")
	ErrParamsNotAdapted     = errors.New("the number of params is not adapted")
	ErrNotAFunction         = errors.New("only functions can be schedule into the job queue")
	ErrPeriodNotSpecified   = errors.New("unspecified job period")
	ErrParameterCannotBeNil = errors.New("nil paramaters cannot be used with reflection")
)

// Job struct keeping information about job
type Job struct {
	interval uint64                   // pause interval * unit between runs
	jobFunc  string                   // the job jobFunc to run, func[jobFunc]
	unit     timeUnit                 // time units, ,e.g. 'minutes', 'hours'...
	atTime   time.Duration            // optional time at which this job runs
	err      error                    // error related to job
	loc      *time.Location           // optional timezone that the atTime is in
	lastRun  time.Time                // datetime of last run
	nextRun  time.Time                // datetime of next run
	startDay time.Weekday             // Specific day of the week to start on
	funcs    map[string]interface{}   // Map for the function task store
	fparams  map[string][]interface{} // Map for function and  params of function
	lock     bool                     // lock the job from running at same time form multiple instances
	tags     []string                 // allow the user to tag jobs with certain labels
}

// NewJob creates a new job with the time interval.
func NewJob(interval uint64) *Job {
	return &Job{
		interval: interval,
		loc:      loc,
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
func (j *Job) run() ([]reflect.Value, error) {
	if j.lock {
		if locker == nil {
			return nil, fmt.Errorf("trying to lock %s with nil locker", j.jobFunc)
		}
		key := getFunctionKey(j.jobFunc)

		locker.Lock(key)
		defer locker.Unlock(key)
	}

	j.lastRun = time.Now()
	if err := j.scheduleNextRun(); err != nil {
		return nil, err
	}
	result, err := callJobFuncWithParams(j.funcs[j.jobFunc], j.fparams[j.jobFunc])
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Err should be checked to ensure an error didn't occur creating the job
func (j *Job) Err() error {
	return j.err
}

// Do specifies the jobFunc that should be called every time the job runs
func (j *Job) Do(jobFun interface{}, params ...interface{}) error {
	if j.err != nil {
		return j.err
	}

	typ := reflect.TypeOf(jobFun)
	if typ.Kind() != reflect.Func {
		return ErrNotAFunction
	}
	fname := getFunctionName(jobFun)
	j.funcs[fname] = jobFun
	j.fparams[fname] = params
	j.jobFunc = fname
	j.scheduleNextRun()

	return nil
}

// DoSafely does the same thing as Do, but logs unexpected panics, instead of unwinding them up the chain
// Deprecated: DoSafely exists due to historical compatibility and will be removed soon. Use Do instead
func (j *Job) DoSafely(jobFun interface{}, params ...interface{}) error {
	recoveryWrapperFunc := func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Internal panic occurred: %s", r)
			}
		}()

		_, _ = callJobFuncWithParams(jobFun, params)
	}

	return j.Do(recoveryWrapperFunc)
}

// At schedules job at specific time of day
//	s.Every(1).Day().At("10:30:01").Do(task)
//	s.Every(1).Monday().At("10:30:01").Do(task)
func (j *Job) At(t string) *Job {
	hour, min, sec, err := formatTime(t)
	if err != nil {
		j.err = ErrTimeFormat
		return j
	}
	// save atTime start as duration from midnight
	j.atTime = time.Duration(hour)*time.Hour + time.Duration(min)*time.Minute + time.Duration(sec)*time.Second
	return j
}

// GetAt returns the specific time of day the job will run at
//	s.Every(1).Day().At("10:30").GetAt() == "10:30"
func (j *Job) GetAt() string {
	return fmt.Sprintf("%d:%d", j.atTime/time.Hour, (j.atTime%time.Hour)/time.Minute)
}

// Loc sets the location for which to interpret "At"
//	s.Every(1).Day().At("10:30").Loc(time.UTC).Do(task)
func (j *Job) Loc(loc *time.Location) *Job {
	j.loc = loc
	return j
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
	switch j.unit {
	case seconds:
		return interval * time.Second, nil
	case minutes:
		return interval * time.Minute, nil
	case hours:
		return interval * time.Hour, nil
	case days:
		return interval * time.Hour * 24, nil
	case weeks:
		return interval * time.Hour * 24 * 7, nil
	}
	return 0, ErrPeriodNotSpecified
}

// roundToMidnight truncate time to midnight
func (j *Job) roundToMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, j.loc)
}

// scheduleNextRun Compute the instant when this job should run next
func (j *Job) scheduleNextRun() error {
	now := time.Now()
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
		j.nextRun = j.roundToMidnight(j.lastRun)
		j.nextRun = j.nextRun.Add(j.atTime)
	case weeks:
		j.nextRun = j.roundToMidnight(j.lastRun)
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

// NextScheduledTime returns the time of when this job is to run next
func (j *Job) NextScheduledTime() time.Time {
	return j.nextRun
}

// set the job's unit with seconds,minutes,hours...
func (j *Job) mustInterval(i uint64) error {
	if j.interval != i {
		return fmt.Errorf("interval must be %d", i)
	}
	return nil
}

// From schedules the next run of the job
func (j *Job) From(t *time.Time) *Job {
	j.nextRun = *t
	return j
}

// setUnit sets unit type
func (j *Job) setUnit(unit timeUnit) *Job {
	j.unit = unit
	return j
}

// Seconds set the unit with seconds
func (j *Job) Seconds() *Job {
	return j.setUnit(seconds)
}

// Minutes set the unit with minute
func (j *Job) Minutes() *Job {
	return j.setUnit(minutes)
}

// Hours set the unit with hours
func (j *Job) Hours() *Job {
	return j.setUnit(hours)
}

// Days set the job's unit with days
func (j *Job) Days() *Job {
	return j.setUnit(days)
}

// Weeks sets the units as weeks
func (j *Job) Weeks() *Job {
	return j.setUnit(weeks)
}

// Second sets the unit with second
func (j *Job) Second() *Job {
	j.mustInterval(1)
	return j.Seconds()
}

// Minute sets the unit  with minute, which interval is 1
func (j *Job) Minute() *Job {
	j.mustInterval(1)
	return j.Minutes()
}

// Hour sets the unit with hour, which interval is 1
func (j *Job) Hour() *Job {
	j.mustInterval(1)
	return j.Hours()
}

// Day sets the job's unit with day, which interval is 1
func (j *Job) Day() *Job {
	j.mustInterval(1)
	return j.Days()
}

// Week sets the job's unit with week, which interval is 1
func (j *Job) Week() *Job {
	j.mustInterval(1)
	return j.Weeks()
}

// Weekday start job on specific Weekday
func (j *Job) Weekday(startDay time.Weekday) *Job {
	j.mustInterval(1)
	j.startDay = startDay
	return j.Weeks()
}

// GetWeekday returns which day of the week the job will run on
// This should only be used when .Weekday(...) was called on the job.
func (j *Job) GetWeekday() time.Weekday {
	return j.startDay
}

// Monday set the start day with Monday
// - s.Every(1).Monday().Do(task)
func (j *Job) Monday() (job *Job) {
	return j.Weekday(time.Monday)
}

// Tuesday sets the job start day Tuesday
func (j *Job) Tuesday() *Job {
	return j.Weekday(time.Tuesday)
}

// Wednesday sets the job start day Wednesday
func (j *Job) Wednesday() *Job {
	return j.Weekday(time.Wednesday)
}

// Thursday sets the job start day Thursday
func (j *Job) Thursday() *Job {
	return j.Weekday(time.Thursday)
}

// Friday sets the job start day Friday
func (j *Job) Friday() *Job {
	return j.Weekday(time.Friday)
}

// Saturday sets the job start day Saturday
func (j *Job) Saturday() *Job {
	return j.Weekday(time.Saturday)
}

// Sunday sets the job start day Sunday
func (j *Job) Sunday() *Job {
	return j.Weekday(time.Sunday)
}

// Lock prevents job to run from multiple instances of gocron
func (j *Job) Lock() *Job {
	j.lock = true
	return j
}
