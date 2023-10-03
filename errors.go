package gocron

import "fmt"

var (
	ErrCronJobParse            = fmt.Errorf("gocron: CronJob: crontab parse failure")
	ErrDurationJobZero         = fmt.Errorf("gocron: DurationJob: duration must be greater than 0")
	ErrEventListenerFuncNil    = fmt.Errorf("gocron: eventListenerFunc must not be nil")
	ErrJobNotFound             = fmt.Errorf("gocron: job not found")
	ErrNewJobTask              = fmt.Errorf("gocron: NewJob: Task.Function must be of kind reflect.Func")
	ErrStopTimedOut            = fmt.Errorf("gocron: timed out waiting for jobs to finish")
	ErrWithContextNilContext   = fmt.Errorf("gocron: WithContext: context must not be nil")
	ErrWithContextNilCancel    = fmt.Errorf("gocron: WithContext: cancel must not be nil")
	ErrWithFakeClockNil        = fmt.Errorf("gocron: WithFakeClock: clock must not be nil")
	ErrWithLimitZero           = fmt.Errorf("gocron: WithLimit: limit must be greater than 0")
	ErrWithLocationNil         = fmt.Errorf("gocron: WithLocation: location must not be nil")
	ErrWithShutdownTimeoutZero = fmt.Errorf("gocron: WithShutdownTimeout: timeout must be greater than 0")
)
