package gocron

import "fmt"

var (
	ErrStopTimedOut = fmt.Errorf("gocron: timed out waiting for jobs to finish")
)
