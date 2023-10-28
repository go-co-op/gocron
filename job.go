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

// internalJob stores the information needed by the scheduler
// to manage scheduling, starting and stopping the job
type internalJob struct {
	ctx    context.Context
	cancel context.CancelFunc
	id     uuid.UUID
	name   string
	tags   []string
	jobSchedule
	lastRun, nextRun time.Time
	function         any
	parameters       []any
	timer            clockwork.Timer
	singletonMode    bool
	limitRunsTo      *limitRunsTo
	startTime        time.Time
	startImmediately bool
	// event listeners
	afterJobRuns          func(jobID uuid.UUID)
	beforeJobRuns         func(jobID uuid.UUID)
	afterJobRunsWithError func(jobID uuid.UUID, err error)
}

// stop is used to stop the job's timer and cancel the context
// stopping the timer is critical for cleaning up jobs that are
// sleeping in a time.AfterFunc timer when the job is being stopped.
// cancelling the context keeps the executor from continuing to try
// and run the job.
func (j *internalJob) stop() {
	j.timer.Stop()
	j.cancel()
}

// task stores the function and parameters
// that are actually run when the job is executed.
type task struct {
	function   any
	parameters []any
}

// Task defines a function that returns the task
// function and parameters.
type Task func() task

// NewTask provides the job's task function and parameters.
func NewTask(function any, parameters ...any) Task {
	return func() task {
		return task{
			function:   function,
			parameters: parameters,
		}
	}
}

// limitRunsTo is used for managing the number of runs
// when the user only wants the job to run a certain
// number of times and then be removed from the scheduler.
type limitRunsTo struct {
	limit    int
	runCount int
}

// -----------------------------------------------
// -----------------------------------------------
// --------------- Job Variants ------------------
// -----------------------------------------------
// -----------------------------------------------

// JobDefinition defines the interface that must be
// implemented to create a job from the definition.
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
		// since the user didn't provide a timezone either in the crontab or within
		// the Scheduler, we default to time.Local.
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

// CronJob defines a new job using the crontab syntax. `* * * * *`.
// An optional 6th field can be used at the beginning if withSeconds
// is set to true. `* * * * * *`.
// The timezone can be set on the Scheduler using WithLocation. Or in the
// crontab in the form `TZ=America/Chicago * * * * *` or
// `CRON_TZ=America/Chicago * * * * *`
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

// DurationJob defines a new job using time.Duration
// for the interval.
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
		rand: rand.New(rand.NewSource(time.Now().UnixNano())), // nolint:gosec
	}
	return nil
}

func (d durationRandomJobDefinition) task() Task {
	return d.tas
}

// DurationRandomJob defines a new job that runs on a random interval
// between the min and max duration values provided.
func DurationRandomJob(minDuration, maxDuration time.Duration, task Task, options ...JobOption) JobDefinition {
	return durationRandomJobDefinition{
		min:  minDuration,
		max:  maxDuration,
		opts: options,
		tas:  task,
	}
}

func DailyJob(interval uint, atTimes AtTimes, task Task, options ...JobOption) JobDefinition {
	return nil
}

var _ JobDefinition = (*dailyJobDefinition)(nil)

type dailyJobDefinition struct {
	interval uint
	atTimes  AtTimes
	opts     []JobOption
	tas      Task
}

func (d dailyJobDefinition) options() []JobOption {
	return d.opts
}

func (d dailyJobDefinition) setup(i *internalJob, location *time.Location) error {
	// TODO implement me
	panic("implement me")
}

func (d dailyJobDefinition) task() Task {
	return d.tas
}

var _ JobDefinition = (*weeklyJobDefinition)(nil)

type weeklyJobDefinition struct {
	interval      uint
	daysOfTheWeek Weekdays
	atTimes       AtTimes
	opts          []JobOption
	tas           Task
}

func (w weeklyJobDefinition) options() []JobOption {
	return w.opts
}

