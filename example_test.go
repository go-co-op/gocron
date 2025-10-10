package gocron_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
)

func ExampleAfterJobRuns() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func() {},
		),
		gocron.WithEventListeners(
			gocron.AfterJobRuns(
				func(jobID uuid.UUID, jobName string) {
					// do something after the job completes
				},
			),
		),
	)
}

func ExampleAfterJobRunsWithError() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func() {},
		),
		gocron.WithEventListeners(
			gocron.AfterJobRunsWithError(
				func(jobID uuid.UUID, jobName string, err error) {
					// do something when the job returns an error
				},
			),
		),
	)
}

var _ gocron.Locker = new(errorLocker)

type errorLocker struct{}

func (e errorLocker) Lock(_ context.Context, _ string) (gocron.Lock, error) {
	return nil, errors.New("locked")
}

func ExampleAfterLockError() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func() {},
		),
		gocron.WithDistributedJobLocker(&errorLocker{}),
		gocron.WithEventListeners(
			gocron.AfterLockError(
				func(jobID uuid.UUID, jobName string, err error) {
					// do something immediately before the job is run
				},
			),
		),
	)
}

func ExampleBeforeJobRuns() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func() {},
		),
		gocron.WithEventListeners(
			gocron.BeforeJobRuns(
				func(jobID uuid.UUID, jobName string) {
					// do something immediately before the job is run
				},
			),
		),
	)
}

func ExampleBeforeJobRunsSkipIfBeforeFuncErrors() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func() {
				fmt.Println("Will never run, because before job func errors")
			},
		),
		gocron.WithEventListeners(
			gocron.BeforeJobRunsSkipIfBeforeFuncErrors(
				func(jobID uuid.UUID, jobName string) error {
					return errors.New("error")
				},
			),
		),
	)
}

func ExampleCronJob() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.CronJob(
			// standard cron tab parsing
			"1 * * * *",
			false,
		),
		gocron.NewTask(
			func() {},
		),
	)
	_, _ = s.NewJob(
		gocron.CronJob(
			// optionally include seconds as the first field
			"* 1 * * * *",
			true,
		),
		gocron.NewTask(
			func() {},
		),
	)
}

func ExampleDailyJob() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.DailyJob(
			1,
			gocron.NewAtTimes(
				gocron.NewAtTime(10, 30, 0),
				gocron.NewAtTime(14, 0, 0),
			),
		),
		gocron.NewTask(
			func(a, b string) {},
			"a",
			"b",
		),
	)
}

func ExampleDurationJob() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.DurationJob(
			time.Second*5,
		),
		gocron.NewTask(
			func() {},
		),
	)
}

func ExampleDurationRandomJob() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.DurationRandomJob(
			time.Second,
			5*time.Second,
		),
		gocron.NewTask(
			func() {},
		),
	)
}

func ExampleJob_id() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func() {},
		),
	)

	fmt.Println(j.ID())
}

func ExampleJob_lastRun() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func() {},
		),
	)

	fmt.Println(j.LastRun())
}

func ExampleJob_name() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func() {},
		),
		gocron.WithName("foobar"),
	)

	fmt.Println(j.Name())
	// Output:
	// foobar
}

func ExampleJob_nextRun() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func() {},
		),
	)

	nextRun, _ := j.NextRun()
	fmt.Println(nextRun)
}

func ExampleJob_nextRuns() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func() {},
		),
	)

	nextRuns, _ := j.NextRuns(5)
	fmt.Println(nextRuns)
}

func ExampleJob_runNow() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		gocron.MonthlyJob(
			1,
			gocron.NewDaysOfTheMonth(3, -5, -1),
			gocron.NewAtTimes(
				gocron.NewAtTime(10, 30, 0),
				gocron.NewAtTime(11, 15, 0),
			),
		),
		gocron.NewTask(
			func() {},
		),
	)
	s.Start()
	// Runs the job one time now, without impacting the schedule
	_ = j.RunNow()
}

func ExampleJob_tags() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func() {},
		),
		gocron.WithTags("foo", "bar"),
	)

	fmt.Println(j.Tags())
	// Output:
	// [foo bar]
}

