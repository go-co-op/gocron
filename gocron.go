// Package gocron : A Golang Job Scheduling Package.
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
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Time location, default set by the time.Local (*time.Location)
var loc = time.Local

// ChangeLoc change default the time location
func ChangeLoc(newLocation *time.Location) {
	loc = newLocation
	defaultScheduler.ChangeLoc(newLocation)
}

// MAXJOBNUM max number of jobs, hack it if you need.
const MAXJOBNUM = 10000

const (
	seconds = "seconds"
	minutes = "minutes"
	hours   = "hours"
	days    = "days"
	weeks   = "weeks"
)

// Job struct keeping information about job
type Job struct {
	interval uint64                   // pause interval * unit bettween runs
	jobFunc  string                   // the job jobFunc to run, func[jobFunc]
	unit     string                   // time units, ,e.g. 'minutes', 'hours'...
	atTime   time.Duration            // optional time at which this job runs
	loc      *time.Location           // optional timezone that the atTime is in
	lastRun  time.Time                // datetime of last run
	nextRun  time.Time                // datetime of next run
	startDay time.Weekday             // Specific day of the week to start on
	funcs    map[string]interface{}   // Map for the function task store
	fparams  map[string][]interface{} // Map for function and  params of function
	lock     bool                     // lock the job from running at same time form multiple instances
	tags     []string                 // allow the user to tag jobs with certain labels
}

// Locker provides a method to lock jobs from running
// at the same time on multiple instances of gocron.
// You can provide any locker implementation you wish.
type Locker interface {
	Lock(key string) (bool, error)
	Unlock(key string) error
}

var locker Locker

// SetLocker sets a locker implementation
func SetLocker(l Locker) {
	locker = l
}

// NewJob creates a new job with the time interval.
func NewJob(interval uint64) *Job {
	return &Job{
		interval,
		"", "", 0,
		loc,
		time.Unix(0, 0),
		time.Unix(0, 0),
		time.Sunday,
		make(map[string]interface{}),
		make(map[string][]interface{}),
		false,
		[]string{},
	}
}

// True if the job should be run now
func (j *Job) shouldRun() bool {
	return time.Now().Unix() >= j.nextRun.Unix()
}

//Run the job and immediately reschedule it
func (j *Job) run() (result []reflect.Value, err error) {
	if j.lock {
		if locker == nil {
			err = fmt.Errorf("trying to lock %s with nil locker", j.jobFunc)
			return
		}
		key := getFunctionKey(j.jobFunc)

		if ok, err := locker.Lock(key); err != nil || !ok {
			return nil, err
		}

		defer func() {
			if e := locker.Unlock(key); e != nil {
				err = e
			}
		}()
	}

	j.lastRun = time.Now()
	result, err = callJobFuncWithParams(j.funcs[j.jobFunc], j.fparams[j.jobFunc])
	j.scheduleNextRun()
	return
}

func callJobFuncWithParams(jobFunc interface{}, params []interface{}) ([]reflect.Value, error) {
	f := reflect.ValueOf(jobFunc)
	if len(params) != f.Type().NumIn() {
		return nil, errors.New("the number of params is not matched")
	}
	in := make([]reflect.Value, len(params))
	for k, param := range params {
		in[k] = reflect.ValueOf(param)
	}
	return f.Call(in), nil
}

// for given function fn, get the name of function.
func getFunctionName(fn interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}

func getFunctionKey(funcName string) string {
	h := sha256.New()
	h.Write([]byte(funcName))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Do specifies the jobFunc that should be called every time the job runs
func (j *Job) Do(jobFun interface{}, params ...interface{}) {
	typ := reflect.TypeOf(jobFun)
	if typ.Kind() != reflect.Func {
		panic("only function can be schedule into the job queue.")
	}
	fname := getFunctionName(jobFun)
	j.funcs[fname] = jobFun
	j.fparams[fname] = params
	j.jobFunc = fname
	j.scheduleNextRun()
}

// DoSafely does the same thing as Do, but logs unexpected panics, instead of unwinding them up the chain
func (j *Job) DoSafely(jobFun interface{}, params ...interface{}) {
	recoveryWrapperFunc := func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Internal panic occurred: %s", r)
			}
		}()

		_, _ = callJobFuncWithParams(jobFun, params)
	}

	j.Do(recoveryWrapperFunc)
}

// Jobs returns the list of Jobs from the defaultScheduler
func Jobs() []*Job {
	return defaultScheduler.Jobs()
}

