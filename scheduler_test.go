package gocron

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestScheduler_OneSecond_NoOptions(t *testing.T) {
	defer goleak.VerifyNone(t)
	cronNoOptionsCh := make(chan struct{})
	durationNoOptionsCh := make(chan struct{})

	tests := []struct {
		name string
		ch   chan struct{}
		jd   JobDefinition
	}{
		{
			"cron",
			cronNoOptionsCh,
			CronJob(
				"* * * * * *",
				true,
				Task{
					Function: func() {
						cronNoOptionsCh <- struct{}{}
					},
				},
			),
		},
		{
			"duration",
			durationNoOptionsCh,
			DurationJob(
				time.Second,
				Task{
					Function: func() {
						durationNoOptionsCh <- struct{}{}
					},
				},
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewScheduler()
			require.NoError(t, err)

			_, err = s.NewJob(tt.jd)
			require.NoError(t, err)

			s.Start()

			startTime := time.Now()
			var runCount int
			for runCount < 1 {
				<-tt.ch
				runCount++
			}
			err = s.Stop()
			require.NoError(t, err)
			stopTime := time.Now()

			select {
			case <-tt.ch:
				t.Fatal("job ran after scheduler was stopped")
			case <-time.After(time.Millisecond * 50):
			}

			runDuration := stopTime.Sub(startTime)
			assert.GreaterOrEqual(t, runDuration, time.Millisecond)
			assert.LessOrEqual(t, runDuration, 1500*time.Millisecond)
		})
	}
}

func TestScheduler_LongRunningJobs(t *testing.T) {
	defer goleak.VerifyNone(t)

	durationCh := make(chan struct{}, 10)
	durationSingletonCh := make(chan struct{}, 10)

	tests := []struct {
		name         string
		ch           chan struct{}
		jd           JobDefinition
		options      []SchedulerOption
		expectedRuns int
	}{
		{
			"duration",
			durationCh,
			DurationJob(
				time.Millisecond*500,
				Task{
					Function: func() {
						time.Sleep(1 * time.Second)
						durationCh <- struct{}{}
					},
				},
			),
			[]SchedulerOption{WithShutdownTimeout(time.Second * 2)},
			3,
		},
		{
			"duration singleton",
			durationSingletonCh,
			DurationJob(
				time.Millisecond*500,
				Task{
					Function: func() {
						time.Sleep(1 * time.Second)
						durationSingletonCh <- struct{}{}
					},
				},
				SingletonMode(),
			),
			[]SchedulerOption{WithShutdownTimeout(time.Second * 5)},
			2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewScheduler(tt.options...)
			require.NoError(t, err)

			_, err = s.NewJob(tt.jd)
			require.NoError(t, err)

			s.Start()
			time.Sleep(1600 * time.Millisecond)
			err = s.Stop()
			require.NoError(t, err)

			var runCount int
			timeout := make(chan struct{})
			go func() {
				time.Sleep(2 * time.Second)
				close(timeout)
			}()
		Outer:
			for {
				select {
				case <-tt.ch:
					runCount++
				case <-timeout:
					break Outer
				}
			}

			assert.Equal(t, tt.expectedRuns, runCount)
		})
	}
}

func TestScheduler_StopTimeout(t *testing.T) {
	// We expect goroutines to leak here because we timed-out
	// and one or both of the go routines waiting for singleton
	// runner and job wait group never returned.
	// defer goleak.VerifyNone(t)

	durationCh := make(chan struct{}, 10)
	durationSingletonCh := make(chan struct{}, 10)

	tests := []struct {
		name string
		ch   chan struct{}
		jd   JobDefinition
	}{
		{
			"duration",
			durationCh,
			DurationJob(
				time.Millisecond*500,
				Task{
					Function: func() {
						time.Sleep(10 * time.Second)
						durationCh <- struct{}{}
					},
				},
			),
		},
		{
			"duration singleton",
			durationSingletonCh,
			DurationJob(
				time.Millisecond*500,
				Task{
					Function: func() {
						time.Sleep(10 * time.Second)
						durationSingletonCh <- struct{}{}
					},
				},
				SingletonMode(),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewScheduler(
				WithShutdownTimeout(time.Second * 1),
			)
			require.NoError(t, err)

			_, err = s.NewJob(tt.jd)
			require.NoError(t, err)

			s.Start()
			time.Sleep(time.Second)
			err = s.Stop()
			assert.ErrorIs(t, err, ErrStopTimedOut)
		})
	}
}
