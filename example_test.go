package gocron_test

import (
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
)

var task = func() {
	fmt.Println("I am a task")
}

func ExampleScheduler_StartBlocking() {
	s := gocron.NewScheduler(time.UTC)
	s.Every(3).Seconds().Do(task)
	s.StartBlocking()
}

func ExampleScheduler_At() {
	s := gocron.NewScheduler(time.UTC)
	s.Every(1).Day().At("10:30").Do(task)
	s.Every(1).Monday().At("10:30:01").Do(task)
}

func ExampleJob_GetScheduledTime() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every(1).Day().At("10:30").Do(task)
	fmt.Println(job.ScheduledAtTime())
	// Output: 10:30
}