func formatTime(t string) (hour, min int, err error) {
	var er = errors.New("time format error")
	ts := strings.Split(t, ":")
	if len(ts) != 2 {
		err = er
		return
	}

	if hour, err = strconv.Atoi(ts[0]); err != nil {
		return
	}
	if min, err = strconv.Atoi(ts[1]); err != nil {
		return
	}

	if hour < 0 || hour > 23 || min < 0 || min > 59 {
		err = er
		return
	}
	return hour, min, nil
}

// At schedules job at specific time of day
//	s.Every(1).Day().At("10:30").Do(task)
//	s.Every(1).Monday().At("10:30").Do(task)
func (j *Job) At(t string) *Job {
	hour, min, err := formatTime(t)
	if err != nil {
		panic(err)
	}
	// save atTime start as duration from midnight
	j.atTime = time.Duration(hour)*time.Hour + time.Duration(min)*time.Minute
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

func (j *Job) periodDuration() time.Duration {
	interval := time.Duration(j.interval)
	switch j.unit {
	case seconds:
		return interval * time.Second
	case minutes:
		return interval * time.Minute
	case hours:
		return interval * time.Hour
	case days:
		return time.Duration(interval * time.Hour * 24)
	case weeks:
		return time.Duration(interval * time.Hour * 24 * 7)
	}
	panic("unspecified job period") // unspecified period
}

// roundToMidnight truncate time to midnight
func (j *Job) roundToMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, j.loc)
}

// scheduleNextRun Compute the instant when this job should run next
func (j *Job) scheduleNextRun() {
	now := time.Now()
	if j.lastRun == time.Unix(0, 0) {
		j.lastRun = now
	}

	if j.nextRun.After(now) {
		return
	}

	switch j.unit {
	case seconds, minutes, hours:
		j.nextRun = j.lastRun.Add(j.periodDuration())
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
		j.nextRun = j.nextRun.Add(j.periodDuration())
	}
}

// NextScheduledTime returns the time of when this job is to run next
func (j *Job) NextScheduledTime() time.Time {
	return j.nextRun
}

// the follow functions set the job's unit with seconds,minutes,hours...

func (j *Job) mustInterval(i uint64) {
	if j.interval != i {
		panic(fmt.Sprintf("interval must be %d", i))
	}
}

// From schedules the next run of the job
func (j *Job) From(t *time.Time) *Job {
	j.nextRun = *t
	return j
}

// NextTick returns a pointer to a time that will run at the next tick
func NextTick() *time.Time {
	now := time.Now().Add(time.Second)
	return &now
}

