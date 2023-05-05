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
