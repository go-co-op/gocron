package gocron

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduler(t *testing.T) {
	t.Parallel()

	cronNoOptionsCh := make(chan struct{})
	durationNoOptionsCh := make(chan struct{})

	type testJob struct {
		name string
		ch   chan struct{}
		jd   JobDefinition
	}

	tests := []struct {
		name            string
		testJobs        []testJob
		options         []SchedulerOption
		runCount        int
		expectedTimeMin time.Duration
		expectedTimeMax time.Duration
	}{
		{
			"no scheduler options 1 second jobs",
			[]testJob{
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
			},
			nil,

			1,
			time.Millisecond * 1,
			time.Millisecond * 1500,
		},
	}

	for _, tt := range tests {

		for _, tj := range tt.testJobs {
			t.Run(tt.name+"_"+tj.name, func(t *testing.T) {
				t.Parallel()

				s, err := NewScheduler(tt.options...)
				require.NoError(t, err)

				_, err = s.NewJob(tj.jd)
				require.NoError(t, err)

				s.Start()

				startTime := time.Now()
				var runCount int
				for runCount < tt.runCount {
					<-tj.ch
					runCount++
				}
				err = s.Stop()
				require.NoError(t, err)
				stopTime := time.Now()

				select {
				case <-tj.ch:
					t.Fatal("job ran after scheduler was stopped")
				case <-time.After(time.Millisecond * 50):
				}

				runDuration := stopTime.Sub(startTime)
				assert.GreaterOrEqual(t, runDuration, tt.expectedTimeMin)
				assert.LessOrEqual(t, runDuration, tt.expectedTimeMax)

			})
		}
	}
}

// TODO tests for singleton mode and other limit mode
// need to handle case where some jobs are waiting for on shutdown
// so can't use a channel because then the job is blocked trying to send