func ExampleMonthlyJob() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.MonthlyJob(
			1,
			gocron.NewDaysOfTheMonth(3, -5, -1),
			gocron.NewAtTimes(
				gocron.NewAtTime(10, 30, 0),
				gocron.NewAtTime(11, 15, 0),
			),
		),
		gocron.NewTask(
			func() {},
		),
	)
}

func ExampleNewScheduler() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	fmt.Println(s.Jobs())
}

func ExampleNewTask() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.DurationJob(time.Second),
		gocron.NewTask(
			func(ctx context.Context) {
				// gocron will pass in a context (either the default Job context, or one
				// provided via WithContext) to the job and will cancel the context on shutdown.
				// This allows you to listen for and handle cancellation within your job.
			},
		),
	)
}

func ExampleOneTimeJob() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	// run a job once, immediately
	_, _ = s.NewJob(
		gocron.OneTimeJob(
			gocron.OneTimeJobStartImmediately(),
		),
		gocron.NewTask(
			func() {},
		),
	)
	// run a job once in 10 seconds
	_, _ = s.NewJob(
		gocron.OneTimeJob(
			gocron.OneTimeJobStartDateTime(time.Now().Add(10*time.Second)),
		),
		gocron.NewTask(
			func() {},
		),
	)
	// run job twice - once in 10 seconds and once in 55 minutes
	n := time.Now()
	_, _ = s.NewJob(
		gocron.OneTimeJob(
			gocron.OneTimeJobStartDateTimes(
				n.Add(10*time.Second),
				n.Add(55*time.Minute),
			),
		),
		gocron.NewTask(func() {}),
	)

	s.Start()
}

func ExampleScheduler_jobs() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.DurationJob(
			10*time.Second,
		),
		gocron.NewTask(
			func() {},
		),
	)
	fmt.Println(len(s.Jobs()))
	// Output:
	// 1
}

func ExampleScheduler_newJob() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, err := s.NewJob(
		gocron.DurationJob(
			10*time.Second,
		),
		gocron.NewTask(
			func() {},
		),
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(j.ID())
}

func ExampleScheduler_removeByTags() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func() {},
		),
		gocron.WithTags("tag1"),
	)
	_, _ = s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func() {},
		),
		gocron.WithTags("tag2"),
	)
	fmt.Println(len(s.Jobs()))

	s.RemoveByTags("tag1", "tag2")

	fmt.Println(len(s.Jobs()))
	// Output:
	// 2
	// 0
}

func ExampleScheduler_removeJob() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
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
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()
}

func ExampleScheduler_start() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.CronJob(
			"* * * * *",
			false,
		),
		gocron.NewTask(
			func() {},
		),
	)

	s.Start()
}

func ExampleScheduler_stopJobs() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.CronJob(
			"* * * * *",
			false,
		),
		gocron.NewTask(
			func() {},
		),
	)

	s.Start()

	_ = s.StopJobs()
}

func ExampleScheduler_update() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		gocron.CronJob(
			"* * * * *",
			false,
		),
		gocron.NewTask(
			func() {},
		),
	)

	s.Start()

	// after some time, need to change the job

	j, _ = s.Update(
		j.ID(),
		gocron.DurationJob(
			5*time.Second,
		),
		gocron.NewTask(
			func() {},
		),
	)
}

func ExampleWeeklyJob() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.WeeklyJob(
			2,
			gocron.NewWeekdays(time.Tuesday, time.Wednesday, time.Saturday),
			gocron.NewAtTimes(
				gocron.NewAtTime(1, 30, 0),
				gocron.NewAtTime(12, 0, 30),
			),
		),
		gocron.NewTask(
			func() {},
		),
	)
}

