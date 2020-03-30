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
	"runtime"
	"strconv"
	"strings"
)

// Error declarations for gocron related errors
var (
	ErrTimeFormat           = errors.New("time format error")
	ErrParamsNotAdapted     = errors.New("the number of params is not adapted")
	ErrNotAFunction         = errors.New("only functions can be schedule into the job queue")
	ErrPeriodNotSpecified   = errors.New("unspecified job period")
	ErrNotScheduledWeekday  = errors.New("job not scheduled weekly on a weekday")
)

// Locker provides an interface for implementing job locking
// to prevent jobs from running at the same time on multiple
// instances of gocron
type Locker interface {
	Lock(key string) (bool, error)
	Unlock(key string) error
}

type timeUnit int

const (
	seconds timeUnit = iota + 1
	minutes
	hours
	days
	weeks
)

var (
	locker Locker
)

// SetLocker sets a locker implementation to be used by
// the scheduler for locking jobs
func SetLocker(l Locker) {
	locker = l
}

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

func formatTime(t string) (hour, min, sec int, err error) {
	ts := strings.Split(t, ":")
	if len(ts) < 2 || len(ts) > 3 {
		return 0, 0, 0, ErrTimeFormat
	}

	if hour, err = strconv.Atoi(ts[0]); err != nil {
		return 0, 0, 0, err
	}
	if min, err = strconv.Atoi(ts[1]); err != nil {
		return 0, 0, 0, err
	}
	if len(ts) == 3 {
		if sec, err = strconv.Atoi(ts[2]); err != nil {
			return 0, 0, 0, err
		}
	}

	if hour < 0 || hour > 23 || min < 0 || min > 59 || sec < 0 || sec > 59 {
		return 0, 0, 0, ErrTimeFormat
	}

	return hour, min, sec, nil
}
