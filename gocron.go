// goCron : A Golang Job Scheduling Package.
//
// An in-process scheduler for periodic jobs that uses the builder pattern
// for configuration. Schedule lets you run Golang functions periodically
// at pre-determined intervals using a simple, human-friendly syntax.
//
// Inspired by the Ruby module clockwork <https://github.com/tomykaira/clockwork>
// and
// Python package schedule <https://github.com/dbader/schedule>
//
// See also
// http://adam.heroku.com/past/2010/4/13/rethinking_cron/
// http://adam.heroku.com/past/2010/6/30/replace_cron_with_clockwork/
//
// Copyright 2014 Jason Lyu. jasonlvhit@gmail.com .
// All rights reserved.
// Use of this source code is governed by a BSD-style .
// license that can be found in the LICENSE file.
package gocron

import (
	"errors"
	"reflect"
	"runtime"
	"sort"
	"time"
)

// Time location, default set by the time.Local (*time.Location)
var loc *time.Location = time.Local

// Change the time location
func ChangeLoc(loc *time.Location) {
	loc = loc
}

// Max number of jobs, hack it if you need.
const MAXJOBNUM = 10000

// Map for the function task store
var funcs = map[string]interface{}{}

// Map for function and  params of function
var fparams = map[string]([]interface{}){}

type Job struct {

	// pause interval * unit bettween runs
	interval uint64

	// the job job_func to run, func[job_func]
	job_func string
	// time units, ,e.g. 'minutes', 'hours'...
	unit string
	// optional time at which this job runs
	at_time string

	// datetime of last run
	last_run time.Time
	// datetime of next run
	next_run time.Time
	// cache the period between last an next run
	period time.Duration

	// Specific day of the week to start on
	start_day time.Weekday
}

// Create a new job with the time interval.
func NewJob(intervel uint64) *Job {
	return &Job{intervel, "", "", "", time.Unix(0, 0), time.Unix(0, 0), 0, time.Sunday}
}

// True if the job should be run now
func (j *Job) should_run() bool {
	return time.Now().After(j.next_run)
}

//Run the job and immdiately reschedulei it
func (j *Job) run() (result []reflect.Value, err error) {
	f := reflect.ValueOf(funcs[j.job_func])
	params := fparams[j.job_func]
	if len(params) != f.Type().NumIn() {
		err = errors.New("The number of param is not adapted.")
		return
	}
	in := make([]reflect.Value, len(params))
	for k, param := range params {
		in[k] = reflect.ValueOf(param)
	}
	result = f.Call(in)
	j.last_run = time.Now()
	j.scheduleNextRun()
	return
}

// for given function fn , get the name of funciton.
func getFunctionName(fn interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf((fn)).Pointer()).Name()
}

// Specifies the job_func that should be called every time the job runs
//
func (j *Job) Do(job_fun interface{}, params ...interface{}) {
	typ := reflect.TypeOf(job_fun)
	if typ.Kind() != reflect.Func {
		panic("only function can be schedule into the job queue.")
	}

	fname := getFunctionName(job_fun)
	funcs[fname] = job_fun
	fparams[fname] = params
	j.job_func = fname
	//schedule the next run
	j.scheduleNextRun()
}

//	s.Every(1).Day().At("10:30").Do(task)
//	s.Every(1).Monday().At("10:30").Do(task)
func (j *Job) At(t string) *Job {
	hour := int((t[0]-'0')*10 + (t[1] - '0'))
	min := int((t[3]-'0')*10 + (t[4] - '0'))
	if hour < 0 || hour > 23 || min < 0 || min > 59 {
		panic("time format error.")
	}
	// time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	mock := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), int(hour), int(min), 0, 0, loc)

	if j.unit == "days" {
		if time.Now().After(mock) {
			j.last_run = mock
		} else {
			j.last_run = time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day()-1, hour, min, 0, 0, loc)
		}
	} else if j.unit == "weeks" {
		if time.Now().After(mock) {
			i := mock.Weekday() - j.start_day
			if i < 0 {
				i = 7 + i
			}
			j.last_run = time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day()-int(i), hour, min, 0, 0, loc)
		} else {
			j.last_run = time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day()-7, hour, min, 0, 0, loc)
		}
	}
	return j
}

