// Package gocron : A Golang Job Scheduling Package.
//
// An in-process scheduler for periodic jobs that uses the builder pattern
// for configuration. gocron lets you run Golang functions periodically
// at pre-determined intervals using a simple, human-friendly syntax.
package gocron

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"runtime"
	"sync"
	"time"
)

// PanicHandlerFunc represents a type that can be set to handle panics occurring
// during job execution.
type PanicHandlerFunc func(jobName string, recoverData interface{})

// The global panic handler
var (
	panicHandler      PanicHandlerFunc
	panicHandlerMutex = sync.RWMutex{}
)

// SetPanicHandler sets the global panicHandler to the given function.
// Leaving it nil or setting it to nil disables automatic panic handling.
// If the panicHandler is not nil, any panic that occurs during executing a job will be recovered
// and the panicHandlerFunc will be called with the job's funcName and the recover data.
func SetPanicHandler(handler PanicHandlerFunc) {
	panicHandlerMutex.Lock()
	defer panicHandlerMutex.Unlock()
	panicHandler = handler
}

// Error declarations for gocron related errors
var (
	ErrNotAFunction                  = errors.New("gocron: only functions can be scheduled into the job queue")
	ErrNotScheduledWeekday           = errors.New("gocron: job not scheduled weekly on a weekday")
	ErrJobNotFoundWithTag            = errors.New("gocron: no jobs found with given tag")
	ErrJobNotFound                   = errors.New("gocron: no job found")
	ErrUnsupportedTimeFormat         = errors.New("gocron: the given time format is not supported")
	ErrInvalidInterval               = errors.New("gocron: .Every() interval must be greater than 0")
	ErrInvalidIntervalType           = errors.New("gocron: .Every() interval must be int, time.Duration, or string")
	ErrInvalidIntervalUnitsSelection = errors.New("gocron: .Every(time.Duration) and .Cron() cannot be used with units (e.g. .Seconds())")
	ErrInvalidFunctionParameters     = errors.New("gocron: length of function parameters must match job function parameters")

	ErrAtTimeNotSupported               = errors.New("gocron: the At() method is not supported for this time unit")
	ErrWeekdayNotSupported              = errors.New("gocron: weekday is not supported for time unit")
	ErrInvalidDayOfMonthEntry           = errors.New("gocron: only days 1 through 28 and -1 through -28 are allowed for monthly schedules")
	ErrInvalidMonthLastDayEntry         = errors.New("gocron: only a single negative integer is permitted for MonthLastDay")
	ErrTagsUnique                       = func(tag string) error { return fmt.Errorf("gocron: a non-unique tag was set on the job: %s", tag) }
	ErrWrongParams                      = errors.New("gocron: wrong list of params")
	ErrDoWithJobDetails                 = errors.New("gocron: DoWithJobDetails expects a function whose last parameter is a gocron.Job")
	ErrUpdateCalledWithoutJob           = errors.New("gocron: a call to Scheduler.Update() requires a call to Scheduler.Job() first")
	ErrCronParseFailure                 = errors.New("gocron: cron expression failed to be parsed")
	ErrInvalidDaysOfMonthDuplicateValue = errors.New("gocron: duplicate days of month is not allowed in Month() and Months() methods")
)

func wrapOrError(toWrap error, err error) error {
	var returnErr error
	if toWrap != nil && !errors.Is(err, toWrap) {
		returnErr = fmt.Errorf("%s: %w", err, toWrap)
	} else {
		returnErr = err
	}
	return returnErr
}

// regex patterns for supported time formats
var (
	timeWithSeconds    = regexp.MustCompile(`(?m)^\d{1,2}:\d\d:\d\d$`)
	timeWithoutSeconds = regexp.MustCompile(`(?m)^\d{1,2}:\d\d$`)
)

type schedulingUnit int

const (
	// default unit is seconds
	milliseconds schedulingUnit = iota
	seconds
	minutes
	hours
	days
	weeks
	months
	duration
	crontab
)

func (s schedulingUnit) String() string {
	return [...]string{"milliseconds", "seconds", "minutes", "hours", "days", "weeks", "months", "duration", "crontab"}[s]
}

func callJobFunc(jobFunc interface{}) {
	if jobFunc == nil {
		return
	}
	f := reflect.ValueOf(jobFunc)
	if !f.IsZero() {
		f.Call([]reflect.Value{})
	}
}

func callJobFuncWithParams(jobFunc interface{}, params []interface{}) error {
	if jobFunc == nil {
		return nil
	}
	f := reflect.ValueOf(jobFunc)
	if f.IsZero() {
		return nil
	}
	if len(params) != f.Type().NumIn() {
		return nil
	}
	in := make([]reflect.Value, len(params))
	for k, param := range params {
		in[k] = reflect.ValueOf(param)
	}
	vals := f.Call(in)
	for _, val := range vals {
		i := val.Interface()
		if err, ok := i.(error); ok {
			return err
		}
	}
	return nil
}

func getFunctionName(fn interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}

func getFunctionNameOfPointer(fn interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(fn).Elem().Pointer()).Name()
}

func parseTime(t string) (hour, min, sec int, err error) {
	var timeLayout string
	switch {
	case timeWithSeconds.MatchString(t):
		timeLayout = "15:04:05"
	case timeWithoutSeconds.MatchString(t):
		timeLayout = "15:04"
	default:
		return 0, 0, 0, ErrUnsupportedTimeFormat
	}

	parsedTime, err := time.Parse(timeLayout, t)
	if err != nil {
		return 0, 0, 0, ErrUnsupportedTimeFormat
	}
	return parsedTime.Hour(), parsedTime.Minute(), parsedTime.Second(), nil
}
