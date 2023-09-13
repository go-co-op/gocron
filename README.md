# gocron: A Golang Job Scheduling Package.

[![Mentioned in Awesome Go](https://awesome.re/mentioned-badge-flat.svg)](https://github.com/avelino/awesome-go#job-scheduler)
[![CI State](https://github.com/go-co-op/gocron/actions/workflows/go_test.yml/badge.svg?branch=main&event=push)](https://github.com/go-co-op/gocron/actions)
![Go Report Card](https://goreportcard.com/badge/github.com/go-co-op/gocron) [![Go Doc](https://godoc.org/github.com/go-co-op/gocron?status.svg)](https://pkg.go.dev/github.com/go-co-op/gocron)

gocron is a job scheduling package which lets you run Go functions at pre-determined intervals 
using a simple, human-friendly syntax.

gocron is a Golang scheduler implementation similar to the Ruby module 
[clockwork](https://github.com/tomykaira/clockwork) and the Python job scheduling package [schedule](https://github.com/dbader/schedule).

See also these two great articles that were used for design input:

- [Rethinking Cron](http://adam.herokuapp.com/past/2010/4/13/rethinking_cron/)
- [Replace Cron with Clockwork](http://adam.herokuapp.com/past/2010/6/30/replace_cron_with_clockwork/)

If you want to chat, you can find us at Slack! 
[<img src="https://img.shields.io/badge/gophers-gocron-brightgreen?logo=slack">](https://gophers.slack.com/archives/CQ7T0T1FW)

## Concepts

- **Scheduler**: The scheduler tracks all the jobs assigned to it and makes sure they are passed to the executor when
  ready to be run. The scheduler is able to manage overall aspects of job behavior like limiting how many jobs 
  are running at one time.
- **Job**: The job is simply aware of the task (go function) it's provided and is therefore only able to perform
  actions related to that task like preventing itself from overruning a previous task that is taking a long time.
- **Executor**: The executor, as it's name suggests, is simply responsible for calling the task (go function) that
  the job hands to it when sent by the scheduler.

## Examples

```golang
s := gocron.NewScheduler(time.UTC)

// Every starts the job immediately and then runs at the 
// specified interval
job, err := s.Every(5).Seconds().Do(func(){ ... })
if err != nil {
	// handle the error related to setting up the job
}

// to wait for the interval to pass before running the first job
// use WaitForSchedule or WaitForScheduleAll
s.Every(5).Second().WaitForSchedule().Do(func(){ ... })

s.WaitForScheduleAll()
s.Every(5).Second().Do(func(){ ... }) // waits for schedule
s.Every(5).Second().Do(func(){ ... }) // waits for schedule

// strings parse to duration
s.Every("5m").Do(func(){ ... })

s.Every(5).Days().Do(func(){ ... })

s.Every(1).Month(1, 2, 3).Do(func(){ ... })

// set time
s.Every(1).Day().At("10:30").Do(func(){ ... })

// set multiple times
s.Every(1).Day().At("10:30;08:00").Do(func(){ ... })

s.Every(1).Day().At("10:30").At("08:00").Do(func(){ ... })

// Schedule each last day of the month
s.Every(1).MonthLastDay().Do(func(){ ... })

// Or each last day of every other month
s.Every(2).MonthLastDay().Do(func(){ ... })

// cron expressions supported
s.Cron("*/1 * * * *").Do(task) // every minute

// cron second-level expressions supported
s.CronWithSeconds("*/1 * * * * *").Do(task) // every second

// you can start running the scheduler in two different ways:
// starts the scheduler asynchronously
s.StartAsync()
// starts the scheduler and blocks current execution path
s.StartBlocking()

// stop the running scheduler in two different ways:
// stop the scheduler
s.Stop()

// stop the scheduler and notify the `StartBlocking()` to exit
s.StopBlockingChan()
```

For more examples, take a look in our [go docs](https://pkg.go.dev/github.com/go-co-op/gocron#pkg-examples)

## Options

| Interval     | Supported schedule options                                          |
| ------------ | ------------------------------------------------------------------- |
| sub-second   | `StartAt()`                                                         |
| milliseconds | `StartAt()`                                                         |
| seconds      | `StartAt()`                                                         |
| minutes      | `StartAt()`                                                         |
| hours        | `StartAt()`                                                         |
| days         | `StartAt()`, `At()`                                                 |
| weeks        | `StartAt()`, `At()`, `Weekday()` (and all week day named functions) |
| months       | `StartAt()`, `At()`                                                 |

There are several options available to restrict how jobs run:

| Mode                | Function                  | Behavior                                                                                             |
|---------------------|---------------------------|------------------------------------------------------------------------------------------------------|
| Default             |                           | jobs are rescheduled at every interval                                                               |
| Job singleton       | `SingletonMode()`         | a long running job will not be rescheduled until the current run is completed                        |
| Scheduler limit     | `SetMaxConcurrentJobs()`  | set a collective maximum number of concurrent jobs running across the scheduler                      |
| Distributed locking | `WithDistributedLocker()` | prevents the same job from being run more than once when running multiple instances of the scheduler |
| Distributed elector | `WithDistributedElector()` | multiple instances exist in a distributed scenario, only the leader instance can run jobs  |

## Distributed Locker Implementations

- Redis: [redislock](https://github.com/go-co-op/gocron-redis-lock) `go get github.com/go-co-op/gocron-redis-lock`

## Tags

Jobs may have arbitrary tags added which can be useful when tracking many jobs.
The scheduler supports both enforcing tags to be unique and when not unique,
running all jobs with a given tag.

```golang
s := gocron.NewScheduler(time.UTC)
s.TagsUnique()

_, _ = s.Every(1).Week().Tag("foo").Do(task)
_, err := s.Every(1).Week().Tag("foo").Do(task)
// error!!!

s := gocron.NewScheduler(time.UTC)

s.Every(2).Day().Tag("tag").At("10:00").Do(task)
s.Every(1).Minute().Tag("tag").Do(task)
s.RunByTag("tag")
// both jobs will run
```

## FAQ

- Q: I'm running multiple pods on a distributed environment. How can I make a job not run once per pod causing duplication?
  - We recommend using your own lock solution within the jobs themselves (you could use [Redis](https://redis.io/topics/distlock), for example)
  - A2: Use the scheduler option `WithDistributedLocker` and either use an implemented [backend](#distributed-locker-implementations)
    or implement your own and contribute it back in a PR!

- Q: I've removed my job from the scheduler, but how can I stop a long-running job that has already been triggered?
  - A: We recommend using a means of canceling your job, e.g. a `context.WithCancel()`.
  - A2: You can listen to the job context Done channel to know when the job has been canceled
    ```golang
    task := func(in string, job gocron.Job) {
        fmt.Printf("this job's last run: %s this job's next run: %s\n", job.LastRun(), job.NextRun())
        fmt.Printf("in argument is %s\n", in)

        ticker := time.NewTicker(100 * time.Millisecond)
        defer ticker.Stop()

        for {
            select {
            case <-job.Context().Done():
                fmt.Printf("function has been canceled, performing cleanup and exiting gracefully\n")
                return
            case <-ticker.C:
                fmt.Printf("performing a hard job that takes a long time that I want to kill whenever I want\n")
            }
        }
    }

    var err error
    s := gocron.NewScheduler(time.UTC)
    s.SingletonModeAll()
    j, err := s.Every(1).Hour().Tag("myJob").DoWithJobDetails(task, "foo")
    if err != nil {
        log.Fatalln("error scheduling job", err)
    }

    s.StartAsync()

    // Simulate some more work
    time.Sleep(time.Second)

    // I want to stop the job, together with the underlying goroutine
    fmt.Printf("now I want to kill the job\n")
    err = s.RemoveByTag("myJob")
    if err != nil {
        log.Fatalln("error removing job by tag", err)
    }

    // Wait a bit so that we can see that the job is exiting gracefully
    time.Sleep(time.Second)
    fmt.Printf("Job: %#v, Error: %#v", j, err)
    ```

---

Looking to contribute? Try to follow these guidelines:

- Use issues for everything
- For a small change, just send a PR!
- For bigger changes, please open an issue for discussion before sending a PR.
- PRs should have: tests, documentation and examples (if it makes sense)
- You can also contribute by:
  - Reporting issues
  - Suggesting new features or enhancements
  - Improving/fixing documentation

---

## Design

![design-diagram](https://user-images.githubusercontent.com/19351306/110375142-2ba88680-8017-11eb-80c3-554cc746b165.png)

[Jetbrains](https://www.jetbrains.com/?from=gocron) supports this project with GoLand licenses. We appreciate their support for free and open source software!

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=go-co-op/gocron&type=Date)](https://star-history.com/#go-co-op/gocron&Date)


