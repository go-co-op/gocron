package gocron

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
)

type observedDoneContext struct {
	context.Context

	observed chan struct{}
	once     sync.Once
}

func (c *observedDoneContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

func TestExecutor_LimitModeRunnerCancellation(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	tests := []struct {
		name           string
		cancelExecutor bool
		cancelJob      bool
	}{
		{"job cancellation", false, true},
		{"executor cancellation", true, false},
		{"normal rescheduling", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			jobCtx, cancelJob := context.WithCancel(context.Background())
			defer cancelJob()
			observed := make(chan struct{})
			j := internalJob{
				id:            uuid.New(),
				ctx:           &observedDoneContext{Context: jobCtx, observed: observed},
				singletonMode: true,
			}
			in := make(chan jobIn)
			limiter := make(chan struct{}, 1)
			limiter <- struct{}{}
			e := executor{
				ctx:                    ctx,
				clock:                  clockwork.NewRealClock(),
				logger:                 noOpLogger{},
				jobOutRequest:          make(chan *jobOutRequest),
				jobsOutForRescheduling: make(chan uuid.UUID),
				jobsOutCompleted:       make(chan jobOutCompleted, 1),
				jobTimingUpdateCh:      make(chan jobTimingUpdate, 2),
				limitMode: &limitModeConfig{
					singletonJobs: map[uuid.UUID]struct{}{j.id: {}},
				},
			}
			wg := &waitGroupWithMutex{}
			wg.Add(1)
			exited := make(chan struct{})
			go func() {
				e.limitModeRunner("test", in, wg, LimitModeReschedule, limiter)
				close(exited)
			}()
			defer func() {
				cancel()
				<-exited
			}()

			in <- jobIn{id: j.id, shouldSendOut: true}
			request := <-e.jobOutRequest
			request.outChan <- j
			// Wait until the duplicate singleton reaches the rescheduling select.
			<-observed
			if tt.cancelExecutor {
				cancel()
			} else {
				if tt.cancelJob {
					cancelJob()
				} else {
					require.Equal(t, j.id, <-e.jobsOutForRescheduling)
				}
				next := internalJob{id: uuid.New(), ctx: context.Background()}
				ran := make(chan struct{})
				next.function = func() { close(ran) }
				select {
				case in <- jobIn{id: next.id}:
				case <-exited:
					t.Fatal("canceling one job retired the shared runner")
				case <-time.After(time.Second):
					t.Fatal("runner did not accept another job")
				}
				require.Empty(t, limiter, "skipped job retained its limiter slot")
				limiter <- struct{}{}
				request = <-e.jobOutRequest
				request.outChan <- next
				select {
				case <-ran:
				case <-time.After(time.Second):
					t.Fatal("runner did not execute another job")
				}
				cancel()
			}
			<-exited
			waited := make(chan struct{})
			go func() {
				wg.Wait()
				close(waited)
			}()
			select {
			case <-waited:
			case <-time.After(time.Second):
				t.Fatal("runner exited without completing its wait group")
			}
			require.Empty(t, limiter, "runner exited without releasing its limiter slot")
		})
	}
}
