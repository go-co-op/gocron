package gocron

import "fmt"

var (
	ErrCronJobParse                = fmt.Errorf("gocron: CronJob: crontab parse failure")
	ErrDurationJobZero             = fmt.Errorf("gocron: DurationJob: duration must be greater than 0")
	ErrDurationRandomJobMinMax     = fmt.Errorf("gocron: DurationRandomJob: minimum duration must be less than maximum duration")
	ErrEventListenerFuncNil        = fmt.Errorf("gocron: eventListenerFunc must not be nil")
	ErrJobNotFound                 = fmt.Errorf("gocron: job not found")
	ErrMonthlyJobDays              = fmt.Errorf("gocron: MonthlyJob: daysOfTheMonth must be between 31 and -31 inclusive, and not 0")
	ErrMonthlyJobHours             = fmt.Errorf("gocron: MonthlyJob: atTimes hours must be between 0 and 23 inclusive")
	ErrMonthlyJobMinutesSeconds    = fmt.Errorf("gocron: MonthlyJob: atTimes minutes and seconds must be between 0 and 59 inclusive")
	ErrNewJobTask                  = fmt.Errorf("gocron: NewJob: Task.Function must be of kind reflect.Func")
	ErrNewJobTaskNil               = fmt.Errorf("gocron: NewJob: Task must not be nil")
	ErrStopTimedOut                = fmt.Errorf("gocron: timed out waiting for jobs to finish")
	ErrWithDistributedElector      = fmt.Errorf("gocron: WithDistributedElector: elector must not be nil")
	ErrWithFakeClockNil            = fmt.Errorf("gocron: WithFakeClock: clock must not be nil")
	ErrWithLimitConcurrentJobsZero = fmt.Errorf("gocron: WithLimitConcurrentJobs: limit must be greater than 0")
	ErrWithLimitedRunsZero         = fmt.Errorf("gocron: WithLimitedRuns: limit must be greater than 0")
	ErrWithLocationNil             = fmt.Errorf("gocron: WithLocation: location must not be nil")
	ErrWithShutdownTimeoutZero     = fmt.Errorf("gocron: WithStopTimeout: timeout must be greater than 0")
)