func ExampleWithClock() {
	fakeClock := clockwork.NewFakeClock()
	s, _ := gocron.NewScheduler(
		gocron.WithClock(fakeClock),
	)
	var wg sync.WaitGroup
	wg.Add(1)
	_, _ = s.NewJob(
		gocron.DurationJob(
			time.Second*5,
		),
		gocron.NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d\n", one, two)
				wg.Done()
			},
			"one", 2,
		),
	)
	s.Start()
	_ = fakeClock.BlockUntilContext(context.Background(), 1)
	fakeClock.Advance(time.Second * 5)
	wg.Wait()
	_ = s.StopJobs()
	// Output:
	// one, 2
}

func ExampleWithContext() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, _ = s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func(ctx context.Context) {
				// gocron will pass in the context provided via WithContext
				// to the job and will cancel the context on shutdown.
				// This allows you to listen for and handle cancellation within your job.
			},
		),
		gocron.WithContext(ctx),
	)
}

var _ gocron.Cron = (*customCron)(nil)

type customCron struct{}

func (c customCron) IsValid(crontab string, location *time.Location, now time.Time) error {
	return nil
}

func (c customCron) Next(lastRun time.Time) time.Time {
	return time.Now().Add(time.Second)
}

func ExampleWithCronImplementation() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()
	_, _ = s.NewJob(
		gocron.CronJob(
			"* * * * *",
			false,
		),
		gocron.NewTask(
			func() {},
		),
		gocron.WithCronImplementation(
			&customCron{},
		),
	)
}

func ExampleWithDisabledDistributedJobLocker() {
	// var _ gocron.Locker = (*myLocker)(nil)
	//
	// type myLocker struct{}
	//
	// func (m myLocker) Lock(ctx context.Context, key string) (Lock, error) {
	//     return &testLock{}, nil
	// }
	//
	// var _ gocron.Lock = (*testLock)(nil)
	//
	// type testLock struct{}
	//
	// func (t testLock) Unlock(_ context.Context) error {
	//     return nil
	// }

	locker := &myLocker{}

	s, _ := gocron.NewScheduler(
		gocron.WithDistributedLocker(locker),
	)

	_, _ = s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func() {},
		),
		gocron.WithDisabledDistributedJobLocker(true),
	)
}

var _ gocron.Elector = (*myElector)(nil)

type myElector struct{}

func (m myElector) IsLeader(_ context.Context) error {
	return nil
}

func ExampleWithDistributedElector() {
	// var _ gocron.Elector = (*myElector)(nil)
	//
	// type myElector struct{}
	//
	// func (m myElector) IsLeader(_ context.Context) error {
	//     return nil
	// }
	//
	elector := &myElector{}

	_, _ = gocron.NewScheduler(
		gocron.WithDistributedElector(elector),
	)
}

var _ gocron.Locker = (*myLocker)(nil)

type myLocker struct{}

func (m myLocker) Lock(ctx context.Context, key string) (gocron.Lock, error) {
	return &testLock{}, nil
}

var _ gocron.Lock = (*testLock)(nil)

type testLock struct{}

func (t testLock) Unlock(_ context.Context) error {
	return nil
}

func ExampleWithDistributedLocker() {
	// var _ gocron.Locker = (*myLocker)(nil)
	//
	// type myLocker struct{}
	//
	// func (m myLocker) Lock(ctx context.Context, key string) (Lock, error) {
	//     return &testLock{}, nil
	// }
	//
	// var _ gocron.Lock = (*testLock)(nil)
	//
	// type testLock struct{}
	//
	// func (t testLock) Unlock(_ context.Context) error {
	//     return nil
	// }

	locker := &myLocker{}

	_, _ = gocron.NewScheduler(
		gocron.WithDistributedLocker(locker),
	)
}

func ExampleWithEventListeners() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func() {},
		),
		gocron.WithEventListeners(
			gocron.AfterJobRuns(
				func(jobID uuid.UUID, jobName string) {
					// do something after the job completes
				},
			),
			gocron.AfterJobRunsWithError(
				func(jobID uuid.UUID, jobName string, err error) {
					// do something when the job returns an error
				},
			),
			gocron.BeforeJobRuns(
				func(jobID uuid.UUID, jobName string) {
					// do something immediately before the job is run
				},
			),
		),
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
		),
		gocron.NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d", one, two)
			},
			"one", 2,
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
		),
		gocron.NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d", one, two)
			},
			"one", 2,
		),
		gocron.WithTags("tag4", "tag5", "tag6"),
	)
	// The job will have the tags set specifically on the job
	// overriding those set globally by the scheduler
	fmt.Println(j2.Tags())
	// Output:
	// [tag1 tag2 tag3]
	// [tag4 tag5 tag6]
}

