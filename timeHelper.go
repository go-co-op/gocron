package gocron

import "time"

type timeHelper interface {
	Now() time.Time
	Unix(int64, int64) time.Time
	Sleep(time.Duration)
	Date(int, time.Month, int, int, int, int, int, *time.Location) time.Time
	NewTicker(time.Duration) *time.Ticker
}

func newTimeHelper() timeHelper {
	return &trueTime{}
}

type trueTime struct{}

func (t *trueTime) Now() time.Time {
	return time.Now()
}

func (t *trueTime) Unix(sec int64, nsec int64) time.Time {
	return time.Unix(sec, nsec)
}

func (t *trueTime) Sleep(d time.Duration) {
	time.Sleep(d)
}

func (t *trueTime) Date(year int, month time.Month, day, hour, min, sec, nsec int, loc *time.Location) time.Time {
	return time.Date(year, month, day, hour, min, sec, nsec, loc)
}

func (t *trueTime) NewTicker(d time.Duration) *time.Ticker {
	return time.NewTicker(d)
}
