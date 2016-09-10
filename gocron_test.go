// Tests for gocron
package gocron

import (
	"fmt"
	"testing"
	"time"
)

var err = 1

func task() {
	fmt.Println("I am a running job.")
}

func taskWithParams(a int, b string) {
	fmt.Println(a, b)
}

func TestSecond(*testing.T) {
	defaultScheduler.Every(1).Second().Do(task)
	defaultScheduler.Every(1).Second().Do(taskWithParams, 1, "hello")
	defaultScheduler.Start()
	time.Sleep(10 * time.Second)
}

func TestScheduled(t *testing.T) {
	n := NewScheduler()
	n.Every(1).Second().Do(task)
	if !n.Scheduled(task) {
		t.Fatal("Task was scheduled but function couldn't found it")
	}
}
