# gocron: A Golang Job Scheduling Package.

[![CI State](https://github.com/go-co-op/gocron/workflows/Go%20Test/badge.svg)](https://github.com/go-co-op/gocron/actions?query=workflow%3A"Go+Test") ![Go Report Card](https://goreportcard.com/badge/github.com/go-co-op/gocron) [![Go Doc](https://godoc.org/github.com/go-co-op/gocron?status.svg)](https://pkg.go.dev/github.com/go-co-op/gocron)

gocron is a Golang job scheduling package which lets you run Go functions periodically at pre-determined interval using a simple, human-friendly syntax.

gocron is a Golang implementation of the Ruby module [clockwork](https://github.com/tomykaira/clockwork) and the Python job scheduling package [schedule](https://github.com/dbader/schedule).

See also these two great articles:

- [Rethinking Cron](http://adam.herokuapp.com/past/2010/4/13/rethinking_cron/)
- [Replace Cron with Clockwork](http://adam.herokuapp.com/past/2010/6/30/replace_cron_with_clockwork/)

If you want to chat, you can find us at Slack! [<img src="https://img.shields.io/badge/gophers-gocron-brightgreen?logo=slack">](https://gophers.slack.com/archives/CQ7T0T1FW)

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

[Jetbrains](https://www.jetbrains.com/?from=gocron) supports this project with GoLand licenses. We appreciate their support for free and open source software!
