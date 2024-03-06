//go:generate mockgen -destination=mocks/job.go -package=gocronmocks . Job
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
	lastRun, nextRun   time.Time
	function           any
	parameters         []any
	timer              clockwork.Timer
	singletonMode      bool
	singletonLimitMode LimitMode
	limitRunsTo        *limitRunsTo
	startTime          time.Time
	startImmediately   bool
	// event listeners
	afterJobRuns          func(jobID uuid.UUID, jobName string)
	beforeJobRuns         func(jobID uuid.UUID, jobName string)
	afterJobRunsWithError func(jobID uuid.UUID, jobName string, err error)
}

// stop is used to stop the job's timer and cancel the context
// stopping the timer is critical for cleaning up jobs that are
// sleeping in a time.AfterFunc timer when the job is being stopped.
// cancelling the context keeps the executor from continuing to try
// and run the job.
func (j *internalJob) stop() {
	if j.timer != nil {
		j.timer.Stop()
	}
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
	limit    uint
	runCount uint
}

// -----------------------------------------------
// -----------------------------------------------
// --------------- Job Variants ------------------
// -----------------------------------------------
// -----------------------------------------------

// JobDefinition defines the interface that must be
// implemented to create a job from the definition.
type JobDefinition interface {
	setup(*internalJob, *time.Location) error
}

var _ JobDefinition = (*cronJobDefinition)(nil)

type cronJobDefinition struct {
	crontab     string
	withSeconds bool
}

