package gocron

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDurationJob(t *testing.T) {
	tests := []struct {
		name        string
		duration    time.Duration
		expectedErr *string
	}{
		{"success", time.Second, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewScheduler()
			require.NoError(t, err)

			_, err = s.NewJob(
				DurationJob(
					tt.duration,
					NewTask(
						func() {},
						nil,
					),
				),
			)
			require.NoError(t, err)

			err = s.Done()
			require.NoError(t, err)
		})
	}
}
