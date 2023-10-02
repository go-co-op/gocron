package gocron

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jonboulle/clockwork"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

type job struct {
	ctx    context.Context
	cancel context.CancelFunc
	id     uuid.UUID
	jobSchedule
	lastRun, nextRun time.Time
	function         interface{}
	parameters       []interface{}
	timer            clockwork.Timer
	singletonMode    bool

	// event listeners
	afterJobRuns          func(jobID uuid.UUID)
	beforeJobRuns         func(jobID uuid.UUID)
	afterJobRunsWithError func(jobID uuid.UUID, err error)
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
		return fmt.Errorf("gocron: crontab pare failure: %w", err)
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
		return fmt.Errorf("gocron: duration must be greater than 0")
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
		if ctx == nil || cancel == nil {
			return fmt.Errorf("gocron: context and cancel cannot be nil")
		}
		j.ctx = ctx
		j.cancel = cancel
		return nil
	}
}

func WithDistributedLockerKey(key string) JobOption {
	return func(j *job) error {
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

func WithTags(tags ...string) JobOption {
	return func(j *job) error {
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
		return nil
	}
}

func AfterJobRunsWithError(eventListenerFunc func(jobID uuid.UUID, err error)) EventListener {
	return func(j *job) error {
		return nil
	}
}

func BeforeJobRuns(eventListenerFunc func(jobID uuid.UUID)) EventListener {
	return func(j *job) error {
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
