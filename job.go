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

var _ Job = (*job)(nil)

type Job interface {
}

type job struct {
	ctx    context.Context
	cancel context.CancelFunc
	id     uuid.UUID
	jobSchedule
	lastRun, nextRun time.Time
	function         interface{}
	parameters       []interface{}
	timer            clockwork.Timer
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
	setup(job, *time.Location) (job, error)
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

func (c cronJobDefinition) setup(j job, location *time.Location) (job, error) {
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
		return j, fmt.Errorf("gocron: crontab pare failure: %w", err)
	}

	j.jobSchedule = &cronJob{cronSchedule: cronSchedule}
	return j, nil
}

func NewCronJob(crontab string, withSeconds bool, task Task, options ...JobOption) JobDefinition {
	return cronJobDefinition{
		crontab:     crontab,
		withSeconds: withSeconds,
		opts:        options,
		tas:         task,
	}
}

func DurationJob(duration string, options ...JobOption) JobDefinition {
	return nil
}

func DurationRandomJob(minDuration, maxDuration string, options ...JobOption) JobDefinition {
	return nil
}

func DailyJob(interval int, at time.Duration, options ...JobOption) JobDefinition {
	return nil
}

func HourlyJob(interval int, options ...JobOption) JobDefinition {
	return nil
}

func MinuteJob(interval int, options ...JobOption) JobDefinition {
	return nil
}

func MillisecondJob(interval int, options ...JobOption) JobDefinition {
	return nil
}

func SecondJob(interval int, options ...JobOption) JobDefinition {
	return nil
}

func WeeklyJob(interval int, daysOfTheWeek []time.Weekday, options ...JobOption) JobDefinition {
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
		return nil
	}
}

func WithContext(ctx context.Context) JobOption {
	return func(j *job) error {
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

type EventListener func(Job) error

func AfterJobRuns(eventListenerFunc func()) EventListener {
	return func(j Job) error {
		return nil
	}
}

func BeforeJobRuns(eventListenerFunc func()) EventListener {
	return func(j Job) error {
		return nil
	}
}

func WhenJobReturnsError(eventListenerFunc func()) EventListener {
	return func(j Job) error {
		return nil
	}
}

func WhenJobReturnsNoError(eventListenerFunc func()) EventListener {
	return func(j Job) error {
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