// setUnit sets unit type
func (j *Job) setUnit(unit string) *Job {
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

// Scheduler struct, the only data member is the list of jobs.
// - implements the sort.Interface{} for sorting jobs, by the time nextRun
type Scheduler struct {
	jobs [MAXJOBNUM]*Job // Array store jobs
	size int             // Size of jobs which jobs holding.
	loc  *time.Location  // Location to use when scheduling jobs with specified times
}

// Jobs returns the list of Jobs from the Scheduler
func (s *Scheduler) Jobs() []*Job {
	return s.jobs[:s.size]
}

func (s *Scheduler) Len() int {
	return s.size
}

func (s *Scheduler) Swap(i, j int) {
	s.jobs[i], s.jobs[j] = s.jobs[j], s.jobs[i]
}

func (s *Scheduler) Less(i, j int) bool {
	return s.jobs[j].nextRun.Unix() >= s.jobs[i].nextRun.Unix()
}

// NewScheduler creates a new scheduler
func NewScheduler() *Scheduler {
	return &Scheduler{
		jobs: [MAXJOBNUM]*Job{},
		size: 0,
		loc:  loc,
	}
}

// ChangeLoc changes the default time location
func (s *Scheduler) ChangeLoc(newLocation *time.Location) {
	s.loc = newLocation
}

// Get the current runnable jobs, which shouldRun is True
func (s *Scheduler) getRunnableJobs() (runningJobs [MAXJOBNUM]*Job, n int) {
	runnableJobs := [MAXJOBNUM]*Job{}
	n = 0
	sort.Sort(s)
	for i := 0; i < s.size; i++ {
		if s.jobs[i].shouldRun() {
			runnableJobs[n] = s.jobs[i]
			n++
		} else {
			break
		}
	}
	return runnableJobs, n
}

// NextRun datetime when the next job should run.
func (s *Scheduler) NextRun() (*Job, time.Time) {
	if s.size <= 0 {
		return nil, time.Now()
	}
	sort.Sort(s)
	return s.jobs[0], s.jobs[0].nextRun
}

// Every schedule a new periodic job with interval
func (s *Scheduler) Every(interval uint64) *Job {
	job := NewJob(interval).Loc(s.loc)
	s.jobs[s.size] = job
	s.size++
	return job
}

// RunPending runs all the jobs that are scheduled to run.
func (s *Scheduler) RunPending() {
	runnableJobs, n := s.getRunnableJobs()

	if n != 0 {
		for i := 0; i < n; i++ {
			runnableJobs[i].run()
		}
	}
}

// RunAll run all jobs regardless if they are scheduled to run or not
func (s *Scheduler) RunAll() {
	s.RunAllwithDelay(0)
}

// RunAllwithDelay runs all jobs with delay seconds
func (s *Scheduler) RunAllwithDelay(d int) {
	for i := 0; i < s.size; i++ {
		s.jobs[i].run()
		if 0 != d {
			time.Sleep(time.Duration(d))
		}
	}
}

// Remove specific job j by function
func (s *Scheduler) Remove(j interface{}) {
	s.removeByCondition(func(someJob *Job) bool {
		return someJob.jobFunc == getFunctionName(j)
	})
}

// RemoveByRef removes specific job j by reference
func (s *Scheduler) RemoveByRef(j *Job) {
	s.removeByCondition(func(someJob *Job) bool {
		return someJob == j
	})
}

func (s *Scheduler) removeByCondition(shouldRemove func(*Job) bool) {
	i := 0

	// keep deleting until no more jobs match the criteria
	for {
		found := false

		for ; i < s.size; i++ {
			if shouldRemove(s.jobs[i]) {
				found = true
				break
			}
		}

		if !found {
			return
		}

		for j := (i + 1); j < s.size; j++ {
			s.jobs[i] = s.jobs[j]
			i++
		}
		s.size--
		s.jobs[s.size] = nil
	}
}

// Scheduled checks if specific job j was already added
func (s *Scheduler) Scheduled(j interface{}) bool {
	for _, job := range s.jobs {
		if job.jobFunc == getFunctionName(j) {
			return true
		}
	}
	return false
}

// Clear delete all scheduled jobs
func (s *Scheduler) Clear() {
	for i := 0; i < s.size; i++ {
		s.jobs[i] = nil
	}
	s.size = 0
}

// Start all the pending jobs
// Add seconds ticker
func (s *Scheduler) Start() chan bool {
	stopped := make(chan bool, 1)
	ticker := time.NewTicker(1 * time.Second)

	go func() {
		for {
			select {
			case <-ticker.C:
				s.RunPending()
			case <-stopped:
				ticker.Stop()
				return
			}
		}
	}()

	return stopped
}

// The following methods are shortcuts for not having to
// create a Scheduler instance

var defaultScheduler = NewScheduler()

// Every schedules a new periodic job running in specific interval
func Every(interval uint64) *Job {
	return defaultScheduler.Every(interval)
}

// RunPending run all jobs that are scheduled to run
//
// Please note that it is *intended behavior that run_pending()
// does not run missed jobs*. For example, if you've registered a job
// that should run every minute and you only call run_pending()
// in one hour increments then your job won't be run 60 times in
// between but only once.
func RunPending() {
	defaultScheduler.RunPending()
}

// RunAll run all jobs regardless if they are scheduled to run or not.
func RunAll() {
	defaultScheduler.RunAll()
}

// RunAllwithDelay run all the jobs with a delay in seconds
//
// A delay of `delay` seconds is added between each job. This can help
// to distribute the system load generated by the jobs more evenly over
// time.
func RunAllwithDelay(d int) {
	defaultScheduler.RunAllwithDelay(d)
}

// Start run all jobs that are scheduled to run
func Start() chan bool {
	return defaultScheduler.Start()
}

// Clear all scheduled jobs
func Clear() {
	defaultScheduler.Clear()
}

// Remove a specific job
func Remove(j interface{}) {
	defaultScheduler.Remove(j)
}

// Scheduled checks if specific job j was already added
func Scheduled(j interface{}) bool {
	for _, job := range defaultScheduler.jobs {
		if job.jobFunc == getFunctionName(j) {
			return true
		}
	}
	return false
}

// NextRun gets the next running time
func NextRun() (job *Job, time time.Time) {
	return defaultScheduler.NextRun()
}
