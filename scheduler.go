package gocron

import (
	"sort"
	"time"
)

// Scheduler struct, the only data member is the list of jobs.
// - implements the sort.Interface{} for sorting jobs, by the time nextRun
type Scheduler struct {
	jobs []*Job         // Array store jobs
	loc  *time.Location // Location to use when scheduling jobs with specified times
}

var (
	defaultScheduler = NewScheduler(time.Local)
)

// NewScheduler creates a new scheduler
func NewScheduler(loc *time.Location) *Scheduler {

	return &Scheduler{
		jobs: newEmptyJobSlice(),
		loc:  loc,
	}
}

// Jobs returns the list of Jobs from the Scheduler
func (s *Scheduler) Jobs() []*Job {
	return s.jobs
}

func (s *Scheduler) Len() int {
	return len(s.jobs)
}

func (s *Scheduler) Swap(i, j int) {
	s.jobs[i], s.jobs[j] = s.jobs[j], s.jobs[i]
}

func (s *Scheduler) Less(i, j int) bool {
	return s.jobs[j].nextRun.Unix() >= s.jobs[i].nextRun.Unix()
}

// ChangeLoc changes the default time location
func (s *Scheduler) ChangeLoc(newLocation *time.Location) {
	s.loc = newLocation
}

// scheduleNextRun Compute the instant when this job should run next
func (s *Scheduler) scheduleNextRun(j *Job) error {
	if j.nextRun.Location() != s.loc {
		j.nextRun = j.nextRun.In(s.loc)
	}

	now := time.Now().In(s.loc)
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
		j.nextRun = s.roundToMidnight(j.lastRun)
		j.nextRun = j.nextRun.Add(j.atTime)
	case weeks:
		j.nextRun = s.roundToMidnight(j.lastRun)
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

// roundToMidnight truncate time to midnight
func (j *Scheduler) roundToMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, j.loc)
}

// Get the current runnable jobs, which shouldRun is True
func (s *Scheduler) getRunnableJobs() (runningJobs []*Job, n int) {
	var runnableJobs []*Job
	n = 0
	sort.Sort(s)
	for i := 0; i < len(s.jobs); i++ {
		if s.jobs[i].shouldRun() {
			runnableJobs = append(runnableJobs, s.jobs[i])
			n++
		} else {
			break
		}
	}
	return runnableJobs, n
}

// NextRun datetime when the next job should run.
func (s *Scheduler) NextRun() (*Job, time.Time) {
	if len(s.jobs) <= 0 {
		return nil, time.Now()
	}
	sort.Sort(s)
	return s.jobs[0], s.jobs[0].nextRun
}

// Every schedule a new periodic job with interval
func (s *Scheduler) Every(interval uint64) *Job {
	job := NewJob(s, interval)
	s.jobs = append(s.jobs, job)
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
	for _, job := range s.jobs {
		job.run()
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
	for i, job := range s.jobs {
		if shouldRemove(job) {
			s.jobs = removeAtIndex(s.jobs, i)
		}
	}
}

func removeAtIndex(jobs []*Job, i int) []*Job {
	jobs[i] = jobs[len(jobs)-1]
	return jobs[:len(jobs)-1]
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
	s.jobs = newEmptyJobSlice()
}

func newEmptyJobSlice() []*Job {
	const initialCapacity = 256
	return make([]*Job, 0, initialCapacity)
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
