// Package gocron : A Golang Job Scheduling Package.
//
// An in-process scheduler for periodic jobs that uses the builder pattern
// for configuration. gocron lets you run Golang functions periodically
// at pre-determined intervals using a simple, human-friendly syntax.
//
package gocron

import (
	"errors"
	"reflect"
	"regexp"
	"runtime"
	"time"
)

// Error declarations for gocron related errors
var (
	ErrTimeFormat            = errors.New("time format error")
	ErrNotAFunction          = errors.New("only functions can be schedule into the job queue")
	ErrNotScheduledWeekday   = errors.New("job not scheduled weekly on a weekday")
	ErrJobNotFoundWithTag    = errors.New("no jobs found with given tag")
	ErrUnsupportedTimeFormat = errors.New("the given time format is not supported")
	ErrInvalidInterval       = errors.New(".Every() interval must be greater than 0")
	ErrInvalidIntervalType   = errors.New(".Every() interval must be int, time.Duration, or string")
	ErrInvalidSelection      = errors.New("an .Every() duration interval cannot be used with units (e.g. .Seconds())")
)

// regex patterns for supported time formats
var (
	timeWithSeconds    = regexp.MustCompile(`(?m)^\d{1,2}:\d\d:\d\d$`)
	timeWithoutSeconds = regexp.MustCompile(`(?m)^\d{1,2}:\d\d$`)
)

type timeUnit int

const (
	// default unit is seconds
	seconds timeUnit = iota
	minutes
	hours
	days
	weeks
	months
	duration
)

func callJobFuncWithParams(jobFunc interface{}, params []interface{}) {
	f := reflect.ValueOf(jobFunc)
	if len(params) != f.Type().NumIn() {
		return
	}
	in := make([]reflect.Value, len(params))
	for k, param := range params {
		in[k] = reflect.ValueOf(param)
	}
	f.Call(in)
}

func getFunctionName(fn interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}

func parseTime(t string) (hour, min, sec int, err error) {
	var timeLayout string
	switch {
	case timeWithSeconds.Match([]byte(t)):
		timeLayout = "15:04:05"
	case timeWithoutSeconds.Match([]byte(t)):
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