//Compute the instant when this job should run next
func (j *Job) scheduleNextRun() {
	if j.last_run == time.Unix(0, 0) {
		if j.unit == "weeks" {
			i := time.Now().Weekday() - j.start_day
			if i < 0 {
				i = 7 + i
			}
			j.last_run = time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day()-int(i), 0, 0, 0, 0, loc)

		} else {
			j.last_run = time.Now()
		}
	}

	if j.period != 0 {
		// translate all the units to the Seconds
		j.next_run = j.last_run.Add(j.period * time.Second)
	} else {
		switch j.unit {
		case "minutes":
			j.period = time.Duration(j.interval * 60)
			break
		case "hours":
			j.period = time.Duration(j.interval * 60 * 60)
			break
		case "days":
			j.period = time.Duration(j.interval * 60 * 60 * 24)
			break
		case "weeks":
			j.period = time.Duration(j.interval * 60 * 60 * 24 * 7)
			break
		case "seconds":
			j.period = time.Duration(j.interval)
		}
		j.next_run = j.last_run.Add(j.period * time.Second)
	}
}

// the follow functions set the job's unit with seconds,minutes,hours...

// Set the unit with second
func (j *Job) Second() (job *Job) {
	if j.interval != 1 {
		panic("")
		return
	}
	job = j.Seconds()
	return
}

// Set the unit with seconds
func (j *Job) Seconds() (job *Job) {
	j.unit = "seconds"
	return j
}

// Set the unit  with minute, which interval is 1
func (j *Job) Minute() (job *Job) {
	if j.interval != 1 {
		panic("")
		return
	}
	job = j.Minutes()
	return
}

//set the unit with minute
func (j *Job) Minutes() (job *Job) {
	j.unit = "minutes"
	return j
}

//set the unit with hour, which interval is 1
func (j *Job) Hour() (job *Job) {
	if j.interval != 1 {
		panic("")
		return
	}
	job = j.Hours()
	return
}

// Set the unit with hours
func (j *Job) Hours() (job *Job) {
	j.unit = "hours"
	return j
}

// Set the job's unit with day, which interval is 1
func (j *Job) Day() (job *Job) {
	if j.interval != 1 {
		panic("")
	}
	job = j.Days()
	return
}

// Set the job's unit with days
func (j *Job) Days() *Job {
	j.unit = "days"
	return j
}

/*
// Set the unit with week, which the interval is 1
func (j *Job) Week() (job *Job) {
	if j.interval != 1 {
		panic("")
	}
	job = j.Weeks()
	return
}

*/

// s.Every(1).Monday().Do(task)
// Set the start day with Monday
func (j *Job) Monday() (job *Job) {
	if j.interval != 1 {
		panic("")
	}
	j.start_day = 1
	job = j.Weeks()
	return
}

// Set the start day with Tuesday
func (j *Job) Tuesday() (job *Job) {
	if j.interval != 1 {
		panic("")
	}
	j.start_day = 2
	job = j.Weeks()
	return
}

// Set the start day woth Wednesday
func (j *Job) Wednesday() (job *Job) {
	if j.interval != 1 {
		panic("")
	}
	j.start_day = 3
	job = j.Weeks()
	return
}

// Set the start day with thursday
func (j *Job) Thursday() (job *Job) {
	if j.interval != 1 {
		panic("")
	}
	j.start_day = 4
	job = j.Weeks()
	return
}

// Set the start day with friday
func (j *Job) Friday() (job *Job) {
	if j.interval != 1 {
		panic("")
	}
	j.start_day = 5
	job = j.Weeks()
	return
}

// Set the start day with saturday
func (j *Job) Saturday() (job *Job) {
	if j.interval != 1 {
		panic("")
	}
	j.start_day = 6
	job = j.Weeks()
	return
}

// Set the start day with sunday
func (j *Job) Sunday() (job *Job) {
	if j.interval != 1 {
		panic("")
	}
	j.start_day = 0
	job = j.Weeks()
	return
}

//Set the units as weeks
func (j *Job) Weeks() *Job {
	j.unit = "weeks"
	return j
}

// Class Scheduler, the only data member is the list of jobs.
type Scheduler struct {
	// Array store jobs
	jobs [MAXJOBNUM]*Job

	// Size of jobs which jobs holding.
	size int
}

