# goCron: A Golang Job Scheduling Package.

[![CI State](https://github.com/go-co-op/gocron/workflows/Go%20Test/badge.svg)](https://github.com/go-co-op/gocron/actions?query=workflow%3A"Go+Test") ![Go Report Card](https://goreportcard.com/badge/github.com/go-co-op/gocron) [![Go Doc](https://godoc.org/github.com/go-co-op/gocron?status.svg)](https://godoc.org/github.com/go-co-op/gocron)

goCron is a Golang job scheduling package which lets you run Go functions periodically at pre-determined interval using a simple, human-friendly syntax.

goCron is a Golang implementation of Ruby module [clockwork](https://github.com/tomykaira/clockwork) and Python job scheduling package [schedule](https://github.com/dbader/schedule).

See also these two great articles:

- [Rethinking Cron](http://adam.herokuapp.com/past/2010/4/13/rethinking_cron/)
- [Replace Cron with Clockwork](http://adam.herokuapp.com/past/2010/6/30/replace_cron_with_clockwork/)

If you want to chat, you can find us at Slack! [<img src="https://img.shields.io/badge/gophers-gocron-brightgreen?logo=slack">](https://gophers.slack.com/archives/CQ7T0T1FW)

Examples:

```go
package main

import (
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
)

func task() {
    fmt.Println("I am running task.")
}

func taskWithParams(a int, b string) {
    fmt.Println(a, b)
}

func main() {
    // defines a new scheduler that schedules and runs jobs
    s1 := gocron.NewScheduler(time.UTC)

    s1.Every(3).Seconds().Do(task)

    // scheduler starts running jobs and current thread continues to execute
    s1.StartAsync()

    // Do jobs without params
    s2 := gocron.NewScheduler(time.UTC)
    s2.Every(1).Second().Do(task)
    s2.Every(2).Seconds().Do(task)
    s2.Every(1).Minute().Do(task)
    s2.Every(2).Minutes().Do(task)
    s2.Every(1).Hour().Do(task)
    s2.Every(2).Hours().Do(task)
    s2.Every(1).Day().Do(task)
    s2.Every(2).Days().Do(task)
    s2.Every(1).Week().Do(task)
    s2.Every(2).Weeks().Do(task)
    s2.Every(1).Month(time.Now().Day()).Do(task)
    s2.Every(2).Months(15).Do(task)

    // check for errors
    _, err := s2.Every(1).Day().At("bad-time").Do(task)
    if err != nil {
        log.Fatalf("error creating job: %v", err)
    }

    // Do jobs with params
    s2.Every(1).Second().Do(taskWithParams, 1, "hello")

    // Do Jobs with tags
    // initialize tag
    tag1 := []string{"tag1"}
    tag2 := []string{"tag2"}


    s2.Every(1).Week().SetTag(tag1).Do(task)
    s2.Every(1).Week().SetTag(tag2).Do(task)

    // Removing Job Based on Tag
    s2.RemoveJobByTag("tag1")

    // Do jobs on specific weekday
    s2.Every(1).Monday().Do(task)
    s2.Every(1).Thursday().Do(task)

    // Do a job at a specific time - 'hour:min:sec' - seconds optional
    s2.Every(1).Day().At("10:30").Do(task)
    s2.Every(1).Monday().At("18:30").Do(task)
    s2.Every(1).Tuesday().At("18:30:59").Do(task)
    s2.Every(1).Wednesday().At("1:01").Do(task)

    // Begin job at a specific date/time. 
    // Attention: scheduler timezone has precedence over job's timezone!
    t := time.Date(2019, time.November, 10, 15, 0, 0, 0, time.UTC)
    s2.Every(1).Hour().StartAt(t).Do(task)

    // use .StartImmediately() to run job upon scheduler start
    s2.Every(1).Hour().StartImmediately().Do(task)


    // NextRun gets the next running time
    _, time := s2.NextRun()
    fmt.Println(time)

    // Remove a specific job
    s2.Remove(task)

    // Clear all scheduled jobs
    s2.Clear()

    // stop our first scheduler (it still exists but doesn't run anymore)
    s1.Stop() 

    // executes the scheduler and blocks current thread
    s2.StartBlocking()

    // this line is never reached
}
```

and full test cases and [document](http://godoc.org/github.com/jasonlvhit/gocron) will be coming soon (help is wanted! If you want to contribute, pull requests are welcome).

If you need to prevent a job from running at the same time from multiple cron instances (like running a cron app from multiple servers),
you can provide a [Locker implementation](example/lock.go) and lock the required jobs.

```go
gocron.SetLocker(lockerImplementation)
gocron.Every(1).Hour().Lock().Do(task)
```

Looking to contribute? Try to follow these guidelines:
 * Use issues for everything
 * For a small change, just send a PR!
 * For bigger changes, please open an issue for discussion before sending a PR.
 * PRs should have: tests, documentation and examples (if it makes sense)
 * You can also contribute by:
    * Reporting issues
    * Suggesting new features or enhancements
    * Improving/fixing documentation
---
[Jetbrains](https://www.jetbrains.com/?from=gocron) supports this project with GoLand licenses. We appreciate their support for free and open source software!
