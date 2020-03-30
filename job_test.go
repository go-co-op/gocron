package gocron

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTags(t *testing.T) {
	j, _ := NewScheduler(time.UTC).Every(1).Minute().Do(task)
	j.Tag("some")
	j.Tag("tag")
	j.Tag("more")
	j.Tag("tags")

	assert.ElementsMatch(t, j.Tags(), []string{"tags", "tag", "more", "some"})

	j.Untag("more")
	assert.ElementsMatch(t, j.Tags(), []string{"tags", "tag", "some"})
}

func TestGetScheduledTime(t *testing.T) {
	j, _ := NewScheduler(time.UTC).Every(1).Minute().At("10:30").Do(task)
	assert.Equal(t, "10:30", j.GetScheduledTime())
}

func TestGetWeekday(t *testing.T) {
	s := NewScheduler(time.UTC)
	weedayJob, _ := s.Every(1).Weekday(time.Wednesday).Do(task)
	nonWeekdayJob, _ := s.Every(1).Minute().Do(task)

	testCases := []struct {
		desc            string
		job             *Job
		expectedWeekday time.Weekday
		expectedError   error
	}{
		{"success", weedayJob, time.Wednesday, nil},
		{"fail - not set for weekday", nonWeekdayJob, time.Sunday, ErrNotScheduledWeekday},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			weekday, err := tc.job.GetWeekday()
			if tc.expectedError != nil {
				assert.Error(t, tc.expectedError, err)
			} else {
				assert.Nil(t, err)
			}

			assert.Equal(t, tc.expectedWeekday, weekday)
		})
	}
}
