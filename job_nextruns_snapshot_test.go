package gocron

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
)

func TestJobNextRunsDoesNotExposeSchedulerStorage(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(fmt.Sprintf("count_%d", count), func(t *testing.T) {
			now := time.Date(2026, time.November, 17, 12, 0, 0, 0, time.UTC)
			s, err := NewScheduler(WithClock(clockwork.NewFakeClockAt(now)))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, s.Shutdown()) })
			j, err := s.NewJob(DurationJob(time.Hour), NewTask(func() {}))
			require.NoError(t, err)
			s.Start()
			first, err := j.NextRuns(count)
			require.NoError(t, err)
			require.Len(t, first, count)
			expected := first[0]
			first[0] = now.Add(-time.Hour)
			next, err := j.NextRun()
			require.NoError(t, err)
			require.Equal(t, expected, next)
			second, err := j.NextRuns(count)
			require.NoError(t, err)
			require.Equal(t, expected, second[0])
		})
	}
}

func TestJobNextRunsOwnsQueuedResults(t *testing.T) {
	for _, count := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("count_%d", count), func(t *testing.T) {
			now := time.Date(2026, time.November, 17, 12, 0, 0, 0, time.UTC)
			expected := []time.Time{now.Add(time.Hour), now.Add(2 * time.Hour)}
			queued := append([]time.Time(nil), expected...)
			requests := make(chan *jobOutRequest)
			j := job{id: uuid.New(), jobOutRequest: requests}
			done := make(chan struct{})
			go func() {
				defer close(done)
				request := <-requests
				request.outChan <- internalJob{
					id:            j.id,
					jobSchedule:   &durationJob{duration: time.Hour},
					nextScheduled: queued,
				}
				close(request.outChan)
			}()
			runs, err := j.NextRuns(count)
			<-done
			require.NoError(t, err)
			require.Len(t, runs, count)
			require.Equal(t, expected[:min(count, len(expected))], runs[:min(count, len(expected))])
			if count > len(expected) {
				require.Equal(t, now.Add(3*time.Hour), runs[2])
			}
			runs[count-1] = time.Time{}
			require.Equal(t, expected, queued, "returned values must not change queued times")
		})
	}
}
