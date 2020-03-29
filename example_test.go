package gocron_test

import (
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
)

func ExampleScheduler_Start() {
	s := gocron.NewScheduler(time.UTC)
	s.Every(3).Seconds().Do(func() { fmt.Println("I am a task") })
	<-s.Start()
}

func ExampleScheduler_At() {
	s := gocron.NewScheduler(time.UTC)
	s.Every(1).Day().At("10:30").Do(func() { fmt.Println("I am a task") })
	s.Every(1).Monday().At("10:30:01").Do(func() { fmt.Println("I am a task") })
}

func ExampleJob_GetAt() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every(1).Day().At("10:30").Do(func() { fmt.Println("I am a task") })
	fmt.Println(job.GetAt())
	// Output: 10:30
}