func ExampleWithIdentifier() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d", one, two)
			},
			"one", 2,
		),
		gocron.WithIdentifier(uuid.MustParse("87b95dfc-3e71-11ef-9454-0242ac120002")),
	)
	fmt.Println(j.ID())
	// Output:
	// 87b95dfc-3e71-11ef-9454-0242ac120002
}

func ExampleWithLimitConcurrentJobs() {
	_, _ = gocron.NewScheduler(
		gocron.WithLimitConcurrentJobs(
			1,
			gocron.LimitModeReschedule,
		),
	)
}

func ExampleWithLimitedRuns() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.DurationJob(
			time.Millisecond,
		),
		gocron.NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d\n", one, two)
			},
			"one", 2,
		),
		gocron.WithLimitedRuns(1),
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

	_, _ = gocron.NewScheduler(
		gocron.WithLocation(location),
	)
}

func ExampleWithLogger() {
	_, _ = gocron.NewScheduler(
		gocron.WithLogger(
			gocron.NewLogger(gocron.LogLevelDebug),
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
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d", one, two)
			},
			"one", 2,
		),
		gocron.WithName("job 1"),
	)
	fmt.Println(j.Name())
	// Output:
	// job 1
}

func ExampleWithSingletonMode() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	_, _ = s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func() {
				// this job will skip half it's executions
				// and effectively run every 2 seconds
				time.Sleep(1500 * time.Second)
			},
		),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
}

func ExampleWithStartAt() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	start := time.Date(9999, 9, 9, 9, 9, 9, 9, time.UTC)

	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d", one, two)
			},
			"one", 2,
		),
		gocron.WithStartAt(
			gocron.WithStartDateTime(start),
		),
	)
	s.Start()

	next, _ := j.NextRun()
	fmt.Println(next)

	_ = s.StopJobs()
	// Output:
	// 9999-09-09 09:09:09.000000009 +0000 UTC
}

func ExampleWithStartDateTime() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	start := time.Date(9999, 9, 9, 9, 9, 9, 9, time.UTC)

	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d", one, two)
			},
			"one", 2,
		),
		gocron.WithStartAt(
			gocron.WithStartDateTime(start),
		),
	)
	s.Start()

	next, _ := j.NextRun()
	fmt.Println(next)

	_ = s.StopJobs()
	// Output:
	// 9999-09-09 09:09:09.000000009 +0000 UTC
}

func ExampleWithStartDateTimePast() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	start := time.Now().Add(-time.Minute)

	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d", one, two)
			},
			"one", 2,
		),
		gocron.WithStartAt(
			gocron.WithStartDateTimePast(start),
		),
	)
	s.Start()

	time.Sleep(100 * time.Millisecond)

	_, _ = j.NextRun()

	_ = s.StopJobs()
}

func ExampleWithStartImmediately() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d", one, two)
			},
			"one", 2,
		),
		gocron.WithStartAt(
			gocron.WithStartImmediately(),
		),
	)
	s.Start()

	_, _ = j.NextRun()

	_ = s.StopJobs()
}

func ExampleWithStopTimeout() {
	_, _ = gocron.NewScheduler(
		gocron.WithStopTimeout(time.Second * 5),
	)
}

func ExampleWithTags() {
	s, _ := gocron.NewScheduler()
	defer func() { _ = s.Shutdown() }()

	j, _ := s.NewJob(
		gocron.DurationJob(
			time.Second,
		),
		gocron.NewTask(
			func(one string, two int) {
				fmt.Printf("%s, %d", one, two)
			},
			"one", 2,
		),
		gocron.WithTags("tag1", "tag2", "tag3"),
	)
	fmt.Println(j.Tags())
	// Output:
	// [tag1 tag2 tag3]
}
