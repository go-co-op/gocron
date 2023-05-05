package gocron

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/redis/go-redis/v9"
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

// NewRedisLocker provides an implementation of the Locker interface using
// redis for storage.
func NewRedisLocker(r *redis.Client) (Locker, error) {
	if err := r.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", ErrFailedToConnectToRedis, err)
	}
	pool := goredis.NewPool(r)
	rs := redsync.New(pool)
	return &redisLocker{rs: rs}, nil
}

var _ Locker = (*redisLocker)(nil)

type redisLocker struct {
	rs *redsync.Redsync
}

func (r *redisLocker) Lock(ctx context.Context, key string) (Lock, error) {
	mu := r.rs.NewMutex(key, redsync.WithTries(1))
	err := mu.LockContext(ctx)
	if err != nil {
		return nil, ErrFailedToObtainLock
	}
	rl := &redisLock{
		mu: mu,
	}
	return rl, nil
}

var _ Lock = (*redisLock)(nil)

type redisLock struct {
	mu *redsync.Mutex
}

func (r *redisLock) Unlock(ctx context.Context) error {
	unlocked, err := r.mu.UnlockContext(ctx)
	if err != nil {
		return ErrFailedToReleaseLock
	}
	if !unlocked {
		return ErrFailedToReleaseLock
	}

	return nil
}
