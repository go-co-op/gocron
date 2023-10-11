package gocron_test

import (
	"fmt"
	"time"

	"github.com/jonboulle/clockwork"

	"github.com/go-co-op/gocron/v2"
)

func ExampleAfterJobRuns() {
	_, _ = gocron.NewScheduler()
}

func ExampleAfterJobRunsWithError() {
	_, _ = gocron.NewScheduler()
}

func ExampleBeforeJobRuns() {
	_, _ = gocron.NewScheduler()
}

func ExampleCronJob() {
	_, _ = gocron.NewScheduler()
}

func ExampleDailyJob() {
	_, _ = gocron.NewScheduler()
}

func ExampleMinuteJob() {
	_, _ = gocron.NewScheduler()
}

func ExampleDurationJob() {
	_, _ = gocron.NewScheduler()
}

func ExampleDurationRandomJob() {
	_, _ = gocron.NewScheduler()
}

func ExampleHourlyJob() {
	_, _ = gocron.NewScheduler()
}

func ExampleJob_Id() {
	_, _ = gocron.NewScheduler()
}

func ExampleJob_LastRun() {
	_, _ = gocron.NewScheduler()
}

func ExampleJob_NextRun() {
	_, _ = gocron.NewScheduler()
}

func ExampleLimitRunsTo() {
	_, _ = gocron.NewScheduler()
}

func ExampleMillisecondJob() {
	_, _ = gocron.NewScheduler()
}

func ExampleNewScheduler() {
	_, _ = gocron.NewScheduler()
}
func ExampleScheduler_NewJob() {
	s, _ := gocron.NewScheduler()
	j, err := s.NewJob(
		gocron.DurationJob(
			10*time.Second,
			gocron.NewTask(
				func() {},
			),
		),
	)
	if err != nil {
		// handle error
	}
	fmt.Println(j.Id())
}

func ExampleScheduler_RemoveByTags() {
	_, _ = gocron.NewScheduler()
}

func ExampleScheduler_RemoveJob() {
	_, _ = gocron.NewScheduler()
}

func ExampleWithShutdownTimeout() {
	_, _ = gocron.NewScheduler()
}

func ExampleScheduler_Start() {
	_, _ = gocron.NewScheduler()
}

func ExampleScheduler_Stop() {
	_, _ = gocron.NewScheduler()
}

func ExampleScheduler_Update() {
	_, _ = gocron.NewScheduler()
}

func ExampleSecondJob() {
	_, _ = gocron.NewScheduler()
}

func ExampleSingletonMode() {
	_, _ = gocron.NewScheduler()
}

func ExampleWeeklyJob() {
	_, _ = gocron.NewScheduler()
}

func ExampleWithContext() {
	_, _ = gocron.NewScheduler()
}

func ExampleWithDistributedElector() {
	_, _ = gocron.NewScheduler()
}

func ExampleWithEventListeners() {
	_, _ = gocron.NewScheduler()
}

func ExampleWithFakeClock() {
	fakeClock := clockwork.NewFakeClock()
	_, _ = gocron.NewScheduler(
		gocron.WithFakeClock(fakeClock),
	)
}

func ExampleWithGlobalJobOptions() {
	s, _ := gocron.NewScheduler(
		gocron.WithGlobalJobOptions(
			gocron.WithTags("tag1", "tag2", "tag3"),
		),
	)
	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
			gocron.NewTask(
				func(one string, two int) {
					fmt.Printf("%s, %d", one, two)
				},
				"one", 2,
			),
		),
	)
	// The job will have the globally applied tags
	fmt.Println(j.Tags())

	s2, _ := gocron.NewScheduler(
		gocron.WithGlobalJobOptions(
			gocron.WithTags("tag1", "tag2", "tag3"),
		),
	)
	j2, _ := s2.NewJob(
		gocron.DurationJob(
			time.Second,
			gocron.NewTask(
				func(one string, two int) {
					fmt.Printf("%s, %d", one, two)
				},
				"one", 2,
			),
			gocron.WithTags("tag4", "tag5", "tag6"),
		),
	)
	// The job will have the tags set specifically on the job
	// overriding those set globally by the scheduler
	fmt.Println(j2.Tags())
	// Output:
	// [tag1 tag2 tag3]
	// [tag4 tag5 tag6]
}

func ExampleWithLimitConcurrentJobs() {
	_, _ = gocron.NewScheduler(
		gocron.WithLimitConcurrentJobs(
			1,
			gocron.LimitModeReschedule,
		),
	)
}

func ExampleWithLocation() {
	location, _ := time.LoadLocation("Asia/Kolkata")

	_, _ = gocron.NewScheduler(
		gocron.WithLocation(location),
	)
}

func ExampleWithName() {
	s, _ := gocron.NewScheduler()
	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
			gocron.NewTask(
				func(one string, two int) {
					fmt.Printf("%s, %d", one, two)
				},
				"one", 2,
			),
			gocron.WithName("job 1"),
		),
	)
	fmt.Println(j.Name())
	// Output:
	// job 1
}

func ExampleWithStartDateTime() {
	s, _ := gocron.NewScheduler()
	start := time.Date(9999, 9, 9, 9, 9, 9, 9, time.UTC)
	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
			gocron.NewTask(
				func(one string, two int) {
					fmt.Printf("%s, %d", one, two)
				},
				"one", 2,
			),
			gocron.WithStartDateTime(start),
		),
	)
	next, _ := j.NextRun()
	fmt.Println(next)
	// Output:
	// 9999-09-09 09:09:09.000000009 +0000 UTC
}

func ExampleWithTags() {
	s, _ := gocron.NewScheduler()
	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
			gocron.NewTask(
				func(one string, two int) {
					fmt.Printf("%s, %d", one, two)
				},
				"one", 2,
			),
			gocron.WithTags("tag1", "tag2", "tag3"),
		),
	)
	fmt.Println(j.Tags())
	// Output:
	// [tag1 tag2 tag3]
}
