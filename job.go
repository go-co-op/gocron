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
		slices.Sort(days)
		slices.Sort(daysEnd)
		ms.days = days
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
// If no days of the month are set, the job will start on the following month and on the
// first at time set.
//
// If no days or at times are set, the job will start X months from now.
//
// If a day of the month is selected that does not exist in all months (e.g. 31st)
// any month that does not have that day will be skipped.
//
// A job can always start per your specification using WithStartAt.
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

func WithStartAt(option StartAtOption) JobOption {
	return func(j *internalJob) error {
		return option(j)
	}
}

type StartAtOption func(*internalJob) error

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

//func WithStartImmediately() JobOption {
//	return func(j *internalJob) error {
//		j.startImmediately = true
//		return nil
//	}
//}

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

var _ jobSchedule = (*monthlyJob)(nil)

type monthlyJob struct {
	days        []int
	daysFromEnd []int
	atTimes     []time.Time
}

func (m monthlyJob) next(lastRun time.Time) time.Time {
	days := make([]int, len(m.days))
	copy(days, m.days)
	firstDayNextMonth := time.Date(lastRun.Year(), lastRun.Month()+1, 1, 0, 0, 0, 0, lastRun.Location())
	for _, daySub := range m.daysFromEnd {
		day := firstDayNextMonth.AddDate(0, 0, daySub).Day()
		days = append(days, day)
	}
	slices.Sort(days)

	next := m.nextMonthDayAtTime(lastRun, days)
	if !next.IsZero() {
		return next
	}
	return m.nextMonthDayAtTime(firstDayNextMonth, days)
}

func (m monthlyJob) nextMonthDayAtTime(lastRun time.Time, days []int) time.Time {
	for _, day := range days {
		if day >= lastRun.Day() {
			for _, at := range m.atTimes {
				atDate := time.Date(lastRun.Year(), lastRun.Month(), day, at.Hour(), at.Minute(), at.Second(), lastRun.Nanosecond(), lastRun.Location())
				if atDate.After(lastRun) {
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
