package gocron_test

import (
	"fmt"
	"sync"
	"time"

	. "github.com/go-co-op/gocron/v2" // nolint:revive
	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
)

func ExampleAfterJobRuns() {
	s, _ := NewScheduler()

	_, _ = s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
		WithEventListeners(
			AfterJobRuns(
				func(jobID uuid.UUID) {
					// do something after the job completes
				},
			),
		),
	)
}

func ExampleAfterJobRunsWithError() {
	s, _ := NewScheduler()

	_, _ = s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
		WithEventListeners(
			AfterJobRunsWithError(
				func(jobID uuid.UUID, err error) {
					// do something when the job returns an error
				},
			),
		),
	)
}

func ExampleBeforeJobRuns() {
	s, _ := NewScheduler()

	_, _ = s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
		WithEventListeners(
			BeforeJobRuns(
				func(jobID uuid.UUID) {
					// do something immediately before the job is run
				},
			),
		),
	)
}

func ExampleCronJob() {
	s, _ := NewScheduler()

	_, _ = s.NewJob(
		CronJob(
			// standard cron tab parsing
			"1 * * * *",
			false,
		),
		NewTask(
			func() {},
		),
	)
	_, _ = s.NewJob(
		CronJob(
			// optionally include seconds as the first field
			"* 1 * * * *",
			true,
		),
		NewTask(
			func() {},
		),
	)
}

func ExampleDailyJob() {
	s, _ := NewScheduler()

	_, _ = s.NewJob(
		DailyJob(
			1,
			NewAtTimes(
				NewAtTime(10, 30, 0),
				NewAtTime(14, 0, 0),
			),
		),
		NewTask(
			func(a, b string) {},
			"a",
			"b",
		),
	)
}

func ExampleDurationJob() {
	s, _ := NewScheduler()

	_, _ = s.NewJob(
		DurationJob(
			time.Second*5,
		),
		NewTask(
			func() {},
		),
	)
}

func ExampleDurationRandomJob() {
	s, _ := NewScheduler()

	_, _ = s.NewJob(
		DurationRandomJob(
			time.Second,
			5*time.Second,
		),
		NewTask(
			func() {},
		),
	)
}

func ExampleJob_ID() {
	s, _ := NewScheduler()

	j, _ := s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
	)

	fmt.Println(j.ID())
}

func ExampleJob_LastRun() {
	s, _ := NewScheduler()

	j, _ := s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
	)

	fmt.Println(j.LastRun())
}

func ExampleJob_NextRun() {
	s, _ := NewScheduler()

	j, _ := s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
	)

	fmt.Println(j.NextRun())
}

func ExampleMonthlyJob() {
	s, _ := NewScheduler()

	_, _ = s.NewJob(
		MonthlyJob(
			1,
			NewDaysOfTheMonth(3, -5, -1),
			NewAtTimes(
				NewAtTime(10, 30, 0),
				NewAtTime(11, 15, 0),
			),
		),
		NewTask(
			func() {},
		),
	)
}

func ExampleNewScheduler() {
	s, _ := NewScheduler()

	fmt.Println(s.Jobs())
}

func ExampleScheduler_NewJob() {
	s, _ := NewScheduler()

	j, err := s.NewJob(
		DurationJob(
			10*time.Second,
		),
		NewTask(
			func() {},
		),
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(j.ID())
}

func ExampleScheduler_RemoveByTags() {
	s, _ := NewScheduler()

	_, _ = s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
		WithTags("tag1"),
	)
	_, _ = s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
		WithTags("tag2"),
	)
	fmt.Println(len(s.Jobs()))

	s.RemoveByTags("tag1", "tag2")

	fmt.Println(len(s.Jobs()))
	// Output:
	// 2
	// 0
}

func ExampleScheduler_RemoveJob() {
	s, _ := NewScheduler()

	j, _ := s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
	)

	fmt.Println(len(s.Jobs()))

	_ = s.RemoveJob(j.ID())

	fmt.Println(len(s.Jobs()))
	// Output:
	// 1
	// 0
}

func ExampleScheduler_Start() {
	s, _ := NewScheduler()

	_, _ = s.NewJob(
		CronJob(
			"* * * * *",
			false,
		),
		NewTask(
			func() {},
		),
	)

	s.Start()
}

func ExampleScheduler_StopJobs() {
	s, _ := NewScheduler()

	_, _ = s.NewJob(
		CronJob(
			"* * * * *",
			false,
		),
		NewTask(
			func() {},
		),
	)

	s.Start()

	_ = s.StopJobs()
}

func ExampleScheduler_Update() {
	s, _ := NewScheduler()

	j, _ := s.NewJob(
		CronJob(
			"* * * * *",
			false,
		),
		NewTask(
			func() {},
		),
	)

	s.Start()

	// after some time, need to change the job

	j, _ = s.Update(
		j.ID(),
		DurationJob(
			5*time.Second,
		),
		NewTask(
			func() {},
		),
	)
}