func (c cronJobDefinition) setup(j *internalJob, location *time.Location) error {
	var withLocation string
	if strings.HasPrefix(c.crontab, "TZ=") || strings.HasPrefix(c.crontab, "CRON_TZ=") {
		withLocation = c.crontab
	} else {
		// since the user didn't provide a timezone default to the location
		// passed in by the scheduler. Default: time.Local
		withLocation = fmt.Sprintf("CRON_TZ=%s %s", location.String(), c.crontab)
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

// CronJob defines a new job using the crontab syntax: `* * * * *`.
// An optional 6th field can be used at the beginning if withSeconds
// is set to true: `* * * * * *`.
// The timezone can be set on the Scheduler using WithLocation, or in the
// crontab in the form `TZ=America/Chicago * * * * *` or
// `CRON_TZ=America/Chicago * * * * *`
func CronJob(crontab string, withSeconds bool) JobDefinition {
	return cronJobDefinition{
		crontab:     crontab,
		withSeconds: withSeconds,
	}
}

var _ JobDefinition = (*durationJobDefinition)(nil)

type durationJobDefinition struct {
	duration time.Duration
}

func (d durationJobDefinition) setup(j *internalJob, _ *time.Location) error {
	j.jobSchedule = &durationJob{duration: d.duration}
	return nil
}

// DurationJob defines a new job using time.Duration
// for the interval.
func DurationJob(duration time.Duration) JobDefinition {
	return durationJobDefinition{
		duration: duration,
	}
}

var _ JobDefinition = (*durationRandomJobDefinition)(nil)

type durationRandomJobDefinition struct {
	min, max time.Duration
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

// DurationRandomJob defines a new job that runs on a random interval
// between the min and max duration values provided.
//
// To achieve a similar behavior as tools that use a splay/jitter technique
// consider the median value as the baseline and the difference between the
// max-median or median-min as the splay/jitter.
//
// For example, if you want a job to run every 5 minutes, but want to add
// up to 1 min of jitter to the interval, you could use
// DurationRandomJob(4*time.Minute, 6*time.Minute)
func DurationRandomJob(minDuration, maxDuration time.Duration) JobDefinition {
	return durationRandomJobDefinition{
		min: minDuration,
		max: maxDuration,
	}
}

// DailyJob runs the job on the interval of days, and at the set times.
// By default, the job will start the next available day, considering the last run to be now,
// and the time and day based on the interval and times you input. This means, if you
// select an interval greater than 1, your job by default will run X (interval) days from now
// if there are no atTimes left in the current day. You can use WithStartAt to tell the
// scheduler to start the job sooner.
func DailyJob(interval uint, atTimes AtTimes) JobDefinition {
	return dailyJobDefinition{
		interval: interval,
		atTimes:  atTimes,
	}
}

var _ JobDefinition = (*dailyJobDefinition)(nil)

type dailyJobDefinition struct {
	interval uint
	atTimes  AtTimes
}

func (d dailyJobDefinition) setup(j *internalJob, location *time.Location) error {
	atTimesDate, err := convertAtTimesToDateTime(d.atTimes, location)
	switch {
	case errors.Is(err, errAtTimesNil):
		return ErrDailyJobAtTimesNil
	case errors.Is(err, errAtTimeNil):
		return ErrDailyJobAtTimeNil
	case errors.Is(err, errAtTimeHours):
		return ErrDailyJobHours
	case errors.Is(err, errAtTimeMinSec):
		return ErrDailyJobMinutesSeconds
	}

	ds := dailyJob{
		interval: d.interval,
		atTimes:  atTimesDate,
	}
	j.jobSchedule = ds
	return nil
}

var _ JobDefinition = (*weeklyJobDefinition)(nil)

type weeklyJobDefinition struct {
	interval      uint
	daysOfTheWeek Weekdays
	atTimes       AtTimes
}

func (w weeklyJobDefinition) setup(j *internalJob, location *time.Location) error {
	var ws weeklyJob
	ws.interval = w.interval

	if w.daysOfTheWeek == nil {
		return ErrWeeklyJobDaysOfTheWeekNil
	}

	daysOfTheWeek := w.daysOfTheWeek()

	slices.Sort(daysOfTheWeek)
	ws.daysOfWeek = daysOfTheWeek

	atTimesDate, err := convertAtTimesToDateTime(w.atTimes, location)
	switch {
	case errors.Is(err, errAtTimesNil):
		return ErrWeeklyJobAtTimesNil
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

// Weekdays defines a function that returns a list of week days.
type Weekdays func() []time.Weekday

// NewWeekdays provide the days of the week the job should run.
func NewWeekdays(weekday time.Weekday, weekdays ...time.Weekday) Weekdays {
	return func() []time.Weekday {
		return append(weekdays, weekday)
	}
}

// WeeklyJob runs the job on the interval of weeks, on the specific days of the week
// specified, and at the set times.
//
// By default, the job will start the next available day, considering the last run to be now,
// and the time and day based on the interval, days and times you input. This means, if you
// select an interval greater than 1, your job by default will run X (interval) weeks from now
// if there are no daysOfTheWeek left in the current week. You can use WithStartAt to tell the
// scheduler to start the job sooner.
func WeeklyJob(interval uint, daysOfTheWeek Weekdays, atTimes AtTimes) JobDefinition {
	return weeklyJobDefinition{
		interval:      interval,
		daysOfTheWeek: daysOfTheWeek,
		atTimes:       atTimes,
	}
}

var _ JobDefinition = (*monthlyJobDefinition)(nil)

type monthlyJobDefinition struct {
	interval       uint
	daysOfTheMonth DaysOfTheMonth
	atTimes        AtTimes
}

func (m monthlyJobDefinition) setup(j *internalJob, location *time.Location) error {
	var ms monthlyJob
	ms.interval = m.interval

	if m.daysOfTheMonth == nil {
		return ErrMonthlyJobDaysNil
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
	case errors.Is(err, errAtTimesNil):
		return ErrMonthlyJobAtTimesNil
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
// X (interval) months from now if there are no daysOfTheMonth left in the current month.
// You can use WithStartAt to tell the scheduler to start the job sooner.
//
// Carefully consider your configuration!
//   - For example: an interval of 2 months on the 31st of each month, starting 12/31
//     would skip Feb, April, June, and next run would be in August.
func MonthlyJob(interval uint, daysOfTheMonth DaysOfTheMonth, atTimes AtTimes) JobDefinition {
	return monthlyJobDefinition{
		interval:       interval,
		daysOfTheMonth: daysOfTheMonth,
		atTimes:        atTimes,
	}
}

var _ JobDefinition = (*oneTimeJobDefinition)(nil)

type oneTimeJobDefinition struct {
	startAt OneTimeJobStartAtOption
}

func (o oneTimeJobDefinition) setup(j *internalJob, _ *time.Location) error {
	j.jobSchedule = oneTimeJob{}
	return o.startAt(j)
}

// OneTimeJobStartAtOption defines when the one time job is run
type OneTimeJobStartAtOption func(*internalJob) error

// OneTimeJobStartImmediately tells the scheduler to run the one time job immediately.
func OneTimeJobStartImmediately() OneTimeJobStartAtOption {
	return func(j *internalJob) error {
		j.startImmediately = true
		return nil
	}
}

// OneTimeJobStartDateTime sets the date & time at which the job should run.
// This datetime must be in the future.
func OneTimeJobStartDateTime(start time.Time) OneTimeJobStartAtOption {
	return func(j *internalJob) error {
		if start.IsZero() || start.Before(time.Now()) {
			return ErrOneTimeJobStartDateTimePast
		}
		j.startTime = start
		return nil
	}
}

// OneTimeJob is to run a job once at a specified time and not on
// any regular schedule.
func OneTimeJob(startAt OneTimeJobStartAtOption) JobDefinition {
	return oneTimeJobDefinition{
		startAt: startAt,
	}
}

// -----------------------------------------------
// -----------------------------------------------
// ----------------- Job Options -----------------
// -----------------------------------------------
// -----------------------------------------------

// JobOption defines the constructor for job options.
type JobOption func(*internalJob) error

// WithEventListeners sets the event listeners that should be
// run for the job.
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
func WithLimitedRuns(limit uint) JobOption {
	return func(j *internalJob) error {
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
			return ErrWithNameEmpty
		}
		j.name = name
		return nil
	}
}

// WithSingletonMode keeps the job from running again if it is already running.
// This is useful for jobs that should not overlap, and that occasionally
// (but not consistently) run longer than the interval between job runs.
func WithSingletonMode(mode LimitMode) JobOption {
	return func(j *internalJob) error {
		j.singletonMode = true
		j.singletonLimitMode = mode
		return nil
	}
}

// WithStartAt sets the option for starting the job at
// a specific datetime.
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
// This datetime must be in the future.
func WithStartDateTime(start time.Time) StartAtOption {
	return func(j *internalJob) error {
		if start.IsZero() || start.Before(time.Now()) {
			return ErrWithStartDateTimePast
		}
		j.startTime = start
		return nil
	}
}

// WithTags sets the tags for the job. Tags provide
// a way to identify jobs by a set of tags and remove
// multiple jobs by tag.
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

// EventListener defines the constructor for event
// listeners that can be used to listen for job events.
type EventListener func(*internalJob) error

// AfterJobRuns is used to listen for when a job has run
// without an error, and then run the provided function.
func AfterJobRuns(eventListenerFunc func(jobID uuid.UUID, jobName string)) EventListener {
	return func(j *internalJob) error {
		if eventListenerFunc == nil {
			return ErrEventListenerFuncNil
		}
		j.afterJobRuns = eventListenerFunc
		return nil
	}
}

// AfterJobRunsWithError is used to listen for when a job has run and
// returned an error, and then run the provided function.
func AfterJobRunsWithError(eventListenerFunc func(jobID uuid.UUID, jobName string, err error)) EventListener {
	return func(j *internalJob) error {
		if eventListenerFunc == nil {
			return ErrEventListenerFuncNil
		}
		j.afterJobRunsWithError = eventListenerFunc
		return nil
	}
}

// BeforeJobRuns is used to listen for when a job is about to run and
// then run the provided function.
func BeforeJobRuns(eventListenerFunc func(jobID uuid.UUID, jobName string)) EventListener {
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
	firstPass := true
	next := d.nextDay(lastRun, firstPass)
	if !next.IsZero() {
		return next
	}
	firstPass = false

	startNextDay := time.Date(lastRun.Year(), lastRun.Month(), lastRun.Day()+int(d.interval), 0, 0, 0, lastRun.Nanosecond(), lastRun.Location())
	return d.nextDay(startNextDay, firstPass)
}

func (d dailyJob) nextDay(lastRun time.Time, firstPass bool) time.Time {
	for _, at := range d.atTimes {
		// sub the at time hour/min/sec onto the lastRun's values
		// to use in checks to see if we've got our next run time
		atDate := time.Date(lastRun.Year(), lastRun.Month(), lastRun.Day(), at.Hour(), at.Minute(), at.Second(), lastRun.Nanosecond(), lastRun.Location())

		if firstPass && atDate.After(lastRun) {
			// checking to see if it is after i.e. greater than,
			// and not greater or equal as our lastRun day/time
			// will be in the loop, and we don't want to select it again
			return atDate
		} else if !firstPass && !atDate.Before(lastRun) {
			// now that we're looking at the next day, it's ok to consider
			// the same at time that was last run (as lastRun has been incremented)
			return atDate
		}
	}
	return time.Time{}
}

var _ jobSchedule = (*weeklyJob)(nil)

type weeklyJob struct {
	interval   uint
	daysOfWeek []time.Weekday
	atTimes    []time.Time
}

func (w weeklyJob) next(lastRun time.Time) time.Time {
	firstPass := true
	next := w.nextWeekDayAtTime(lastRun, firstPass)
	if !next.IsZero() {
		return next
	}
	firstPass = false

	startOfTheNextIntervalWeek := (lastRun.Day() - int(lastRun.Weekday())) + int(w.interval*7)
	from := time.Date(lastRun.Year(), lastRun.Month(), startOfTheNextIntervalWeek, 0, 0, 0, 0, lastRun.Location())
	return w.nextWeekDayAtTime(from, firstPass)
}

func (w weeklyJob) nextWeekDayAtTime(lastRun time.Time, firstPass bool) time.Time {
	for _, wd := range w.daysOfWeek {
		// checking if we're on the same day or later in the same week
		if wd >= lastRun.Weekday() {
			// weekDayDiff is used to add the correct amount to the atDate day below
			weekDayDiff := wd - lastRun.Weekday()
			for _, at := range w.atTimes {
				// sub the at time hour/min/sec onto the lastRun's values
				// to use in checks to see if we've got our next run time
				atDate := time.Date(lastRun.Year(), lastRun.Month(), lastRun.Day()+int(weekDayDiff), at.Hour(), at.Minute(), at.Second(), lastRun.Nanosecond(), lastRun.Location())

				if firstPass && atDate.After(lastRun) {
					// checking to see if it is after i.e. greater than,
					// and not greater or equal as our lastRun day/time
					// will be in the loop, and we don't want to select it again
					return atDate
				} else if !firstPass && !atDate.Before(lastRun) {
					// now that we're looking at the next week, it's ok to consider
					// the same at time that was last run (as lastRun has been incremented)
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

	daysFromEnd := m.handleNegativeDays(lastRun, daysList, m.daysFromEnd)
	next := m.nextMonthDayAtTime(lastRun, daysFromEnd, true)
	if !next.IsZero() {
		return next
	}

	from := time.Date(lastRun.Year(), lastRun.Month()+time.Month(m.interval), 1, 0, 0, 0, 0, lastRun.Location())
	for next.IsZero() {
		daysFromEnd = m.handleNegativeDays(from, daysList, m.daysFromEnd)
		next = m.nextMonthDayAtTime(from, daysFromEnd, false)
		from = from.AddDate(0, int(m.interval), 0)
	}

	return next
}

func (m monthlyJob) handleNegativeDays(from time.Time, days, negativeDays []int) []int {
	var out []int
	// getting a list of the days from the end of the following month
	// -1 == the last day of the month
	firstDayNextMonth := time.Date(from.Year(), from.Month()+1, 1, 0, 0, 0, 0, from.Location())
	for _, daySub := range negativeDays {
		day := firstDayNextMonth.AddDate(0, 0, daySub).Day()
		out = append(out, day)
	}
	out = append(out, days...)
	slices.Sort(out)
	return out
}

func (m monthlyJob) nextMonthDayAtTime(lastRun time.Time, days []int, firstPass bool) time.Time {
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

				if firstPass && atDate.After(lastRun) {
					// checking to see if it is after i.e. greater than,
					// and not greater or equal as our lastRun day/time
					// will be in the loop, and we don't want to select it again
					return atDate
				} else if !firstPass && !atDate.Before(lastRun) {
					// now that we're looking at the next month, it's ok to consider
					// the same at time that was  lastRun (as lastRun has been incremented)
					return atDate
				}
			}
			continue
		}
	}
	return time.Time{}
}

var _ jobSchedule = (*oneTimeJob)(nil)

type oneTimeJob struct{}

func (o oneTimeJob) next(_ time.Time) time.Time {
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
	// ID returns the job's unique identifier.
	ID() uuid.UUID
	// LastRun returns the time of the job's last run
	LastRun() (time.Time, error)
	// Name returns the name defined on the job.
	Name() string
	// NextRun returns the time of the job's next scheduled run.
	NextRun() (time.Time, error)
	// RunNow runs the job once, now. This does not alter
	// the existing run schedule, and will respect all job
	// and scheduler limits. This means that running a job now may
	// cause the job's regular interval to be rescheduled due to
	// the instance being run by RunNow blocking your run limit.
	RunNow() error
	// Tags returns the job's string tags.
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
	runJobRequest chan runJobRequest
}

func (j job) ID() uuid.UUID {
	return j.id
}

func (j job) LastRun() (time.Time, error) {
	ij := requestJob(j.id, j.jobOutRequest)
	if ij == nil || ij.id == uuid.Nil {
		return time.Time{}, ErrJobNotFound
	}
	return ij.lastRun, nil
}

func (j job) Name() string {
	return j.name
}

func (j job) NextRun() (time.Time, error) {
	ij := requestJob(j.id, j.jobOutRequest)
	if ij == nil || ij.id == uuid.Nil {
		return time.Time{}, ErrJobNotFound
	}
	return ij.nextRun, nil
}

func (j job) Tags() []string {
	return j.tags
}

func (j job) RunNow() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resp := make(chan error, 1)

	select {
	case j.runJobRequest <- runJobRequest{
		id:      j.id,
		outChan: resp,
	}:
	case <-time.After(100 * time.Millisecond):
		return ErrJobRunNowFailed
	}
	var err error
	select {
	case <-ctx.Done():
		return ErrJobRunNowFailed
	case errReceived := <-resp:
		err = errReceived
	}
	return err
}
