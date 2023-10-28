package gocron

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/robfig/cron/v3"
	"golang.org/x/exp/slices"
)

type internalJob struct {
	ctx    context.Context
	cancel context.CancelFunc
	id     uuid.UUID
	name   string
	tags   []string
	jobSchedule
	lastRun, nextRun time.Time
	function         interface{}
	parameters       []interface{}
	timer            clockwork.Timer
	singletonMode    bool
	limitRunsTo      *limitRunsTo
	lockerKey        string
	startTime        time.Time
	startImmediately bool
	// event listeners
	afterJobRuns          func(jobID uuid.UUID)
	beforeJobRuns         func(jobID uuid.UUID)
	afterJobRunsWithError func(jobID uuid.UUID, err error)
}

func (j *internalJob) stop() {
	j.timer.Stop()
	j.cancel()
}

type task struct {
	function   interface{}
	parameters []interface{}
}

type Task func() task

func NewTask(function interface{}, parameters ...interface{}) Task {
	return func() task {
		return task{
			function:   function,
			parameters: parameters,
		}
	}
}

type limitRunsTo struct {
	limit    int
	runCount int
}

// -----------------------------------------------
// -----------------------------------------------
// --------------- Job Variants ---------------
// -----------------------------------------------
// -----------------------------------------------

type JobDefinition interface {
	options() []JobOption
	setup(*internalJob, *time.Location) error
	task() Task
}

var _ JobDefinition = (*cronJobDefinition)(nil)

type cronJobDefinition struct {
	crontab     string
	withSeconds bool
	opts        []JobOption
	tas         Task
}

func (c cronJobDefinition) options() []JobOption {
	return c.opts
}

func (c cronJobDefinition) task() Task {
	return c.tas
}

func (c cronJobDefinition) setup(j *internalJob, location *time.Location) error {
	var withLocation string
	if strings.HasPrefix(c.crontab, "TZ=") || strings.HasPrefix(c.crontab, "CRON_TZ=") {
		withLocation = c.crontab
	} else if location != nil {
		withLocation = fmt.Sprintf("CRON_TZ=%s %s", location.String(), c.crontab)
	} else {
		withLocation = fmt.Sprintf("CRON_TZ=%s %s", time.Local.String(), c.crontab)
	}

	var (
		cronSchedule cron.Schedule
		err          error
	)

	if c.withSeconds {
		p := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		cronSchedule, err = p.Parse(withLocation)
	} else {
		cronSchedule, err = cron.ParseStandard(withLocation)
	}
	if err != nil {
		return errors.Join(ErrCronJobParse, err)
	}

	j.jobSchedule = &cronJob{cronSchedule: cronSchedule}
	return nil
}

func CronJob(crontab string, withSeconds bool, task Task, options ...JobOption) JobDefinition {
	return cronJobDefinition{
		crontab:     crontab,
		withSeconds: withSeconds,
		opts:        options,
		tas:         task,
	}
}

var _ JobDefinition = (*durationJobDefinition)(nil)

type durationJobDefinition struct {
	duration time.Duration
	opts     []JobOption
	tas      Task
}

func (d durationJobDefinition) options() []JobOption {
	return d.opts
}

func (d durationJobDefinition) setup(j *internalJob, _ *time.Location) error {
	if d.duration <= 0 {
		return ErrDurationJobZero
	}

	j.jobSchedule = &durationJob{duration: d.duration}
	return nil
}

func (d durationJobDefinition) task() Task {
	return d.tas
}

func DurationJob(duration time.Duration, task Task, options ...JobOption) JobDefinition {
	return durationJobDefinition{
		duration: duration,
		opts:     options,
		tas:      task,
	}
}

var _ JobDefinition = (*durationRandomJobDefinition)(nil)

type durationRandomJobDefinition struct {
	min, max time.Duration
	opts     []JobOption
	tas      Task
}

func (d durationRandomJobDefinition) options() []JobOption {
	return d.opts
}

