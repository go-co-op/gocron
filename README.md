# gocron: A Golang Job Scheduling Package.

[![CI State](https://github.com/go-co-op/gocron/workflows/Go%20Test/badge.svg)](https://github.com/go-co-op/gocron/actions?query=workflow%3A"Go+Test") ![Go Report Card](https://goreportcard.com/badge/github.com/go-co-op/gocron) [![Go Doc](https://godoc.org/github.com/go-co-op/gocron?status.svg)](https://pkg.go.dev/github.com/go-co-op/gocron)

gocron is a Golang job scheduling package which lets you run Go functions periodically at pre-determined interval using a simple, human-friendly syntax.

gocron is a Golang implementation of the Ruby module [clockwork](https://github.com/tomykaira/clockwork) and the Python job scheduling package [schedule](https://github.com/dbader/schedule).

See also these two great articles:

- [Rethinking Cron](http://adam.herokuapp.com/past/2010/4/13/rethinking_cron/)
- [Replace Cron with Clockwork](http://adam.herokuapp.com/past/2010/6/30/replace_cron_with_clockwork/)

If you want to chat, you can find us at Slack! [<img src="https://img.shields.io/badge/gophers-gocron-brightgreen?logo=slack">](https://gophers.slack.com/archives/CQ7T0T1FW)

## Concepts

- **Scheduler**: The scheduler tracks all the jobs assigned to it and makes sure they are passed to the executor when ready to be run. The scheduler is able to manage overall aspects of job behavior like limiting how many jobs are running at one time.
- **Job**: The job is simply aware of the task (go function) it's provided and is therefore only able to perform actions related to that task like preventing itself from overruning a previous task that is taking a long time.
- **Executor**: The executor, as it's name suggests, is simply responsible for calling the task (go function) that the job hands to it when sent by the scheduler.

## Examples

```golang
s := gocron.NewScheduler(time.UTC)

s.Every(5).Seconds().Do(func(){ ... })

// strings parse to duration
s.Every("5m").Do(func(){ ... })

s.Every(5).Days().Do(func(){ ... })

// cron expressions supported
s.Cron("*/1 * * * *").Do(task) // every minute
```

For more examples, take a look in our [go docs](https://pkg.go.dev/github.com/go-co-op/gocron#pkg-examples)

## Options

Interval | Supported schedule options
-- | --
sub-second | `StartAt()`
milliseconds | `StartAt()`
seconds | `StartAt()`
minutes | `StartAt()`
hours | `StartAt()`
days | `StartAt()`, `At()`
weeks | `StartAt()`, `At()`, `Weekday()` (and all week day named functions)
months | `StartAt()`, `At()`

There are several options available to restrict how jobs run:

Mode | Function | Behavior
-- | -- | --
Default |  | jobs are rescheduled at every interval
Job singleton | `SingletonMode()` | a long running job will not be rescheduled until the current run is completed
Scheduler limit | `SetMaxConcurrentJobs()` | set a collective maximum number of concurrent jobs running across the scheduler

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

* Q: I'm running multiple pods on a distributed environment. How can I make a job not run once per pod causing duplication?
* A: We recommend using your own lock solution within the jobs themselves (you could use [Redis](https://redis.io/topics/distlock), for example)

* Q: I've removed my job from the scheduler, but how can I stop a long-running job that has already been triggered?
* A: We recommend using a means of canceling your job, e.g. a `context.WithCancel()`.

--- 
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

## Design

![design-diagram](https://user-images.githubusercontent.com/19351306/110375142-2ba88680-8017-11eb-80c3-554cc746b165.png)

[Jetbrains](https://www.jetbrains.com/?from=gocron) supports this project with GoLand licenses. We appreciate their support for free and open source software!
