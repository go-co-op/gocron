package gocron

import "time"

type timeHelper interface {
	Now(*time.Location) time.Time
	NowRoundedDownToSeconds(*time.Location) time.Time
	Unix(int64, int64) time.Time
	Sleep(time.Duration)
	Date(int, time.Month, int, int, int, int, int, *time.Location) time.Time
	NewTicker(time.Duration) *time.Ticker
}

func newTimeHelper() timeHelper {
	return &trueTime{}
}

type trueTime struct{}

func (t *trueTime) Now(location *time.Location) time.Time {
	return time.Now().In(location)
}

func (t *trueTime) NowRoundedDownToSeconds(location *time.Location) time.Time {
	n := t.Now(location)
	return t.Date(n.Year(), n.Month(), n.Day(), n.Hour(), n.Minute(), n.Second(), 0, location)
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
