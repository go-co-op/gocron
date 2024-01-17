package gocron

import (
	"time"

	"github.com/google/uuid"
)

// JobStatus is the status of job run that should be collected with the metric.
type JobStatus string

// The different statuses of job that can be used.
const (
	Fail    JobStatus = "fail"
	Success JobStatus = "success"
)

// Monitor represents the interface to collect jobs metrics.
type Monitor interface {
	JobRunInc(id uuid.UUID, name string, tags []string, status JobStatus)
	JobRunTiming(startTime, endTime time.Time, id uuid.UUID, name string, tags []string)
}
