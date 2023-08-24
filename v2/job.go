package v2

import (
	"context"
	"time"
)

var _ Job = (*job)(nil)

type Job interface {
}

func newJob() (Job, error) {
	return &job{}, nil
}

type job struct {
}

func CronJob(crontab string, withSeconds bool, options ...Option) (Job, error) {
	return nil, nil
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

type Option func(Job) error

func LimitRunsTo(runLimit int) Option {
	return func(j Job) error {
		return nil
	}
}

func LockerKey(key string) Option {
	return func(j Job) error {
		return nil
	}
}

func SingletonMode() Option {
	return func(j Job) error {
		return nil
	}
}

func WithContext(ctx context.Context) Option {
	return func(j Job) error {
		return nil
	}
}

func WithEventListeners(eventListeners ...EventListener) Option {
	return func(j Job) error {
		return nil
	}
}

func WithTags(tags ...string) Option {
	return func(j Job) error {
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
