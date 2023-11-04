package gocron

import (
	"fmt"
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
			func(one string, two int) {},
			[]any{"one"},
			nil,
		},
		{
			"function that returns an error",
			func() error {
				return fmt.Errorf("test error")
			},
			nil,
			fmt.Errorf("test error"),
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
