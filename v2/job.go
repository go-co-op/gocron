package v2

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

var _ Job = (*job)(nil)

type Job interface {
}

func newJob() (Job, error) {
	return &job{}, nil
}

type job struct {
	location *time.Location
	jobSchedule
}

// -----------------------------------------------
// -----------------------------------------------
// -------------- New Job Variants ---------------
// -----------------------------------------------
// -----------------------------------------------

func CronJob(crontab string, withSeconds bool, options ...Option) (Job, error) {
	job := &job{}

	for _, option := range options {
		err := option(job)
		if err != nil {
			return nil, err
		}
	}

	var withLocation string
	if strings.HasPrefix(crontab, "TZ=") || strings.HasPrefix(crontab, "CRON_TZ=") {
		withLocation = crontab
	} else if job.location != nil {
		withLocation = fmt.Sprintf("CRON_TZ=%s %s", job.location.String(), crontab)
	} else {
		withLocation = fmt.Sprintf("CRON_TZ=%s %s", time.Local.String(), crontab)
	}

	cronSchedule, err := cron.ParseStandard(withLocation)
	if err != nil {
		return nil, err
	}
	job.jobSchedule = &cronJob{cronSchedule: cronSchedule}
	return job, nil
}

func DurationJob(duration string, options ...Option) (Job, error) {
	return nil, nil
}

func DurationRandomJob(minDuration, maxDuration string, options ...Option) (Job, error) {
	return nil, nil
}

func DailyJob(interval int, at time.Duration, options ...Option) (Job, error) {
	return nil, nil
}

func HourlyJob(interval int, options ...Option) (Job, error) {
	return nil, nil
}

func MinuteJob(interval int, options ...Option) (Job, error) {
	return nil, nil
}

func MillisecondJob(interval int, options ...Option) (Job, error) {
	return nil, nil
}

func SecondJob(interval int, options ...Option) (Job, error) {
	return nil, nil
}

func WeeklyJob(interval int, daysOfTheWeek []time.Weekday, options ...Option) (Job, error) {
	return nil, nil
}

// -----------------------------------------------
// -----------------------------------------------
// ----------------- Job Options -----------------
// -----------------------------------------------
// -----------------------------------------------

type Option func(*job) error

func LimitRunsTo(runLimit int) Option {
	return func(j *job) error {
		return nil
	}
}

func LockerKey(key string) Option {
	return func(j *job) error {
		return nil
	}
}

func SingletonMode() Option {
	return func(j *job) error {
		return nil
	}
}

func WithContext(ctx context.Context) Option {
	return func(j *job) error {
		return nil
	}
}

func WithEventListeners(eventListeners ...EventListener) Option {
	return func(j *job) error {
		return nil
	}
}

func WithTags(tags ...string) Option {
	return func(j *job) error {
		return nil
	}
}

func WithTimezone(location *time.Location) Option {
	return func(j *job) error {
		j.location = location
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