// Scheduler implements the sort.Interface{} for sorting jobs, by the time next_run

func (s *Scheduler) Len() int {
	return s.size
}

func (s *Scheduler) Swap(i, j int) {
	s.jobs[i], s.jobs[j] = s.jobs[j], s.jobs[i]
}

func (s *Scheduler) Less(i, j int) bool {
	return s.jobs[j].next_run.After(s.jobs[i].next_run)
}

// Create a new scheduler
func NewScheduler() *Scheduler {
	return &Scheduler{[MAXJOBNUM]*Job{}, 0}
}

// Get the current runnable jobs, which should_run is True
func (s *Scheduler) getRunnableJobs() (running_jobs [MAXJOBNUM]*Job, n int) {
	runnable_jobs := [MAXJOBNUM]*Job{}
	n = 0
	sort.Sort(s)
	for i := 0; i < s.size; i++ {
		if s.jobs[i].should_run() {

			runnable_jobs[n] = s.jobs[i]
			//fmt.Println(runnable_jobs)
			n++
		} else {
			break
		}
	}
	return runnable_jobs, n
}

// Datetime when the next job should run.
func (s *Scheduler) NextRun() (*Job, time.Time) {
	if s.size <= 0 {
		return nil, time.Now()
	}
	sort.Sort(s)
	return s.jobs[0], s.jobs[0].next_run
}

// Schedule a new periodic job
func (s *Scheduler) Every(interval uint64) *Job {
	job := NewJob(interval)
	s.jobs[s.size] = job
	s.size++
	return job
}

// Run all the jobs that are scheduled to run.
func (s *Scheduler) RunPending() {
	runnable_jobs, n := s.getRunnableJobs()
	if n != 0 {
		for i := 0; i < n; i++ {
			runnable_jobs[i].run()
		}
	}
}

// Run all jobs regardless if they are scheduled to run or not
func (s *Scheduler) RunAll() {
	for i := 0; i < s.size; i++ {
		s.jobs[i].run()
	}
}

// Run all jobs with delay seconds
func (s *Scheduler) RunAllwithDelay(d int) {
	for i := 0; i < s.size; i++ {
		s.jobs[i].run()
		time.Sleep(time.Duration(d))
	}
}

// Remove specific job j
func (s *Scheduler) Remove(j interface{}) {
	i := 0
	for ; i < s.size; i++ {
		if s.jobs[i].job_func == getFunctionName(j) {
			break
		}
	}

	for j := (i + 1); j < s.size; j++ {
		s.jobs[i] = s.jobs[j]
		i++
	}
	s.size = s.size - 1
}

// Delete all scheduled jobs
func (s *Scheduler) Clear() {
	for i := 0; i < s.size; i++ {
		s.jobs[i] = nil
	}
	s.size = 0
}

// Start all the pending jobs
func (s *Scheduler) Start() {
	for {
		s.RunPending()
	}
}

// The following methods are shortcuts for not having to
// create a Schduler instance

var default_scheduler = NewScheduler()
var jobs = default_scheduler.jobs

// Schedule a new periodic job
func Every(interval uint64) *Job {
	return default_scheduler.Every(interval)
}

// Run all jobs that are scheduled to run
//
// Please note that it is *intended behavior that run_pending()
// does not run missed jobs*. For example, if you've registered a job
// that should run every minute and you only call run_pending()
// in one hour increments then your job won't be run 60 times in
// between but only once.
func RunPending() {
	default_scheduler.RunPending()
}

// Run all jobs regardless if they are scheduled to run or not.
func RunAll() {
	default_scheduler.RunAll()
}

// Run all the jobs with a delay in seconds
//
// A delay of `delay` seconds is added between each job. This can help
// to distribute the system load generated by the jobs more evenly over
// time.
func RunAllwithDelay(d int) {
	default_scheduler.RunAllwithDelay(d)
}

// Run all jobs that are scheduled to run
func Start() {
	default_scheduler.Start()
}

// Clear
func Clear() {
	default_scheduler.Clear()
}

// Remove
func Remove(j interface{}) {
	default_scheduler.Remove(j)
}

// get the next running time
func NextRun() (job *Job, time time.Time) {
	return default_scheduler.NextRun()
}
