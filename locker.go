package gocron

// Locker provides an interface for implementing job locking
// to prevent jobs from running at the same time on multiple
// instances of gocron
type Locker interface {
	Lock(key string) (bool, error)
	Unlock(key string) error
}

var (
	locker Locker
)

// SetLocker sets a locker implementation to be used by
// the scheduler for locking jobs
func SetLocker(l Locker) {
	locker = l
}