func (w weeklyJobDefinition) setup(j *internalJob, location *time.Location) error {
	var ws weeklyJob
	ws.interval = w.interval

	if w.daysOfTheWeek == nil {
		return ErrWeeklyJobDaysOfTheWeekNil
	}
	if w.atTimes == nil {
		return ErrWeeklyJobAtTimesNil
	}

	daysOfTheWeek := w.daysOfTheWeek()

	slices.Sort(daysOfTheWeek)

	atTimesDate, err := convertAtTimesToDateTime(w.atTimes, location)
	switch {
	case errors.Is(err, errAtTimeNil):
		return ErrWeeklyJobAtTimeNil
	case errors.Is(err, errAtTimeHours):
		return ErrWeeklyJobHours
	case errors.Is(err, errAtTimeMinSec):
		return ErrWeeklyJobMinutesSeconds
	}
	ws.atTimes = atTimesDate

	j.jobSchedule = ws
	return nil
}

func (w weeklyJobDefinition) task() Task {
	return w.tas
}

type Weekdays func() []time.Weekday

func NewWeekdays(weekday time.Weekday, weekdays ...time.Weekday) Weekdays {
	return func() []time.Weekday {
		return append(weekdays, weekday)
	}
}

func WeeklyJob(interval uint, daysOfTheWeek Weekdays, atTimes AtTimes, task Task, options ...JobOption) JobDefinition {
	return weeklyJobDefinition{
		interval:      interval,
		daysOfTheWeek: daysOfTheWeek,
		atTimes:       atTimes,
		opts:          options,
		tas:           task,
	}
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

	if m.daysOfTheMonth == nil {
		return ErrMonthlyJobDaysNil
	}
	if m.atTimes == nil {
		return ErrMonthlyJobAtTimesNil
	}

	var daysStart, daysEnd []int
	for _, day := range m.daysOfTheMonth() {
		if day > 31 || day == 0 || day < -31 {
			return ErrMonthlyJobDays
		}
		if day > 0 {
			daysStart = append(daysStart, day)
		} else {
			daysEnd = append(daysEnd, day)
		}
	}
	daysStart = removeSliceDuplicatesInt(daysStart)
	slices.Sort(daysStart)
	ms.days = daysStart

	daysEnd = removeSliceDuplicatesInt(daysEnd)
	slices.Sort(daysEnd)
	ms.daysFromEnd = daysEnd

	atTimesDate, err := convertAtTimesToDateTime(m.atTimes, location)
	switch {
	case errors.Is(err, errAtTimeNil):
		return ErrMonthlyJobAtTimeNil
	case errors.Is(err, errAtTimeHours):
		return ErrMonthlyJobHours
	case errors.Is(err, errAtTimeMinSec):
		return ErrMonthlyJobMinutesSeconds
	}
	ms.atTimes = atTimesDate

	j.jobSchedule = ms
	return nil
}

func (m monthlyJobDefinition) task() Task {
	return m.tas
}

type days []int

// DaysOfTheMonth defines a function that returns a list of days.
type DaysOfTheMonth func() days

// NewDaysOfTheMonth provide the days of the month the job should
// run. The days can be positive 1 to 31 and/or negative -31 to -1.
// Negative values count backwards from the end of the month.
// For example: -1 == the last day of the month.
//
//	-5 == 5 days before the end of the month.
func NewDaysOfTheMonth(day int, moreDays ...int) DaysOfTheMonth {
	return func() days {
		moreDays = append(moreDays, day)
		return moreDays
	}
}

type atTime struct {
	hours, minutes, seconds uint
}

func (a atTime) time(location *time.Location) time.Time {
	return time.Date(0, 0, 0, int(a.hours), int(a.minutes), int(a.seconds), 0, location)
}

// AtTime defines a function that returns the internal atTime
type AtTime func() atTime

// NewAtTime provide the hours, minutes and seconds at which
// the job should be run
func NewAtTime(hours, minutes, seconds uint) AtTime {
	return func() atTime {
		return atTime{hours: hours, minutes: minutes, seconds: seconds}
	}
}

// AtTimes define a list of AtTime
type AtTimes func() []AtTime

// NewAtTimes provide the hours, minutes and seconds at which
// the job should be run
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

var _ jobSchedule = (*dailyJob)(nil)

type dailyJob struct {
	interval uint
	atTimes  []time.Time
}

func (d dailyJob) next(lastRun time.Time) time.Time {
	// TODO implement me
	panic("implement me")
}

