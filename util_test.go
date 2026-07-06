package gocron

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRemoveSliceDuplicatesInt(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		expected []int
	}{
		{
			"lots of duplicates",
			[]int{
				1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
				2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2,
				3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
				4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4,
				5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5,
			},
			[]int{1, 2, 3, 4, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeSliceDuplicatesInt(tt.input)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestCallJobFuncWithParams(t *testing.T) {
	type f1 func()
	tests := []struct {
		name        string
		jobFunc     any
		params      []any
		expectedErr error
	}{
		{
			"nil jobFunc",
			nil,
			nil,
			nil,
		},
		{
			"zero jobFunc",
			f1(nil),
			nil,
			nil,
		},
		{
			"wrong number of params",
			func(_ string, _ int) {},
			[]any{"one"},
			ErrJobParameterMismatch,
		},
		{
			"function that returns an error",
			func() error {
				return errors.New("test error")
			},
			nil,
			errors.New("test error"),
		},
		{
			"function that returns no error",
			func() error {
				return nil
			},
			nil,
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := callJobFuncWithParams(tt.jobFunc, tt.params...)
			assert.Equal(t, tt.expectedErr, err)
		})
	}
}

func TestConvertAtTimesToDateTime(t *testing.T) {
	tests := []struct {
		name     string
		atTimes  AtTimes
		location *time.Location
		expected []time.Time
		err      error
	}{
		{
			"atTimes is nil",
			nil,
			time.UTC,
			nil,
			errAtTimesNil,
		},
		{
			"atTime is nil",
			NewAtTimes(nil),
			time.UTC,
			nil,
			errAtTimeNil,
		},
		{
			"atTimes hours is invalid",
			NewAtTimes(
				NewAtTime(24, 0, 0),
			),
			time.UTC,
			nil,
			errAtTimeHours,
		},
		{
			"atTimes minutes are invalid",
			NewAtTimes(
				NewAtTime(0, 60, 0),
			),
			time.UTC,
			nil,
			errAtTimeMinSec,
		},
		{
			"atTimes seconds are invalid",
			NewAtTimes(
				NewAtTime(0, 0, 60),
			),
			time.UTC,
			nil,
			errAtTimeMinSec,
		},
		{
			"atTimes valid",
			NewAtTimes(
				NewAtTime(0, 0, 3),
				NewAtTime(0, 0, 0),
				NewAtTime(0, 0, 1),
				NewAtTime(0, 0, 2),
			),
			time.UTC,
			[]time.Time{
				time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC),
				time.Date(0, 0, 0, 0, 0, 1, 0, time.UTC),
				time.Date(0, 0, 0, 0, 0, 2, 0, time.UTC),
				time.Date(0, 0, 0, 0, 0, 3, 0, time.UTC),
			},
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertAtTimesToDateTime(tt.atTimes, tt.location)
			assert.Equal(t, tt.expected, result)
			assert.Equal(t, tt.err, err)
		})
	}
}

func TestInsertNextScheduled(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		start  []time.Time
		insert time.Time
		want   []time.Time
	}{
		{
			name:   "empty slice",
			start:  nil,
			insert: base,
			want:   []time.Time{base},
		},
		{
			name:   "insert at start",
			start:  []time.Time{base.Add(time.Hour), base.Add(2 * time.Hour)},
			insert: base,
			want:   []time.Time{base, base.Add(time.Hour), base.Add(2 * time.Hour)},
		},
		{
			name:   "insert in middle",
			start:  []time.Time{base, base.Add(2 * time.Hour)},
			insert: base.Add(time.Hour),
			want:   []time.Time{base, base.Add(time.Hour), base.Add(2 * time.Hour)},
		},
		{
			name:   "insert at end",
			start:  []time.Time{base, base.Add(time.Hour)},
			insert: base.Add(2 * time.Hour),
			want:   []time.Time{base, base.Add(time.Hour), base.Add(2 * time.Hour)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// make a defensive copy so table entries stay pristine across sub-tests
			start := append([]time.Time(nil), tt.start...)
			got := insertNextScheduled(start, tt.insert)
			assert.Equal(t, tt.want, got)

			// verify sorted invariant
			for i := 1; i < len(got); i++ {
				assert.False(t, got[i].Before(got[i-1]),
					"result must be sorted ascending: idx %d (%v) < idx %d (%v)",
					i, got[i], i-1, got[i-1])
			}
		})
	}
}

func TestInsertNextScheduled_MonotonicVsWallClock(t *testing.T) {
	// A time.Time from time.Now() carries a monotonic reading; the
	// same instant reconstructed via time.Date does not. Go's ==
	// operator considers these unequal, but ascendingTime (which
	// uses time.Compare) treats them as equal instants. Confirm the
	// helper places the "same instant" value adjacent to its twin
	// rather than at the wrong end of the slice.
	nowMonotonic := time.Now()
	sameInstantNoMono := time.Unix(0, nowMonotonic.UnixNano()).In(nowMonotonic.Location())

	got := insertNextScheduled([]time.Time{sameInstantNoMono}, nowMonotonic)
	assert.Len(t, got, 2)
	// both entries should represent the same wall-clock instant
	assert.True(t, got[0].Equal(got[1]),
		"the two entries should represent the same wall-clock instant")
}

func TestNextScheduledContains(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	slice := []time.Time{base, base.Add(time.Hour), base.Add(2 * time.Hour)}

	assert.True(t, nextScheduledContains(slice, base))
	assert.True(t, nextScheduledContains(slice, base.Add(time.Hour)))
	assert.True(t, nextScheduledContains(slice, base.Add(2*time.Hour)))
	assert.False(t, nextScheduledContains(slice, base.Add(30*time.Minute)))
	assert.False(t, nextScheduledContains(nil, base))
}

func TestNextScheduledContains_MonotonicMismatch(t *testing.T) {
	// slices.Contains uses ==, which compares monotonic readings.
	// nextScheduledContains uses time.Compare via ascendingTime, so
	// it correctly treats two representations of the same instant
	// as a match. This is a correctness fix over the previous
	// slices.Contains-based duplicate detection.
	nowMonotonic := time.Now()
	sameInstantNoMono := time.Unix(0, nowMonotonic.UnixNano()).In(nowMonotonic.Location())

	assert.True(t, nextScheduledContains([]time.Time{sameInstantNoMono}, nowMonotonic),
		"instants are equal per time.Compare; helper must report contains=true")
}
