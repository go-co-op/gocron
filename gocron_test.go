// Tests for gocron
package gocron

import (
	"fmt"
	"testing"
	"time"
)

var err = 1

func task() string {
	return "I am a running job."
}

func taskWithParams(a int, b string) (a_ int, b_ string) {
	return a, b
}

func TestSecond(*testing.T) {
	now := time.Now()
	default_scheduler.Every(1).Second().Do(task)
	default_scheduler.Every(1).Second().Do(taskWithParams, 1, "hello")
}