var _ jobSchedule = (*weeklyJob)(nil)

type weeklyJob struct {
	interval   uint
	daysOfWeek []time.Weekday
	atTimes    []time.Time
}

func (w weeklyJob) next(lastRun time.Time) time.Time {
	next := w.nextWeekDayAtTime(lastRun)
	if !next.IsZero() {
		return next
	}

	startOfTheNextIntervalWeek := (lastRun.Day() - int(lastRun.Weekday())) + int(w.interval*7)
	from := time.Date(lastRun.Year(), lastRun.Month(), startOfTheNextIntervalWeek, 0, 0, 0, 0, lastRun.Location())
	return w.nextWeekDayAtTime(from)
}

func (w weeklyJob) nextWeekDayAtTime(lastRun time.Time) time.Time {
	for _, wd := range w.daysOfWeek {
		// checking if we're on the same day or later in the same week
		if wd >= lastRun.Weekday() {
			// weekDayDiff is used to add the correct amount to the atDate day below
			weekDayDiff := wd - lastRun.Weekday()
			for _, at := range w.atTimes {
				// sub the at time hour/min/sec onto the lastRun's values
				// to use in checks to see if we've got our next run time
				atDate := time.Date(lastRun.Year(), lastRun.Month(), lastRun.Day()+int(weekDayDiff), at.Hour(), at.Minute(), at.Second(), lastRun.Nanosecond(), lastRun.Location())

				if atDate.After(lastRun) {
					// checking to see if it is after i.e. greater than,
					// and not greater or equal as our lastRun day/time
					// will be in the loop, and we don't want to select it again
					return atDate
				}
			}
		}
	}
	return time.Time{}
}

var _ jobSchedule = (*monthlyJob)(nil)

type monthlyJob struct {
	interval    uint
	days        []int
	daysFromEnd []int
	atTimes     []time.Time
}

func (m monthlyJob) next(lastRun time.Time) time.Time {
	daysList := make([]int, len(m.days))
	copy(daysList, m.days)
	firstDayNextMonth := time.Date(lastRun.Year(), lastRun.Month()+1, 1, 0, 0, 0, 0, lastRun.Location())
	for _, daySub := range m.daysFromEnd {
		// getting a combined list of all the daysList and the negative daysList
		// which count backwards from the first day of the next month
		// -1 == the last day of the month
		day := firstDayNextMonth.AddDate(0, 0, daySub).Day()
		daysList = append(daysList, day)
	}
	slices.Sort(daysList)

	next := m.nextMonthDayAtTime(lastRun, daysList)
	if !next.IsZero() {
		return next
	}

	from := time.Date(lastRun.Year(), lastRun.Month()+time.Month(m.interval), 1, 0, 0, 0, 0, lastRun.Location())
	for next.IsZero() {
		next = m.nextMonthDayAtTime(from, daysList)
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

// Job provides the available methods on the job
// available to the caller.
type Job interface {
	ID() uuid.UUID
	LastRun() (time.Time, error)
	Name() string
	NextRun() (time.Time, error)
	Tags() []string
}

var _ Job = (*job)(nil)

// job is the internal struct that implements
// the public interface. This is used to avoid
// leaking information the caller never needs
// to have or tinker with.
type job struct {
	id            uuid.UUID
	name          string
	tags          []string
	jobOutRequest chan jobOutRequest
}

// ID returns the job's unique identifier.
func (j job) ID() uuid.UUID {
	return j.id
}

// LastRun returns the time of the job's last run
func (j job) LastRun() (time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ij := requestJob(ctx, j.id, j.jobOutRequest)
	if ij == nil || ij.id == uuid.Nil {
		return time.Time{}, ErrJobNotFound
	}
	return ij.lastRun, nil
}

// Name returns the name defined on the job.
func (j job) Name() string {
	return j.name
}

// NextRun returns the time of the job's next scheduled run.
func (j job) NextRun() (time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ij := requestJob(ctx, j.id, j.jobOutRequest)
	if ij == nil || ij.id == uuid.Nil {
		return time.Time{}, ErrJobNotFound
	}
	return ij.nextRun, nil
}

// Tags returns the job's string tags.
func (j job) Tags() []string {
	return j.tags
}
