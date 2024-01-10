package gocron

import (
	"time"

	"github.com/google/uuid"
)

type JobStatus string

const (
	Fail    JobStatus = "fail"
	Success JobStatus = "success"
)

// Monitorer represents the interface to collect jobs metrics.
type Monitorer interface {
	Inc(id uuid.UUID, name string, status JobStatus)
	WriteTiming(startTime, endTime time.Time, id uuid.UUID, name string)
}
