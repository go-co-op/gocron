package gocron

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/robfig/cron/v3"
)

type job struct {
	ctx    context.Context
	cancel context.CancelFunc
	id     uuid.UUID
	name   string
	tags   map[string]struct{}
	jobSchedule
	lastRun, nextRun time.Time
	function         interface{}
	parameters       []interface{}
	timer            clockwork.Timer
	singletonMode    bool
	lockerKey        string
	startTime        time.Time
	// event listeners
	afterJobRuns          func(jobID uuid.UUID)
	beforeJobRuns         func(jobID uuid.UUID)
	afterJobRunsWithError func(jobID uuid.UUID, err error)
}

func (j *job) stop() {
	j.timer.Stop()
	j.cancel()
}

type Task struct {
	Function   interface{}
	Parameters []interface{}
}

// -----------------------------------------------
// -----------------------------------------------
// --------------- Job Variants ---------------
// -----------------------------------------------
// -----------------------------------------------

type JobDefinition interface {
	options() []JobOption
	setup(*job, *time.Location) error
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

func (c cronJobDefinition) setup(j *job, location *time.Location) error {
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

func (d durationJobDefinition) setup(j *job, location *time.Location) error {
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

func DurationRandomJob(minDuration, maxDuration string, task Task, options ...JobOption) JobDefinition {
	return nil
}

func DailyJob(interval int, at time.Duration, task Task, options ...JobOption) JobDefinition {
	return nil
}

func HourlyJob(interval int, task Task, options ...JobOption) JobDefinition {
	return nil
}

func MinuteJob(interval int, task Task, options ...JobOption) JobDefinition {
	return nil
}

func MillisecondJob(interval int, task Task, options ...JobOption) JobDefinition {
	return nil
}

func SecondJob(interval int, task Task, options ...JobOption) JobDefinition {
	return nil
}

func WeeklyJob(interval int, daysOfTheWeek []time.Weekday, task Task, options ...JobOption) JobDefinition {
	return nil
}

// -----------------------------------------------
// -----------------------------------------------
// ----------------- Job Options -----------------
// -----------------------------------------------
// -----------------------------------------------

type JobOption func(*job) error

func LimitRunsTo(runLimit int) JobOption {
	return func(j *job) error {
		return nil
	}
}

func SingletonMode() JobOption {
	return func(j *job) error {
		j.singletonMode = true
		return nil
	}
}

func WithContext(ctx context.Context, cancel context.CancelFunc) JobOption {
	return func(j *job) error {
		if ctx == nil {
			return ErrWithContextNilContext
		}
		if cancel == nil {
			return ErrWithContextNilCancel
		}
		j.ctx = ctx
		j.cancel = cancel
		return nil
	}
}

func WithDistributedLockerKey(key string) JobOption {
	return func(j *job) error {
		if key == "" {
			return fmt.Errorf("gocron: WithDistributedLockerKey: key must not be empty")
		}
		j.lockerKey = key
		return nil
	}
}

func WithEventListeners(eventListeners ...EventListener) JobOption {
	return func(j *job) error {
		for _, eventListener := range eventListeners {
			if err := eventListener(j); err != nil {
				return err
			}
		}
		return nil
	}
}

// WithName sets the name of the job. Name provides
// a human-readable identifier for the job.
func WithName(name string) JobOption {
	// TODO use the name for metrics and future logging option
	return func(j *job) error {
		if name == "" {
			return fmt.Errorf("gocron: WithName: name must not be empty")
		}
		j.name = name
		return nil
	}
}

// WithStartDateTime sets the first date & time at which the job should run.
func WithStartDateTime(start time.Time) JobOption {
	return func(j *job) error {
		if start.IsZero() || start.Before(time.Now()) {
			return fmt.Errorf("gocron: WithStartDateTime: start must not be in the past")
		}
		j.startTime = start
		return nil
	}
}

func WithTags(tags ...string) JobOption {
	return func(j *job) error {
		mapTags := make(map[string]struct{})
		for _, t := range tags {
			mapTags[t] = struct{}{}
		}
		j.tags = mapTags
		return nil
	}
}

// -----------------------------------------------
// -----------------------------------------------
// ------------- Job Event Listeners -------------
// -----------------------------------------------
// -----------------------------------------------

type EventListener func(*job) error

func AfterJobRuns(eventListenerFunc func(jobID uuid.UUID)) EventListener {
	return func(j *job) error {
		if eventListenerFunc == nil {
			return ErrEventListenerFuncNil
		}
		j.afterJobRuns = eventListenerFunc
		return nil
	}
}

func AfterJobRunsWithError(eventListenerFunc func(jobID uuid.UUID, err error)) EventListener {
	return func(j *job) error {
		if eventListenerFunc == nil {
			return ErrEventListenerFuncNil
		}
		j.afterJobRunsWithError = eventListenerFunc
		return nil
	}
}

func BeforeJobRuns(eventListenerFunc func(jobID uuid.UUID)) EventListener {
	return func(j *job) error {
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

// -----------------------------------------------
// -----------------------------------------------
// ---------------- Job Interface ----------------
// -----------------------------------------------
// -----------------------------------------------

type Job interface {
	Id() uuid.UUID
	LastRun() (time.Time, error)
	NextRun() (time.Time, error)
}

var _ Job = (*publicJob)(nil)

type publicJob struct {
	id            uuid.UUID
	jobOutRequest chan jobOutRequest
}

func (pj publicJob) Id() uuid.UUID {
	return pj.id
}

func (pj publicJob) LastRun() (time.Time, error) {
	j := requestJob(pj.id, pj.jobOutRequest)
	if j.id == uuid.Nil {
		return time.Time{}, ErrJobNotFound
	}
	return j.lastRun, nil
}

func (pj publicJob) NextRun() (time.Time, error) {
	j := requestJob(pj.id, pj.jobOutRequest)
	if j.id == uuid.Nil {
		return time.Time{}, ErrJobNotFound
	}
	return j.nextRun, nil
}
