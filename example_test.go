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
	_, _ = gocron.NewScheduler()
}

func ExampleWithLimitConcurrentJobs() {
	_, _ = gocron.NewScheduler()
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
	_, _ = gocron.NewScheduler()
}

func ExampleWithTags() {
	_, _ = gocron.NewScheduler()
}
