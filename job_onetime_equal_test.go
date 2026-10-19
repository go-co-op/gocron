package gocron

import (
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
)

func TestOneTimeJobEqualInstants(t *testing.T) {
	defer verifyNoGoroutineLeaks(t)

	now := time.Now()
	first := now.Add(24 * time.Hour)
	second := first.Add(time.Hour)
	offset := time.FixedZone("offset", 3600)
	zoneA := time.FixedZone("zone A", 0)
	zoneB := time.FixedZone("zone B", 0)
	for _, tt := range []struct {
		name  string
		times []time.Time
		want  []time.Time
	}{
		{
			name:  "different locations",
			times: []time.Time{second.UTC(), first.UTC(), first.In(offset)},
			want:  []time.Time{first.UTC(), second.UTC()},
		},
		{
			name:  "different location names with the same offset",
			times: []time.Time{first.In(zoneA), second, first.In(zoneB)},
			want:  []time.Time{first.In(zoneA), second},
		},
		{
			name:  "with and without monotonic readings",
			times: []time.Time{first, first.Round(0), second},
			want:  []time.Time{first, second},
		},
		{
			name:  "identical values",
			times: []time.Time{first, first, second, second},
			want:  []time.Time{first, second},
		},
		{
			name:  "same wall clock values at different instants",
			times: []time.Time{first.UTC(), first.Add(-time.Hour).In(offset)},
			want:  []time.Time{first.Add(-time.Hour).In(offset), first.UTC()},
		},
		{
			name:  "equivalent times at now are all excluded",
			times: []time.Time{first, now.UTC(), now.In(offset), now.Add(-time.Hour), second},
			want:  []time.Time{first, second},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScheduler(t, WithClock(clockwork.NewFakeClockAt(now)))
			t.Cleanup(func() { require.NoError(t, s.Shutdown()) })
			j, err := s.NewJob(OneTimeJob(OneTimeJobStartDateTimes(tt.times...)), NewTask(func() {}))
			require.NoError(t, err)
			require.Equal(t, tt.want, j.Schedule().(OneTimeJobSchedule).StartAt)

			s.Start()
			runs, err := j.NextRuns(len(tt.want))
			require.NoError(t, err)
			require.Len(t, runs, len(tt.want))
			for i, expected := range tt.want {
				require.True(t, runs[i].Equal(expected), "run %d: got %s, want %s", i, runs[i], expected)
			}
		})
	}
}
