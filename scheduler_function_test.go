package gocron

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestScheduler_TaskFunctionPointer(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)
	for depth := range 3 {
		t.Run(fmt.Sprintf("depth=%d", depth), func(t *testing.T) {
			s := newTestScheduler(t)
			defer func() { require.NoError(t, s.Shutdown()) }()
			result := make(chan string, 1)
			fn := func(ctx context.Context, value string) {
				if err := ctx.Err(); err != nil {
					result <- err.Error()
					return
				}
				result <- value
			}
			pointer := &fn
			var taskFunc any = fn
			switch depth {
			case 1:
				taskFunc = pointer
			case 2:
				taskFunc = &pointer
			}
			_, err := s.NewJob(DurationJob(time.Hour), NewTask(taskFunc, "original"),
				WithStartAt(WithStartImmediately()),
				WithEventListeners(AfterJobRunsWithPanic(func(_ uuid.UUID, _ string, recovered any) {
					result <- fmt.Sprintf("panic: %v", recovered)
				})))
			require.NoError(t, err)
			s.Start()
			select {
			case got := <-result:
				require.Equal(t, "original", got)
			case <-time.After(time.Second):
				t.Fatal("task function did not execute")
			}
		})
	}
}
