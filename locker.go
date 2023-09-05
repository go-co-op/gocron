package gocron

import (
	"context"
	"errors"
)

var (
	ErrFailedToConnectToRedis = errors.New("gocron: failed to connect to redis")
	ErrFailedToObtainLock     = errors.New("gocron: failed to obtain lock")
	ErrFailedToReleaseLock    = errors.New("gocron: failed to release lock")
)

// Locker represents the required interface to lock jobs when running multiple schedulers.
type Locker interface {
	// Lock if an error is returned by lock, the job will not be scheduled.
	Lock(ctx context.Context, key string) (Lock, error)
}

// Lock represents an obtained lock
type Lock interface {
	Unlock(ctx context.Context) error
}

// Election the leader instance can run the jobs only.
type Election interface {
	// IsLeader if an error is returned by IsLeader, the job will not be scheduled.
	// IsLeader is a non-blocking implementation. it will start a thread internally to elect
	// and update the status to a variable atomically. IsLeader() returns the election status only.
	IsLeader(ctx context.Context) error
}
