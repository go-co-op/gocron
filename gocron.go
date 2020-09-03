// Package gocron : A Golang Job Scheduling Package.
//
// An in-process scheduler for periodic jobs that uses the builder pattern
// for configuration. Schedule lets you run Golang functions periodically
// at pre-determined intervals using a simple, human-friendly syntax.
//
// Copyright 2014 Jason Lyu. jasonlvhit@gmail.com .
// All rights reserved.
// Use of this source code is governed by a BSD-style .
// license that can be found in the LICENSE file.
package gocron

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"runtime"
	"time"
)

// Error declarations for gocron related errors
var (
	ErrTimeFormat            = errors.New("time format error")
	ErrParamsNotAdapted      = errors.New("the number of params is not adapted")
	ErrNotAFunction          = errors.New("only functions can be schedule into the job queue")
	ErrPeriodNotSpecified    = errors.New("unspecified job period")
	ErrNotScheduledWeekday   = errors.New("job not scheduled weekly on a weekday")
	ErrJobNotFoundWithTag    = errors.New("no jobs found with given tag")
	ErrUnsupportedTimeFormat = errors.New("the given time format is not supported")
)

// regex patterns for supported time formats
var (
	timeWithSeconds    = regexp.MustCompile(`(?m)^\d{1,2}:\d\d:\d\d$`)
	timeWithoutSeconds = regexp.MustCompile(`(?m)^\d{1,2}:\d\d$`)
)

type timeUnit int

const (
	seconds timeUnit = iota + 1
	minutes
	hours
	days
	weeks
	months
)

func callJobFuncWithParams(jobFunc interface{}, params []interface{}) ([]reflect.Value, error) {
	f := reflect.ValueOf(jobFunc)
	if len(params) != f.Type().NumIn() {
		return nil, ErrParamsNotAdapted
	}
	in := make([]reflect.Value, len(params))
	for k, param := range params {
		in[k] = reflect.ValueOf(param)
	}
	return f.Call(in), nil
}

func getFunctionName(fn interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}

func getFunctionKey(funcName string) string {
	h := sha256.New()
	h.Write([]byte(funcName))
	return fmt.Sprintf("%x", h.Sum(nil))
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
