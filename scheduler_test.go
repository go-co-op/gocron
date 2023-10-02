package gocron

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduler_OneSecond_NoOptions(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

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
	t.Parallel()

	durationCh := make(chan struct{}, 10)
	durationSingletonCh := make(chan struct{}, 10)

	tests := []struct {
		name     string
		ch       chan struct{}
		jd       JobDefinition
		options  []SchedulerOption
		expected int
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
			nil,
			3,
		},
		{
			"duration",
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
			nil,
			2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, err := NewScheduler()
			require.NoError(t, err)

			_, err = s.NewJob(tt.jd)
			require.NoError(t, err)

			s.Start()
			time.Sleep(1600 * time.Millisecond)
			err = s.Stop()
			require.NoError(t, err)

			time.Sleep(1 * time.Second)
			close(tt.ch)

			var runCount int
			for range tt.ch {
				runCount++
			}

			assert.Equal(t, tt.expected, runCount)
		})
	}
}
