package gocron

import (
	"container/list"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"time"
)

var units = [...]string{
	"seconds",
	"minutes",
	"hours",
	"days",
	"weeks",
}

var weekdays = [...]string{
	"monday",
	"tuesday",
	"wednesday",
	"thursday",
	"friday",
	"saturday",
	"sunday",
}

var funcs = map[string]interface{}{}
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
	start_day string
}

// Create a new job with the time interval.
func NewJob(intervel uint64) *Job {
	return &Job{intervel, "", "", "", time.Unix(0, 0), time.Unix(0, 0), 0, ""}
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

//Compute the instant when this job should run next
func (j *Job) scheduleNextRun() {
	if j.last_run == time.Unix(0, 0) {
		j.last_run = time.Now()
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
func (j *Job) Second() (job *Job, err error) {
	if j.interval != 1 {
		err = errors.New("function Second require the job's interval must be 1.")
		return
	}
	job = j.Seconds()
	return
}

func (j *Job) Seconds() (job *Job) {
	j.unit = "seconds"
	return j
}

func (j *Job) Minute() (job *Job, err error) {
	if j.interval != 1 {
		err = errors.New("function Minute require the job's interval must be 1.")
		return
	}
	job = j.Minutes()
	return
}

func (j *Job) Minutes() (job *Job) {
	j.unit = "minutes"
	return j
}

func (j *Job) Hour() (job *Job, err error) {
	if j.interval != 1 {
		err = errors.New("function Hour require the job's interval must be 1.")
		return
	}
	job = j.Hours()
	return
}

func (j *Job) Hours() (job *Job) {
	j.unit = "hours"
	return j
}

func (j *Job) Day() (job *Job, err error) {
	if j.interval != 1 {
		err = errors.New("function Day require the job's interval must be 1.")
		return
	}
	job = j.Days()
	return
}

func (j *Job) Days() *Job {
	j.unit = "days"
	return j
}

func (j *Job) Week() (job *Job, err error) {
	if j.interval != 1 {
		err = errors.New("function Second require the job's interval must be 1.")
		return
	}
	job = j.Weeks()
	return
}

func (j *Job) Monday() (job *Job, err error) {
	if j.interval != 1 {
		err = errors.New("")
	}
	j.start_day = "monday"
	job = j.Weeks()
	return
}

func (j *Job) Tuesday() (job *Job, err error) {
	if j.interval != 1 {
		err = errors.New("")
	}
	j.start_day = "tuesday"
	job = j.Weeks()
	return
}

func (j *Job) Wednesday() (job *Job, err error) {
	if j.interval != 1 {
		err = errors.New("")
	}
	j.start_day = "wednesday"
	job = j.Weeks()
	return
}

func (j *Job) Thursday() (job *Job, err error) {
	if j.interval != 1 {
		err = errors.New("")
	}
	j.start_day = "thursday"
	job = j.Weeks()
	return
}

func (j *Job) Friday() (job *Job, err error) {
	if j.interval != 1 {
		err = errors.New("")
	}
	j.start_day = "friday"
	job = j.Weeks()
	return
}

func (j *Job) Saturday() (job *Job, err error) {
	if j.interval != 1 {
		err = errors.New("")
	}
	j.start_day = "saturday"
	job = j.Weeks()
	return
}

func (j *Job) Sunday() (job *Job, err error) {
	if j.interval != 1 {
		err = errors.New("")
	}
	j.start_day = "sunday"
	job = j.Weeks()
	return
}

func (j *Job) Weeks() *Job {
	j.unit = "weeks"
	return j
}

// Class Scheduler, the only data member is the list of jobs.
type Scheduler struct {
	jobs *list.List
}

// Create a new scheduler
func NewScheduler() *Scheduler {
	return &Scheduler{list.New()}
}

// Get the current runnable jobs, which should_run is True
func (s *Scheduler) getRunnableJobs() *list.List {
	runnable_jobs := list.New()
	for e := s.jobs.Front(); e != nil; e = e.Next() {
		if e.Value.(*Job).should_run() {
			runnable_jobs.PushBack(e.Value)
		}
	}
	return runnable_jobs
}

// Schedule a new periodic job
func (s *Scheduler) Every(interval uint64) *Job {
	job := NewJob(interval)
	s.jobs.PushBack(job)
	return job
}

// Run all the jobs that are scheduled to run.
func (s *Scheduler) RunPending() {
	runnable_jobs := s.getRunnableJobs()
	if runnable_jobs != nil {
		for e := runnable_jobs.Front(); e != nil; e = e.Next() {
			e.Value.(*Job).run()
		}
	}
}

// Run all jobs regardless if they are scheduled to run or not
func (s *Scheduler) RunAll() {
	for e := s.jobs.Front(); e != nil; e = e.Next() {
		e.Value.(*Job).run()
	}
}

// Run all jobs with delay seconds
func (s *Scheduler) RunAllwithDelay(d int) {
	for e := s.jobs.Front(); e != nil; e = e.Next() {
		e.Value.(*Job).run()
		time.Sleep(time.Duration(d))
	}
}

// Delete all scheduled jobs
func (s *Scheduler) Clear() {
	for e := s.jobs.Front(); e != nil; e = e.Next() {
		s.jobs.Remove(e)
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
