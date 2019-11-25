This package is currently looking for new maintainers (cause @jasonlvhit is in [ICU](https://github.com/996icu/996.ICU)). Please message @jasonlvhit if you are interested.

## goCron: A Golang Job Scheduling Package.

[![GgoDoc](https://godoc.org/github.com/golang/gddo?status.svg)](http://godoc.org/github.com/jasonlvhit/gocron)
[![Go Report Card](https://goreportcard.com/badge/github.com/jasonlvhit/gocron)](https://goreportcard.com/report/github.com/jasonlvhit/gocron)

goCron is a Golang job scheduling package which lets you run Go functions periodically at pre-determined interval using a simple, human-friendly syntax.

goCron is a Golang implementation of Ruby module [clockwork](https://github.com/tomykaira/clockwork) and Python job scheduling package [schedule](https://github.com/dbader/schedule), and personally, this package is my first Golang program, just for fun and practice.

See also this two great articles:

- [Rethinking Cron](http://adam.herokuapp.com/past/2010/4/13/rethinking_cron/)
- [Replace Cron with Clockwork](http://adam.herokuapp.com/past/2010/6/30/replace_cron_with_clockwork/)

If you want to chat, you can find us at Slack! [<img src="https://img.shields.io/badge/gophers-gocron-brightgreen?logo=slack">](https://gophers.slack.com/archives/CQ7T0T1FW)


Back to this package, you could just use this simple API as below, to run a cron scheduler.

```go
package main

import (
	"fmt"
	"time"
	
	"github.com/jasonlvhit/gocron"
)

func task() {
	fmt.Println("I am running task.")
}

func taskWithParams(a int, b string) {
	fmt.Println(a, b)
}

func main() {
	// Do jobs with params
	gocron.Every(1).Second().Do(taskWithParams, 1, "hello")
	
	// Do jobs safely, preventing an unexpected panic from bubbling up
	gocron.Every(1).Second().DoSafely(taskWithParams, 1, "hello")

	// Do jobs without params
	gocron.Every(1).Second().Do(task)
	gocron.Every(2).Seconds().Do(task)
	gocron.Every(1).Minute().Do(task)
	gocron.Every(2).Minutes().Do(task)
	gocron.Every(1).Hour().Do(task)
	gocron.Every(2).Hours().Do(task)
	gocron.Every(1).Day().Do(task)
	gocron.Every(2).Days().Do(task)
	gocron.Every(1).Week().Do(task)
	gocron.Every(2).Weeks().Do(task)

	// Do jobs on specific weekday
	gocron.Every(1).Monday().Do(task)
	gocron.Every(1).Thursday().Do(task)

	// Do a job at a specific time - 'hour:min'
	gocron.Every(1).Day().At("10:30").Do(task)
	gocron.Every(1).Monday().At("18:30").Do(task)

	// Begin job immediately upon start
	gocron.Every(1).Hour().From(gocron.NextTick()).Do(task)
	
	// Begin job at a specific date/time
	t := time.Date(2019, time.November, 10, 15, 0, 0, 0, time.Local)
	gocron.Every(1).Hour().From(&t).Do(task)

	// NextRun gets the next running time
	_, time := gocron.NextRun()
	fmt.Println(time)

    // Remove a specific job
	gocron.Remove(task)
	
	// Clear all scheduled jobs
	gocron.Clear()

	// Start all the pending jobs
	<- gocron.Start()

	// also, you can create a new scheduler
	// to run two schedulers concurrently
	s := gocron.NewScheduler()
	s.Every(3).Seconds().Do(task)
	<- s.Start()
}
```

and full test cases and [document](http://godoc.org/github.com/jasonlvhit/gocron) will be coming soon (help is wanted! If you want to contribute, pull requests are welcome).

If you need to prevent a job from running at the same time from multiple cron instances (like running a cron app from multiple servers),
you can provide a [Locker implementation](example/lock.go) and lock the required jobs.

```go
gocron.SetLocker(lockerImplementation)
gocron.Every(1).Hour().Lock().Do(task)
```

Once again, thanks to the great works of Ruby clockwork and Python schedule package. BSD license is used, see the file License for detail.

Looking to contribute? Try to follow these guidelines:
 * Use issues for everything
 * For a small change, just send a PR!
 * For bigger changes, please open an issue for discussion before sending a PR.
 * PRs should have: tests, documentation and examples (if it makes sense)
 * You can also contribute by: 
    * Reporting issues 
    * Suggesting new features or enhancements 
    * Improving/fixing documentation

Have fun!
