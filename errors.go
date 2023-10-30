package gocron

import "fmt"

// Public error definitions
var (
	ErrCronJobParse              = fmt.Errorf("gocron: CronJob: crontab parse failure")
	ErrDailyJobAtTimeNil         = fmt.Errorf("gocron: DailyJob: atTime within atTimes must not be nil")
	ErrDailyJobAtTimesNil        = fmt.Errorf("gocron: DailyJob: atTimes must not be nil")
	ErrDailyJobHours             = fmt.Errorf("gocron: DailyJob: atTimes hours must be between 0 and 23 inclusive")
	ErrDailyJobMinutesSeconds    = fmt.Errorf("gocron: DailyJob: atTimes minutes and seconds must be between 0 and 59 inclusive")
	ErrDurationRandomJobMinMax   = fmt.Errorf("gocron: DurationRandomJob: minimum duration must be less than maximum duration")
	ErrEventListenerFuncNil      = fmt.Errorf("gocron: eventListenerFunc must not be nil")
	ErrJobNotFound               = fmt.Errorf("gocron: job not found")
	ErrMonthlyJobDays            = fmt.Errorf("gocron: MonthlyJob: daysOfTheMonth must be between 31 and -31 inclusive, and not 0")
	ErrMonthlyJobAtTimeNil       = fmt.Errorf("gocron: MonthlyJob: atTime within atTimes must not be nil")
	ErrMonthlyJobAtTimesNil      = fmt.Errorf("gocron: MonthlyJob: atTimes must not be nil")
	ErrMonthlyJobDaysNil         = fmt.Errorf("gocron: MonthlyJob: daysOfTheMonth must not be nil")
	ErrMonthlyJobHours           = fmt.Errorf("gocron: MonthlyJob: atTimes hours must be between 0 and 23 inclusive")
	ErrMonthlyJobMinutesSeconds  = fmt.Errorf("gocron: MonthlyJob: atTimes minutes and seconds must be between 0 and 59 inclusive")
	ErrNewJobTask                = fmt.Errorf("gocron: NewJob: Task.Function must be of kind reflect.Func")
	ErrNewJobTaskNil             = fmt.Errorf("gocron: NewJob: Task must not be nil")
	ErrStopTimedOut              = fmt.Errorf("gocron: timed out waiting for jobs to finish")
	ErrWeeklyJobAtTimeNil        = fmt.Errorf("gocron: WeeklyJob: atTime within atTimes must not be nil")
	ErrWeeklyJobAtTimesNil       = fmt.Errorf("gocron: WeeklyJob: atTimes must not be nil")
	ErrWeeklyJobDaysOfTheWeekNil = fmt.Errorf("gocron: WeeklyJob: daysOfTheWeek must not be nil")
	ErrWeeklyJobHours            = fmt.Errorf("gocron: WeeklyJob: atTimes hours must be between 0 and 23 inclusive")
	ErrWeeklyJobMinutesSeconds   = fmt.Errorf("gocron: WeeklyJob: atTimes minutes and seconds must be between 0 and 59 inclusive")
	ErrWithDistributedElector    = fmt.Errorf("gocron: WithDistributedElector: elector must not be nil")
	ErrWithClockNil              = fmt.Errorf("gocron: WithClock: clock must not be nil")
	ErrWithLocationNil           = fmt.Errorf("gocron: WithLocation: location must not be nil")
)

// internal errors
var (
	errAtTimeNil    = fmt.Errorf("errAtTimeNil")
	errAtTimeHours  = fmt.Errorf("errAtTimeHours")
	errAtTimeMinSec = fmt.Errorf("errAtTimeMinSec")
)