func ExampleWeeklyJob() {
	s, _ := NewScheduler()

	_, _ = s.NewJob(
		WeeklyJob(
			2,
			NewWeekdays(time.Tuesday, time.Wednesday, time.Saturday),
			NewAtTimes(
				NewAtTime(1, 30, 0),
				NewAtTime(12, 0, 30),
			),
		),
		NewTask(
			func() {},
		),
	)
}

func ExampleWithClock() {
	fakeClock := clockwork.NewFakeClock()
	s, _ := NewScheduler(
		WithClock(fakeClock),
	)
	var wg sync.WaitGroup
	wg.Add(1)
	_, _ = s.NewJob(
		DurationJob(
			time.Second*5,
		),
		NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d\n", one, two)
				wg.Done()
			},
			"one", 2,
		),
	)
	s.Start()
	fakeClock.BlockUntil(1)
	fakeClock.Advance(time.Second * 5)
	wg.Wait()
	_ = s.StopJobs()
	// Output:
	// one, 2
}

func ExampleWithDistributedElector() {
	//var _ Elector = (*myElector)(nil)
	//
	//type myElector struct{}
	//
	//func (m myElector) IsLeader(_ context.Context) error {
	//	return nil
	//}
	//
	//elector := myElector{}
	//
	//_, _ = NewScheduler(
	//	WithDistributedElector(elector),
	//)
}

func ExampleWithEventListeners() {
	s, _ := NewScheduler()

	_, _ = s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
		WithEventListeners(
			AfterJobRuns(
				func(jobID uuid.UUID) {
					// do something after the job completes
				},
			),
			AfterJobRunsWithError(
				func(jobID uuid.UUID, err error) {
					// do something when the job returns an error
				},
			),
			BeforeJobRuns(
				func(jobID uuid.UUID) {
					// do something immediately before the job is run
				},
			),
		),
	)
}

func ExampleWithGlobalJobOptions() {
	s, _ := NewScheduler(
		WithGlobalJobOptions(
			WithTags("tag1", "tag2", "tag3"),
		),
	)

	j, _ := s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d", one, two)
			},
			"one", 2,
		),
	)
	// The job will have the globally applied tags
	fmt.Println(j.Tags())

	s2, _ := NewScheduler(
		WithGlobalJobOptions(
			WithTags("tag1", "tag2", "tag3"),
		),
	)
	j2, _ := s2.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d", one, two)
			},
			"one", 2,
		),
		WithTags("tag4", "tag5", "tag6"),
	)
	// The job will have the tags set specifically on the job
	// overriding those set globally by the scheduler
	fmt.Println(j2.Tags())
	// Output:
	// [tag1 tag2 tag3]
	// [tag4 tag5 tag6]
}

func ExampleWithLimitConcurrentJobs() {
	_, _ = NewScheduler(
		WithLimitConcurrentJobs(
			1,
			LimitModeReschedule,
		),
	)
}

func ExampleWithLimitedRuns() {
	s, _ := NewScheduler()

	_, _ = s.NewJob(
		DurationJob(
			time.Millisecond,
		),
		NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d\n", one, two)
			},
			"one", 2,
		),
		WithLimitedRuns(1),
	)
	s.Start()

	time.Sleep(100 * time.Millisecond)
	fmt.Printf("no jobs in scheduler: %v\n", s.Jobs())
	_ = s.StopJobs()
	// Output:
	// one, 2
	// no jobs in scheduler: []
}

func ExampleWithLocation() {
	location, _ := time.LoadLocation("Asia/Kolkata")

	_, _ = NewScheduler(
		WithLocation(location),
	)
}

func ExampleWithName() {
	s, _ := NewScheduler()

	j, _ := s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d", one, two)
			},
			"one", 2,
		),
		WithName("job 1"),
	)
	fmt.Println(j.Name())
	// Output:
	// job 1
}

func ExampleWithSingletonMode() {
	s, _ := NewScheduler()

	_, _ = s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {
				// this job will skip half it's executions
				// and effectively run every 2 seconds
				time.Sleep(1500 * time.Second)
			},
		),
		WithSingletonMode(LimitModeReschedule),
	)
}

func ExampleWithStartAt() {
	s, _ := NewScheduler()

	start := time.Date(9999, 9, 9, 9, 9, 9, 9, time.UTC)

	j, _ := s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d", one, two)
			},
			"one", 2,
		),
		WithStartAt(
			WithStartDateTime(start),
		),
	)
	s.Start()
	defer func() {
		_ = s.StopJobs()
	}()
	time.Sleep(10 * time.Millisecond)
	next, _ := j.NextRun()
	fmt.Println(next)
	// Output:
	// 9999-09-09 09:09:09.000000009 +0000 UTC
}

func ExampleWithStopTimeout() {
	_, _ = NewScheduler(
		WithStopTimeout(time.Second * 5),
	)
}

func ExampleWithTags() {
	s, _ := NewScheduler()

	j, _ := s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d", one, two)
			},
			"one", 2,
		),
		WithTags("tag1", "tag2", "tag3"),
	)
	fmt.Println(j.Tags())
	// Output:
	// [tag1 tag2 tag3]
}
