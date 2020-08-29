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
	assert.Equal(t, "10:30", j.ScheduledAtTime())
}

func TestGetWeekday(t *testing.T) {
	s := NewScheduler(time.UTC)
	wednesday := time.Wednesday
	weedayJob, _ := s.Every(1).Weekday(wednesday).Do(task)
	nonWeekdayJob, _ := s.Every(1).Minute().Do(task)

	testCases := []struct {
		desc            string
		job             *Job
		expectedWeekday *time.Weekday
		expectedError   error
	}{
		{"success", weedayJob, &wednesday, nil},
		{"fail - not set for weekday", nonWeekdayJob, nil, ErrNotScheduledWeekday},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			weekday, err := tc.job.Weekday()
			if tc.expectedError != nil {
				assert.Error(t, tc.expectedError, err)
			} else {
				assert.Equal(t, tc.expectedWeekday, weekday)
				assert.Nil(t, err)
			}
		})
	}
}

func TestSetPeriodDuration(t *testing.T) {

	testCases := []struct {
		desc             string
		job              *Job
		expectedDuration time.Duration
		expectedError    error
	}{
		{"seconds", &Job{interval: 1, unit: seconds}, time.Duration(1) * time.Second, nil},
		{"minutes", &Job{interval: 1, unit: minutes}, time.Duration(1) * time.Minute, nil},
		{"hours", &Job{interval: 1, unit: hours}, time.Duration(1) * time.Hour, nil},
		{"days", &Job{interval: 1, unit: days}, time.Duration(1) * time.Hour * 24, nil},
		{"weeks", &Job{interval: 1, unit: weeks}, time.Duration(1) * time.Hour * 24 * 7, nil},
		{"none", &Job{interval: 1}, 0, ErrPeriodNotSpecified},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			err := tc.job.setPeriodDuration()
			assert.Equal(t, tc.expectedError, err)
			assert.Equal(t, tc.expectedDuration, tc.job.periodDuration)
		})
	}

}
