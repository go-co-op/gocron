package gocron

import "fmt"

var (
	ErrCronJobParse                = fmt.Errorf("gocron: CronJob: crontab parse failure")
	ErrDurationJobZero             = fmt.Errorf("gocron: DurationJob: duration must be greater than 0")
	ErrDurationRandomJobMinMax     = fmt.Errorf("gocron: DurationRandomJob: minimum duration must be less than maximum duration")
	ErrEventListenerFuncNil        = fmt.Errorf("gocron: eventListenerFunc must not be nil")
	ErrJobNotFound                 = fmt.Errorf("gocron: job not found")
	ErrNewJobTask                  = fmt.Errorf("gocron: NewJob: Task.Function must be of kind reflect.Func")
	ErrStopTimedOut                = fmt.Errorf("gocron: timed out waiting for jobs to finish")
	ErrWithDistributedElector      = fmt.Errorf("gocron: WithDistributedElector: elector must not be nil")
	ErrWithFakeClockNil            = fmt.Errorf("gocron: WithFakeClock: clock must not be nil")
	ErrWithLimitConcurrentJobsZero = fmt.Errorf("gocron: WithLimitConcurrentJobs: limit must be greater than 0")
	ErrWithLimitedRunsZero         = fmt.Errorf("gocron: WithLimitedRuns: limit must be greater than 0")
	ErrWithLocationNil             = fmt.Errorf("gocron: WithLocation: location must not be nil")
	ErrWithShutdownTimeoutZero     = fmt.Errorf("gocron: WithStopTimeout: timeout must be greater than 0")
)
