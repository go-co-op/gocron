package gocron_test

import (
	"testing"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestWithTagsInputDoesNotChangeRegisteredJobs(t *testing.T) {
	for _, update := range []bool{false, true} {
		name := "NewJob"
		if update {
			name = "Update"
		}
		t.Run(name, func(t *testing.T) {
			s, err := gocron.NewScheduler()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, s.Shutdown()) })
			tags := []string{"keep"}
			j, err := s.NewJob(gocron.DurationJob(time.Hour), gocron.NewTask(func() {}), gocron.WithTags(tags...))
			require.NoError(t, err)
			if update {
				j, err = s.Update(j.ID(), gocron.DurationJob(time.Hour), gocron.NewTask(func() {}), gocron.WithTags(tags...))
				require.NoError(t, err)
			}
			other, err := s.NewJob(gocron.DurationJob(time.Hour), gocron.NewTask(func() {}), gocron.WithTags("remove"))
			require.NoError(t, err)

			// Reuse the caller's input storage only after registration returns.
			tags[0] = "remove"
			require.Equal(t, []string{"keep"}, j.Tags())
			for _, registered := range s.Jobs() {
				if registered.ID() == j.ID() {
					require.Equal(t, []string{"keep"}, registered.Tags())
				}
			}
			s.RemoveByTags("remove")
			remaining := s.Jobs()
			require.Len(t, remaining, 1)
			require.Equal(t, j.ID(), remaining[0].ID())
			require.NotEqual(t, other.ID(), remaining[0].ID())
		})
	}
}

func TestWithTagsSharedOptionInputDoesNotChangeJobs(t *testing.T) {
	tags := []string{"shared"}
	s, err := gocron.NewScheduler(gocron.WithGlobalJobOptions(gocron.WithTags(tags...)))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Shutdown()) })
	ids := make(map[uuid.UUID]bool)
	for range 2 {
		j, addErr := s.NewJob(gocron.DurationJob(time.Hour), gocron.NewTask(func() {}))
		require.NoError(t, addErr)
		ids[j.ID()] = true
	}
	tags[0] = "unrelated"
	s.RemoveByTags("unrelated")
	jobs := s.Jobs()
	require.Len(t, jobs, 2)
	for _, j := range jobs {
		require.True(t, ids[j.ID()])
		require.Equal(t, []string{"shared"}, j.Tags())
	}
}

func TestWithTagsPreservesValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		tags []string
	}{
		{name: "nil"},
		{name: "empty", tags: []string{}},
		{name: "ordered duplicates", tags: []string{"second", "first", "second"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := gocron.NewScheduler()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, s.Shutdown()) })
			j, err := s.NewJob(gocron.DurationJob(time.Hour), gocron.NewTask(func() {}), gocron.WithTags(tc.tags...))
			require.NoError(t, err)
			require.Equal(t, tc.tags, j.Tags())
		})
	}
}
