package gocron

import (
	"context"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
)

func callJobFuncWithParams(jobFunc any, params ...any) error {
	if jobFunc == nil {
		return nil
	}
	f := reflect.ValueOf(jobFunc)
	if f.IsZero() {
		return nil
	}
	if len(params) != f.Type().NumIn() {
		return ErrJobParameterMismatch
	}
	in := make([]reflect.Value, len(params))
	for k, param := range params {
		in[k] = reflect.ValueOf(param)
	}
	returnValues := f.Call(in)
	for _, val := range returnValues {
		i := val.Interface()
		if err, ok := i.(error); ok {
			return err
		}
	}
	return nil
}

// requestJob resolves a Job.X() accessor's request against the
// scheduler goroutine. Returns:
//
//   - (nil, ErrSchedulerBusy) if the scheduler didn't respond within
//     defaultRequestJobTimeout. Callers should surface this to users
//     rather than reporting ErrJobNotFound, so users can distinguish
//     "gone" from "temporarily unreachable."
//   - (&internalJob{}, nil) with id == uuid.Nil when the scheduler
//     responded but the id isn't registered. Callers translate this
//     to ErrJobNotFound.
//   - (&j, nil) with a populated job on success.
func requestJob(id uuid.UUID, ch chan *jobOutRequest) (*internalJob, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultRequestJobTimeout)
	defer cancel()
	ij := requestJobCtx(ctx, id, ch)
	if ij == nil {
		return nil, ErrSchedulerBusy
	}
	return ij, nil
}

func requestJobCtx(ctx context.Context, id uuid.UUID, ch chan *jobOutRequest) *internalJob {
	resp := make(chan internalJob, 1)
	select {
	case ch <- &jobOutRequest{
		id:      id,
		outChan: resp,
	}:
	case <-ctx.Done():
		return nil
	}
	var j internalJob
	select {
	case <-ctx.Done():
		return nil
	case jobReceived := <-resp:
		j = jobReceived
	}
	return &j
}

func removeSliceDuplicatesInt(in []int) []int {
	slices.Sort(in)
	return slices.Compact(in)
}

func convertAtTimesToDateTime(atTimes AtTimes, location *time.Location) ([]time.Time, error) {
	if atTimes == nil {
		return nil, errAtTimesNil
	}
	var atTimesDate []time.Time
	for _, a := range atTimes() {
		if a == nil {
			return nil, errAtTimeNil
		}
		at := a()
		if at.hours > 23 {
			return nil, errAtTimeHours
		} else if at.minutes > 59 || at.seconds > 59 {
			return nil, errAtTimeMinSec
		}
		atTimesDate = append(atTimesDate, at.time(location))
	}
	slices.SortStableFunc(atTimesDate, ascendingTime)
	return atTimesDate, nil
}

func ascendingTime(a, b time.Time) int {
	return a.Compare(b)
}

// insertNextScheduled inserts t into times while preserving the
// ascending-time invariant that Job.NextRun and Job.NextRuns rely on.
// The caller must ensure times is already sorted ascending; this is
// enforced by construction because every mutation site goes through
// this helper. Uses ascendingTime (time.Time.Compare) so ordering is
// determined by wall-clock instant regardless of monotonic readings.
func insertNextScheduled(times []time.Time, t time.Time) []time.Time {
	idx, _ := slices.BinarySearchFunc(times, t, ascendingTime)
	return slices.Insert(times, idx, t)
}

// nextScheduledContains reports whether times (sorted ascending)
// contains a value at the same wall-clock instant as t, ignoring any
// difference in monotonic readings. Callers relying on slices.Contains
// would miss such matches because Go's == on time.Time also compares
// monotonic components.
func nextScheduledContains(times []time.Time, t time.Time) bool {
	_, found := slices.BinarySearchFunc(times, t, ascendingTime)
	return found
}

// waitGroupWithMutex wraps sync.WaitGroup so that Add() cannot race
// with Wait(). This addresses the classic "Add called after Wait
// returned (or with counter == 0)" panic that sync.WaitGroup enforces:
// gocron dispatches new work concurrently with executor shutdown, and
// the mutex serializes the two.
//
// Invariant the mutex protects: an Add() call is never concurrent with
// a Wait() call. Done() intentionally does NOT take the mutex, because
// sync.WaitGroup.Done is already safe to call while Wait is blocked
// (that's the entire point of Wait). Adding the lock to Done would
// deadlock any goroutine that Waits on this group from inside
// wg-tracked work.
type waitGroupWithMutex struct {
	wg sync.WaitGroup
	mu sync.Mutex
}

func (w *waitGroupWithMutex) Add(delta int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.wg.Add(delta)
}

func (w *waitGroupWithMutex) Done() {
	w.wg.Done()
}

func (w *waitGroupWithMutex) Wait() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.wg.Wait()
}
