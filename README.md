# gocron: A Golang Job Scheduling Package.

[![CI State](https://github.com/go-co-op/gocron/actions/workflows/go_test.yml/badge.svg?branch=v2&event=push)](https://github.com/go-co-op/gocron/actions)
![Go Report Card](https://goreportcard.com/badge/github.com/go-co-op/gocron) [![Go Doc](https://godoc.org/github.com/go-co-op/gocron/v2?status.svg)](https://pkg.go.dev/github.com/go-co-op/gocron/v2)

gocron is a job scheduling package which lets you run Go functions at pre-determined intervals.

If you want to chat, you can find us on Slack at
[<img src="https://img.shields.io/badge/gophers-gocron-brightgreen?logo=slack">](https://gophers.slack.com/archives/CQ7T0T1FW)

## Concepts

- **Job**: The encapsulates a "task", which is made up of a go func and any function parameters, and then
  provides the scheduler with the time the job should be scheduled to run.
- **Executor**: The executor, calls the "task" function and manages the complexities of different job
  execution timing (e.g. singletons that shouldn't overrun each other, limiting the max number of jobs running)
- **Scheduler**: The scheduler keeps track of all the jobs and sends each job to the executor when
  it is ready to be run.

## Quick Start

```
go get github.com/go-co-op/gocron/v2
```

```golang
package main

import (
	"fmt"
	"time"

	"github.com/go-co-op/gocron/v2"
)

func main() {
	// create a scheduler
	s, err := gocron.NewScheduler()
	if err != nil {
		// handle error
	}

	// add a job to the scheduler
	j, err := s.NewJob(
		gocron.DurationJob(
			10*time.Second,
		),
		gocron.NewTask(
			func(a string, b int) {
				// do things
            },
			"hello",
			1,
		),
	)
	if err != nil {
		// handle error
	}
	// each job has a unique id
	fmt.Println(j.ID())

	// start the scheduler
	s.Start()

	// when you're done, shut it down
	err = s.Shutdown()
	if err != nil {
		// handle error
	}
}
```

## Features

- **Job types**: Jobs can be run at various intervals.
  - [**Duration**](https://pkg.go.dev/github.com/go-co-op/gocron/v2#DurationJob):
    Jobs can be run at a fixed `time.Duration`.
  - [**Random duration**](https://pkg.go.dev/github.com/go-co-op/gocron/v2#DurationRandomJob):
    Jobs can be run at a random `time.Duration` between a min and max.
  - [**Cron**](https://pkg.go.dev/github.com/go-co-op/gocron/v2#CronJob):
    Jobs can be run using a crontab.
  - [**Daily**](https://pkg.go.dev/github.com/go-co-op/gocron/v2#DailyJob):
    Jobs can be run every x days at specific times.
  - [**Weekly**](https://pkg.go.dev/github.com/go-co-op/gocron/v2#WeeklyJob):
    Jobs can be run every x weeks on specific days of the week and at specific times.
  - [**Monthly**](https://pkg.go.dev/github.com/go-co-op/gocron/v2#MonthlyJob):
    Jobs can be run every x months on specific days of the month and at specific times.
- **Limited Concurrency**: Jobs can be limited individually or across the entire scheduler.
  - [**Per job limiting with singleton mode**](https://pkg.go.dev/github.com/go-co-op/gocron/v2#WithSingletonMode):
    Jobs can be limited to a single concurrent execution that either reschedules (skips overlapping executions)
    or queues (waits for the previous execution to finish).
  - [**Per scheduler limiting with limit mode**](https://pkg.go.dev/github.com/go-co-op/gocron/v2#WithLimitConcurrentJobs):
    Jobs can be limited to a certain number of concurrent executions across the entire scheduler
    using either reschedule (skip when the limit is met) or queue (jobs are added to a queue to
    wait for the limit to be available).
- **Distributed instances of gocron**: Multiple instances of gocron can be run.
  - [**Elector**](https://pkg.go.dev/github.com/go-co-op/gocron/v2#WithDistributedElector):
    An elector can be used to elect a single instance of gocron to run as the primary with the
    other instances checking to see if a new leader needs to be elected.
    - Implementations: [go-co-op electors](https://github.com/go-co-op?q=-elector&type=all&language=&sort=)
  - [**Locker**](https://pkg.go.dev/github.com/go-co-op/gocron/v2#WithDistributedLocker):
    A locker can be used to lock each run of a job to a single instance of gocron.
    - Implementations: [go-co-op lockers](https://github.com/go-co-op?q=-lock&type=all&language=&sort=)
- **Events**: Job events can trigger actions.
  - [**Listeners**](https://pkg.go.dev/github.com/go-co-op/gocron/v2#WithEventListeners):
    [Event listeners](https://pkg.go.dev/github.com/go-co-op/gocron/v2#EventListener)
    can be added to a job or all jobs in the
    [scheduler](https://pkg.go.dev/github.com/go-co-op/gocron/v2#WithGlobalJobOptions)
    to listen for job events and trigger actions.
- **Options**: Many job and scheduler options are available
  - [**Job options**](https://pkg.go.dev/github.com/go-co-op/gocron/v2#JobOption):
    Job options can be set when creating a job using `NewJob`.
  - [**Global job options**](https://pkg.go.dev/github.com/go-co-op/gocron/v2#WithGlobalJobOptions):
    Global job options can be set when creating a scheduler using `NewScheduler`.
  - [**Scheduler options**](https://pkg.go.dev/github.com/go-co-op/gocron/v2#SchedulerOption):
    Scheduler options can be set when creating a scheduler using `NewScheduler`.
- **Logging**: Logs can be enabled.
  - [Logger](https://pkg.go.dev/github.com/go-co-op/gocron/v2#Logger):
    The Logger interface can be implemented with your desired logging library.
    The provided NewLogger uses the standard library's log package.
- **Mocking**: The gocron library is set up to enable testing.
  - Mocks are provided in [the mock package](mocks) using [gomock](https://github.com/uber-go/mock).
  - Time can be mocked by passing in a [FakeClock](https://pkg.go.dev/github.com/jonboulle/clockwork#FakeClock)
    to [WithClock](https://pkg.go.dev/github.com/go-co-op/gocron/v2#WithClock) -
    see the example on WithClock in the go-docs.

## Supporters

[Jetbrains](https://www.jetbrains.com/?from=gocron) supports this project with Intellij licenses.
We appreciate their support for free and open source software!

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=go-co-op/gocron&type=Date)](https://star-history.com/#go-co-op/gocron&Date)
