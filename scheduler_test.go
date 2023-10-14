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
				NewTask(
					func() {
						cronNoOptionsCh <- struct{}{}
					},
				),
			),
		},
		{
			"duration",
			durationNoOptionsCh,
			DurationJob(
				time.Second,
				NewTask(
					func() {
						durationNoOptionsCh <- struct{}{}
					},
				),
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
			err = s.Shutdown()
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
				NewTask(
					func() {
						time.Sleep(1 * time.Second)
						durationCh <- struct{}{}
					},
				),
			),
			[]SchedulerOption{WithStopTimeout(time.Second * 2)},
			3,
		},
		{
			"duration singleton",
			durationSingletonCh,
			DurationJob(
				time.Millisecond*500,
				NewTask(
					func() {
						time.Sleep(1 * time.Second)
						durationSingletonCh <- struct{}{}
					},
				),
				WithSingletonMode(),
			),
			[]SchedulerOption{WithStopTimeout(time.Second * 5)},
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
			err = s.Shutdown()
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

func TestScheduler_Update(t *testing.T) {
	defer goleak.VerifyNone(t)

	durationJobCh := make(chan struct{})

	tests := []struct {
		name               string
		initialJob         JobDefinition
		updateJob          JobDefinition
		ch                 chan struct{}
		runCount           int
		updateAfterCount   int
		expectedMinTime    time.Duration
		expectedMaxRunTime time.Duration
	}{
		{
			"duration, updated to another duration",
			DurationJob(
				time.Millisecond*500,
				NewTask(
					func() {
						durationJobCh <- struct{}{}
					},
				),
			),
			DurationJob(
				time.Second,
				NewTask(
					func() {
						durationJobCh <- struct{}{}
					},
				),
			),
			durationJobCh,
			2,
			1,
			time.Millisecond * 1499,
			time.Second * 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewScheduler()
			require.NoError(t, err)

			j, err := s.NewJob(tt.initialJob)
			require.NoError(t, err)

			startTime := time.Now()
			s.Start()

			var runCount int
			for runCount < tt.runCount {
				select {
				case <-tt.ch:
					runCount++
					if runCount == tt.updateAfterCount {
						_, err = s.Update(j.ID(), tt.updateJob)
						require.NoError(t, err)
					}
				default:
				}
			}
			err = s.Shutdown()
			require.NoError(t, err)
			stopTime := time.Now()

			select {
			case <-tt.ch:
				t.Fatal("job ran after scheduler was stopped")
			case <-time.After(time.Millisecond * 50):
			}

			runDuration := stopTime.Sub(startTime)
			assert.GreaterOrEqual(t, runDuration, tt.expectedMinTime)
			assert.LessOrEqual(t, runDuration, tt.expectedMaxRunTime)
		})
	}
}

func TestScheduler_StopTimeout(t *testing.T) {
	defer goleak.VerifyNone(t)

	durationCh := make(chan struct{}, 10)
	durationSingletonCh := make(chan struct{}, 10)
	testDone := make(chan struct{})

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
				NewTask(
					func() {
						select {
						case <-time.After(10 * time.Second):
						case <-testDone:
						}
						durationCh <- struct{}{}
					},
				),
			),
		},
		{
			"duration singleton",
			durationSingletonCh,
			DurationJob(
				time.Millisecond*500,
				NewTask(
					func() {
						select {
						case <-time.After(10 * time.Second):
						case <-testDone:
						}

						durationSingletonCh <- struct{}{}
					},
				),
				WithSingletonMode(),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewScheduler(
				WithStopTimeout(time.Second * 1),
			)
			require.NoError(t, err)

			_, err = s.NewJob(tt.jd)
			require.NoError(t, err)

			s.Start()
			time.Sleep(time.Second)
			err = s.Shutdown()
			assert.ErrorIs(t, err, ErrStopTimedOut)
			testDone <- struct{}{}
		})
	}
}

func TestScheduler_Start_Stop_Start_Shutdown(t *testing.T) {
	goleak.VerifyNone(t)
	s, err := NewScheduler(
		WithStopTimeout(time.Second),
	)
	require.NoError(t, err)
	_, err = s.NewJob(
		DurationJob(
			5*time.Millisecond,
			NewTask(
				func() {},
			),
			WithStartAt(
				WithStartImmediately(),
			),
		),
	)
	require.NoError(t, err)

	s.Start()
	time.Sleep(5 * time.Millisecond)
	require.NoError(t, s.StopJobs())

	time.Sleep(50 * time.Millisecond)
	s.Start()

	time.Sleep(5 * time.Millisecond)
	require.NoError(t, s.Shutdown())
}
