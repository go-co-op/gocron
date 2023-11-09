package gocron

import (
	"time"

	"github.com/google/uuid"
)

type JobObject struct {
	UUID    uuid.UUID
	JobName string
	LastRun time.Time

	Cron            string
	CronWithSeconds bool

	Every interface{}

	Unit    string
	WeekDay *time.Weekday
	Months  []int

	At []interface{}
}

type JobStorage interface {
	LoadJobs() ([]JobObject, error)
}

type JobResult struct {
	UUID       uuid.UUID
	JobName    string
	RunTimes   *jobRunTimes
	ReportTime time.Time
	Err        error
}

type JobResultReporter interface {
	ReportJobResult(JobResult)
}
