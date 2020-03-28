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

func TestGetAt(t *testing.T) {
	j, _ := NewScheduler(time.UTC).Every(1).Minute().At("10:30").Do(task)
	assert.Equal(t, "10:30", j.GetAt())
}

func TestGetWeekday(t *testing.T) {
	j, _ := NewScheduler(time.UTC).Every(1).Weekday(time.Wednesday).Do(task)
	assert.Equal(t, time.Wednesday, j.GetWeekday())
}
