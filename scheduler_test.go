package gocron

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduler_Cron_Start_Stop(t *testing.T) {
	s, err := NewScheduler()
	require.NoError(t, err)

	jobRan := make(chan struct{})

	id, err := s.NewJob(
		CronJob(
			"* * * * * *",
			true,
			Task{
				Function: func() {
					close(jobRan)
				},
			},
		),
	)
	require.NoError(t, err)

	s.Start()

	select {
	case <-jobRan:
	case <-time.After(2 * time.Second):
		t.Errorf("job failed to run")
	}

	lastRun, err := s.GetJobLastRun(id)
	assert.NotZero(t, lastRun)
	err = s.Stop()
	assert.NoError(t, err)
}
