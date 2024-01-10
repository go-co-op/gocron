package gocron

import (
	"time"

	"github.com/google/uuid"
)

// JobStatus is the status of job run that should be collect
// with metric.
type JobStatus string

// The different statuses of job that can be used.
const (
	Fail    JobStatus = "fail"
	Success JobStatus = "success"
)

// Monitorer represents the interface to collect jobs metrics.
type Monitorer interface {
	Inc(id uuid.UUID, name string, tags []string, status JobStatus)
	WriteTiming(startTime, endTime time.Time, id uuid.UUID, name string, tags []string)
}
