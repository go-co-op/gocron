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
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
		WithEventListeners(
			AfterJobRuns(
				func(jobID uuid.UUID, jobName string) {
					// do something after the job completes
				},
			),
		),
	)
}

func ExampleAfterJobRunsWithError() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
		WithEventListeners(
			AfterJobRunsWithError(
				func(jobID uuid.UUID, jobName string, err error) {
					// do something when the job returns an error
				},
			),
		),
	)
}

func ExampleBeforeJobRuns() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
		WithEventListeners(
			BeforeJobRuns(
				func(jobID uuid.UUID, jobName string) {
					// do something immediately before the job is run
				},
			),
		),
	)
}

func ExampleCronJob() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

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
	defer func() { _ = s.Shutdown() }()

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
	defer func() { _ = s.Shutdown() }()

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
	defer func() { _ = s.Shutdown() }()

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

func ExampleJob_id() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

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

func ExampleJob_lastRun() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

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

func ExampleJob_name() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
		WithName("foobar"),
	)

	fmt.Println(j.Name())
	// Output:
	// foobar
}

func ExampleJob_nextRun() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
	)

	nextRun, _ := j.NextRun()
	fmt.Println(nextRun)
}

func ExampleJob_nextRuns() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
	)

	nextRuns, _ := j.NextRuns(5)
	fmt.Println(nextRuns)
}

func ExampleJob_runNow() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
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
	s.Start()
	// Runs the job one time now, without impacting the schedule
	_ = j.RunNow()
}

func ExampleJob_tags() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
		WithTags("foo", "bar"),
	)

	fmt.Println(j.Tags())
	// Output:
	// [foo bar]
}

func ExampleMonthlyJob() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

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
	defer func() { _ = s.Shutdown() }()

	fmt.Println(s.Jobs())
}

func ExampleOneTimeJob() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

	// run a job once, immediately
	_, _ = s.NewJob(
		OneTimeJob(
			OneTimeJobStartImmediately(),
		),
		NewTask(
			func() {},
		),
	)
	// run a job once in 10 seconds
	_, _ = s.NewJob(
		OneTimeJob(
			OneTimeJobStartDateTime(time.Now().Add(10*time.Second)),
		),
		NewTask(
			func() {},
		),
	)

	s.Start()
}

func ExampleScheduler_jobs() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		DurationJob(
			10*time.Second,
		),
		NewTask(
			func() {},
		),
	)
	fmt.Println(len(s.Jobs()))
	// Output:
	// 1
}

func ExampleScheduler_newJob() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

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

func ExampleScheduler_removeByTags() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

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

func ExampleScheduler_removeJob() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

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

func ExampleScheduler_shutdown() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()
}

func ExampleScheduler_start() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

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

func ExampleScheduler_stopJobs() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

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

func ExampleScheduler_update() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

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
	defer func() { _ = s.Shutdown() }()

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
	//var _ gocron.Elector = (*myElector)(nil)
	//
	//type myElector struct{}
	//
	//func (m myElector) IsLeader(_ context.Context) error {
	//	return nil
	//}
	//
	//elector := &myElector{}
	//
	//_, _ = gocron.NewScheduler(
	//	gocron.WithDistributedElector(elector),
	//)
}

func ExampleWithDistributedLocker() {
	//var _ gocron.Locker = (*myLocker)(nil)
	//
	//type myLocker struct{}
	//
	//func (m myLocker) Lock(ctx context.Context, key string) (Lock, error) {
	//	return &testLock, nil
	//}
	//
	//var _ Lock = (*testLock)(nil)
	//
	//type testLock struct {
	//}
	//
	//func (t testLock) Unlock(_ context.Context) error {
	//	return nil
	//}
	//
	//locker := &myLocker{}
	//
	//_, _ = gocron.NewScheduler(
	//	gocron.WithDistributedLocker(locker),
	//)
}

func ExampleWithEventListeners() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		DurationJob(
			time.Second,
		),
		NewTask(
			func() {},
		),
		WithEventListeners(
			AfterJobRuns(
				func(jobID uuid.UUID, jobName string) {
					// do something after the job completes
				},
			),
			AfterJobRunsWithError(
				func(jobID uuid.UUID, jobName string, err error) {
					// do something when the job returns an error
				},
			),
			BeforeJobRuns(
				func(jobID uuid.UUID, jobName string) {
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
	defer func() { _ = s.Shutdown() }()

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
	_ = s.StopJobs()
	fmt.Printf("no jobs in scheduler: %v\n", s.Jobs())
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

func ExampleWithLogger() {
	_, _ = NewScheduler(
		WithLogger(
			NewLogger(LogLevelDebug),
		),
	)
}

func ExampleWithMonitor() {
	//type exampleMonitor struct {
	//	mu      sync.Mutex
	//	counter map[string]int
	//	time    map[string][]time.Duration
	//}
	//
	//func newExampleMonitor() *exampleMonitor {
	//	return &exampleMonitor{
	//	counter: make(map[string]int),
	//	time:    make(map[string][]time.Duration),
	//}
	//}
	//
	//func (t *exampleMonitor) IncrementJob(_ uuid.UUID, name string, _ []string, _ JobStatus) {
	//	t.mu.Lock()
	//	defer t.mu.Unlock()
	//	_, ok := t.counter[name]
	//	if !ok {
	//		t.counter[name] = 0
	//	}
	//	t.counter[name]++
	//}
	//
	//func (t *exampleMonitor) RecordJobTiming(startTime, endTime time.Time, _ uuid.UUID, name string, _ []string) {
	//	t.mu.Lock()
	//	defer t.mu.Unlock()
	//	_, ok := t.time[name]
	//	if !ok {
	//		t.time[name] = make([]time.Duration, 0)
	//	}
	//	t.time[name] = append(t.time[name], endTime.Sub(startTime))
	//}
	//
	//monitor := newExampleMonitor()
	//s, _ := NewScheduler(
	//	WithMonitor(monitor),
	//)
	//name := "example"
	//_, _ = s.NewJob(
	//	DurationJob(
	//		time.Second,
	//	),
	//	NewTask(
	//		func() {
	//			time.Sleep(1 * time.Second)
	//		},
	//	),
	//	WithName(name),
	//	WithStartAt(
	//		WithStartImmediately(),
	//	),
	//)
	//s.Start()
	//time.Sleep(5 * time.Second)
	//_ = s.Shutdown()
	//
	//fmt.Printf("Job %q total execute count: %d\n", name, monitor.counter[name])
	//for i, val := range monitor.time[name] {
	//	fmt.Printf("Job %q execute #%d elapsed %.4f seconds\n", name, i+1, val.Seconds())
	//}
}

func ExampleWithName() {
	s, _ := NewScheduler()
	defer func() { _ = s.Shutdown() }()

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
	defer func() { _ = s.Shutdown() }()

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
	defer func() { _ = s.Shutdown() }()

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

	next, _ := j.NextRun()
	fmt.Println(next)

	_ = s.StopJobs()
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
	defer func() { _ = s.Shutdown() }()

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
