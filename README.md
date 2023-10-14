# gocron: A Golang Job Scheduling Package.

[![CI State](https://github.com/go-co-op/gocron/actions/workflows/go_test.yml/badge.svg?branch=main&event=push)](https://github.com/go-co-op/gocron/actions)
![Go Report Card](https://goreportcard.com/badge/github.com/go-co-op/gocron) [![Go Doc](https://godoc.org/github.com/go-co-op/gocron?status.svg)](https://pkg.go.dev/github.com/go-co-op/gocron)

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
			gocron.NewTask(
				func() {},
			),
		),
	)
	if err != nil {
		// handle error
	}
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


## Supporters

[Jetbrains](https://www.jetbrains.com/?from=gocron) supports this project with Intellij licenses. 
We appreciate their support for free and open source software!

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=go-co-op/gocron&type=Date)](https://star-history.com/#go-co-op/gocron&Date)