func (d durationRandomJobDefinition) setup(j *internalJob, _ *time.Location) error {
	if d.min >= d.max {
		return ErrDurationRandomJobMinMax
	}

	j.jobSchedule = &durationRandomJob{
		min:  d.min,
		max:  d.max,
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	return nil
}

func (d durationRandomJobDefinition) task() Task {
	return d.tas
}

func DurationRandomJob(minDuration, maxDuration time.Duration, task Task, options ...JobOption) JobDefinition {
	return durationRandomJobDefinition{
		min:  minDuration,
		max:  maxDuration,
		opts: options,
		tas:  task,
	}
}

func DailyJob(interval int, atTimes []time.Duration, task Task, options ...JobOption) JobDefinition {
	return nil
}

var _ JobDefinition = (*weeklyJobDefinition)(nil)

type weeklyJobDefinition struct {
}

func (w weeklyJobDefinition) options() []JobOption {
	//TODO implement me
	panic("implement me")
}

func (w weeklyJobDefinition) setup(i *internalJob, location *time.Location) error {
	//TODO implement me
	panic("implement me")
}

func (w weeklyJobDefinition) task() Task {
	//TODO implement me
	panic("implement me")
}

func WeeklyJob(interval int, daysOfTheWeek []time.Weekday, atTimes []time.Duration, task Task, options ...JobOption) JobDefinition {
	return nil
}

var _ JobDefinition = (*monthlyJobDefinition)(nil)

type monthlyJobDefinition struct {
	interval       uint
	daysOfTheMonth DaysOfTheMonth
	atTimes        AtTimes
	opts           []JobOption
	tas            Task
}

func (m monthlyJobDefinition) options() []JobOption {
	return m.opts
}

func (m monthlyJobDefinition) setup(j *internalJob, location *time.Location) error {
	var ms monthlyJob
	ms.interval = m.interval

	if m.daysOfTheMonth != nil {
		var days, daysEnd []int
		for _, day := range m.daysOfTheMonth() {
			if day > 31 || day == 0 || day < -31 {
				return ErrMonthlyJobDays
			}
			if day > 0 {
				days = append(days, day)
			} else {
				daysEnd = append(daysEnd, day)
			}
		}
		days = removeSliceDuplicatesInt(days)
		slices.Sort(days)
		ms.days = days

		daysEnd = removeSliceDuplicatesInt(daysEnd)
		slices.Sort(daysEnd)
		ms.daysFromEnd = daysEnd
	}

	if m.atTimes != nil {
		var atTimesDate []time.Time
		for _, a := range m.atTimes() {
			if a == nil {
				continue
			}
			at := a()
			if at.hours > 23 {
				return ErrMonthlyJobHours
			} else if at.minutes > 59 || at.seconds > 59 {
				return ErrMonthlyJobMinutesSeconds
			}
			atTimesDate = append(atTimesDate, at.Time(location))
		}
		slices.SortStableFunc(atTimesDate, func(a, b time.Time) int {
			return a.Compare(b)
		})
		ms.atTimes = atTimesDate
	}

	j.jobSchedule = ms
	return nil
}

func (m monthlyJobDefinition) task() Task {
	return m.tas
}

type DaysOfTheMonth func() []int

func NewDaysOfTheMonth(day int, days ...int) DaysOfTheMonth {
	return func() []int {
		days = append(days, day)
		return days
	}
}

type atTime struct {
	hours, minutes, seconds uint
}

func (a atTime) Time(location *time.Location) time.Time {
	return time.Date(0, 0, 0, int(a.hours), int(a.minutes), int(a.seconds), 0, location)
}

type AtTime func() atTime

func NewAtTime(hours, minutes, seconds uint) AtTime {
	return func() atTime {
		return atTime{hours: hours, minutes: minutes, seconds: seconds}
	}
}

type AtTimes func() []AtTime

func NewAtTimes(atTime AtTime, atTimes ...AtTime) AtTimes {
	return func() []AtTime {
		atTimes = append(atTimes, atTime)
		return atTimes
	}
}

// MonthlyJob runs the job on the interval of months, on the specific days of the month
// specified, and at the set times. Days of the month can be 1 to 31 or negative (-1 to -31), which
// count backwards from the end of the month. E.g. -1 is the last day of the month.
//
// If a day of the month is selected that does not exist in all months (e.g. 31st)
// any month that does not have that day will be skipped.
//
// By default, the job will start the next available day, considering the last run to be now,
// and the time and month based on the interval, days and times you input.
// This means, if you select an interval greater than 1, your job by default will run
// X (interval) months from now.
// You can use WithStartAt to tell the scheduler to start the job sooner.
//
// Carefully consider your configuration!
//   - For example: an interval of 2 months on the 31st of each month, starting 12/31
//     would skip Feb, April, June, and next run would be in August.
func MonthlyJob(interval uint, daysOfTheMonth DaysOfTheMonth, atTimes AtTimes, task Task, options ...JobOption) JobDefinition {
	return monthlyJobDefinition{
		interval:       interval,
		daysOfTheMonth: daysOfTheMonth,
		atTimes:        atTimes,
		tas:            task,
		opts:           options,
	}
}

// -----------------------------------------------
// -----------------------------------------------
// ----------------- Job Options -----------------
// -----------------------------------------------
// -----------------------------------------------

type JobOption func(*internalJob) error

func WithEventListeners(eventListeners ...EventListener) JobOption {
	return func(j *internalJob) error {
		for _, eventListener := range eventListeners {
			if err := eventListener(j); err != nil {
				return err
			}
		}
		return nil
	}
}

// WithLimitedRuns limits the number of executions of this job to n.
// Upon reaching the limit, the job is removed from the scheduler.
func WithLimitedRuns(limit int) JobOption {
	return func(j *internalJob) error {
		if limit <= 0 {
			return ErrWithLimitedRunsZero
		}
		j.limitRunsTo = &limitRunsTo{
			limit:    limit,
			runCount: 0,
		}
		return nil
	}
}

// WithName sets the name of the job. Name provides
// a human-readable identifier for the job.
func WithName(name string) JobOption {
	// TODO use the name for metrics and future logging option
	return func(j *internalJob) error {
		if name == "" {
			return fmt.Errorf("gocron: WithName: name must not be empty")
		}
		j.name = name
		return nil
	}
}

func WithSingletonMode() JobOption {
	return func(j *internalJob) error {
		j.singletonMode = true
		return nil
	}
}

// WithStartAt sets the option for starting the job
func WithStartAt(option StartAtOption) JobOption {
	return func(j *internalJob) error {
		return option(j)
	}
}

// StartAtOption defines options for starting the job
type StartAtOption func(*internalJob) error

// WithStartImmediately tells the scheduler to run the job immediately
// regardless of the type or schedule of job. After this immediate run
// the job is scheduled from this time based on the job definition.
func WithStartImmediately() StartAtOption {
	return func(j *internalJob) error {
		j.startImmediately = true
		return nil
	}
}

// WithStartDateTime sets the first date & time at which the job should run.
func WithStartDateTime(start time.Time) StartAtOption {
	return func(j *internalJob) error {
		if start.IsZero() || start.Before(time.Now()) {
			return fmt.Errorf("gocron: WithStartDateTime: start must not be in the past")
		}
		j.startTime = start
		return nil
	}
}

func WithTags(tags ...string) JobOption {
	return func(j *internalJob) error {
		j.tags = tags
		return nil
	}
}

// -----------------------------------------------
// -----------------------------------------------
// ------------- Job Event Listeners -------------
// -----------------------------------------------
// -----------------------------------------------

type EventListener func(*internalJob) error

func AfterJobRuns(eventListenerFunc func(jobID uuid.UUID)) EventListener {
	return func(j *internalJob) error {
		if eventListenerFunc == nil {
			return ErrEventListenerFuncNil
		}
		j.afterJobRuns = eventListenerFunc
		return nil
	}
}

func AfterJobRunsWithError(eventListenerFunc func(jobID uuid.UUID, err error)) EventListener {
	return func(j *internalJob) error {
		if eventListenerFunc == nil {
			return ErrEventListenerFuncNil
		}
		j.afterJobRunsWithError = eventListenerFunc
		return nil
	}
}

func BeforeJobRuns(eventListenerFunc func(jobID uuid.UUID)) EventListener {
	return func(j *internalJob) error {
		if eventListenerFunc == nil {
			return ErrEventListenerFuncNil
		}
		j.beforeJobRuns = eventListenerFunc
		return nil
	}
}

// -----------------------------------------------
// -----------------------------------------------
// ---------------- Job Schedules ----------------
// -----------------------------------------------
// -----------------------------------------------

type jobSchedule interface {
	next(lastRun time.Time) time.Time
}

var _ jobSchedule = (*cronJob)(nil)

type cronJob struct {
	cronSchedule cron.Schedule
}

func (j *cronJob) next(lastRun time.Time) time.Time {
	return j.cronSchedule.Next(lastRun)
}

var _ jobSchedule = (*durationJob)(nil)

type durationJob struct {
	duration time.Duration
}

func (j *durationJob) next(lastRun time.Time) time.Time {
	return lastRun.Add(j.duration)
}

var _ jobSchedule = (*durationRandomJob)(nil)

type durationRandomJob struct {
	min, max time.Duration
	rand     *rand.Rand
}

func (j *durationRandomJob) next(lastRun time.Time) time.Time {
	r := j.rand.Int63n(int64(j.max - j.min))
	return lastRun.Add(j.min + time.Duration(r))
}

var _ jobSchedule = (*weeklyJob)(nil)

type weeklyJob struct {
	daysOfWeek []time.Weekday
	atTimes    []time.Time
}

func (w weeklyJob) next(lastRun time.Time) time.Time {
	//TODO implement me
	panic("implement me")
}

var _ jobSchedule = (*monthlyJob)(nil)

type monthlyJob struct {
	interval    uint
	days        []int
	daysFromEnd []int
	atTimes     []time.Time
}

func (m monthlyJob) next(lastRun time.Time) time.Time {
	days := make([]int, len(m.days))
	copy(days, m.days)
	firstDayNextMonth := time.Date(lastRun.Year(), lastRun.Month()+1, 1, 0, 0, 0, 0, lastRun.Location())
	for _, daySub := range m.daysFromEnd {
		// getting a combined list of all the days and the negative days
		// which count backwards from the first day of the next month
		// -1 == the last day of the month
		day := firstDayNextMonth.AddDate(0, 0, daySub).Day()
		days = append(days, day)
	}
	slices.Sort(days)

	next := m.nextMonthDayAtTime(lastRun, days)
	if !next.IsZero() {
		return next
	}

	from := time.Date(lastRun.Year(), lastRun.Month()+time.Month(m.interval), 1, 0, 0, 0, 0, lastRun.Location())
	for next.IsZero() {
		next = m.nextMonthDayAtTime(from, days)
		from = from.AddDate(0, int(m.interval), 0)
	}

	return next
}

func (m monthlyJob) nextMonthDayAtTime(lastRun time.Time, days []int) time.Time {
	// find the next day in the month that should run and then check for an at time
	for _, day := range days {
		if day >= lastRun.Day() {
			for _, at := range m.atTimes {
				// sub the day, and the at time hour/min/sec onto the lastRun's values
				// to use in checks to see if we've got our next run time
				atDate := time.Date(lastRun.Year(), lastRun.Month(), day, at.Hour(), at.Minute(), at.Second(), lastRun.Nanosecond(), lastRun.Location())

				if atDate.Month() != lastRun.Month() {
					// this check handles if we're setting a day not in the current month
					// e.g. setting day 31 in Feb results in March 2nd
					continue
				}

				if atDate.After(lastRun) {
					// checking to see if it is after i.e. greater than,
					// and not greater or equal as our lastRun day/time
					// will be in the loop, and we don't want to select it again
					return atDate
				}
			}
			continue
		}
	}
	return time.Time{}
}

// -----------------------------------------------
// -----------------------------------------------
// ---------------- Job Interface ----------------
// -----------------------------------------------
// -----------------------------------------------

type Job interface {
	ID() uuid.UUID
	LastRun() (time.Time, error)
	Name() string
	NextRun() (time.Time, error)
	Tags() []string
}

var _ Job = (*job)(nil)

type job struct {
	id            uuid.UUID
	name          string
	tags          []string
	jobOutRequest chan jobOutRequest
}

func (j job) ID() uuid.UUID {
	return j.id
}

func (j job) LastRun() (time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ij := requestJob(j.id, j.jobOutRequest, ctx)
	if ij == nil || ij.id == uuid.Nil {
		return time.Time{}, ErrJobNotFound
	}
	return ij.lastRun, nil
}

func (j job) Name() string {
	return j.name
}

func (j job) NextRun() (time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ij := requestJob(j.id, j.jobOutRequest, ctx)
	if ij == nil || ij.id == uuid.Nil {
		return time.Time{}, ErrJobNotFound
	}
	return ij.nextRun, nil
}

func (j job) Tags() []string {
	return j.tags
}
