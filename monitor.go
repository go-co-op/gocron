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
	Skip    JobStatus = "skip"
)

// Monitor represents the interface to collect jobs metrics.
type Monitor interface {
	// IncrementJob will provide details about the job and expects the underlying implementation
	// to handle instantiating and incrementing a value
	IncrementJob(id uuid.UUID, name string, tags []string, status JobStatus)
	// RecordJobTiming will provide details about the job and the timing and expects the underlying implementation
	// to handle instantiating and recording the value
	RecordJobTiming(startTime, endTime time.Time, id uuid.UUID, name string, tags []string)
}
