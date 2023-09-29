package gocron

import (
	"testing"
	"time"
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
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.NewJob(
				DurationJob(
					tt.duration,
					Task{
						Function:   func() {},
						Parameters: nil,
					},
				),
			)
			if err != nil {
				t.Fatal(err)
			}
			s.Start()
			s.Stop()
		})
	}
}
