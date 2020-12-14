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
	_, _ = s.Every(3).Seconds().Do(task)
	s.StartBlocking()
}

func ExampleScheduler_StartAsync() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(3).Seconds().Do(task)
	s.StartAsync()
}

// Deprecated: All jobs start immediately by default unless set to a specific date or time
func ExampleScheduler_StartImmediately() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Hour().StartImmediately().Do(task)
	s.StartBlocking()
}

func ExampleScheduler_StartAt() {
	s := gocron.NewScheduler(time.UTC)
	specificTime := time.Date(2019, time.November, 10, 15, 0, 0, 0, time.UTC)
	_, _ = s.Every(1).Hour().StartAt(specificTime).Do(task)
	s.StartBlocking()
}

func ExampleScheduler_Stop() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Second().Do(task)
	s.StartAsync()
	s.Stop()
}

func ExampleScheduler_At() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Day().At("10:30").Do(task)
	_, _ = s.Every(1).Monday().At("10:30:01").Do(task)
}

func ExampleJob_ScheduledTime() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every(1).Day().At("10:30").Do(task)
	fmt.Println(job.ScheduledAtTime())
	// Output: 10:30
}

func ExampleScheduler_RemoveJobByTag() {
	s := gocron.NewScheduler(time.UTC)
	tag1 := []string{"tag1"}
	tag2 := []string{"tag2"}
	_, _ = s.Every(1).Week().SetTag(tag1).Do(task)
	_, _ = s.Every(1).Week().SetTag(tag2).Do(task)
	s.StartAsync()
	_ = s.RemoveJobByTag("tag1")
}

func ExampleScheduler_NextRun() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Day().At("10:30").Do(task)
	s.StartAsync()
	_, time := s.NextRun()
	fmt.Println(time.Format("15:04")) // print only the hour and minute (hh:mm)
	// Output: 10:30
}

func ExampleScheduler_Clear() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Second().Do(task)
	_, _ = s.Every(1).Minute().Do(task)
	_, _ = s.Every(1).Month(1).Do(task)
	fmt.Println(len(s.Jobs())) // Print the number of jobs before clearing
	s.Clear()                  // Clear all the jobs
	fmt.Println(len(s.Jobs())) // Print the number of jobs after clearing
	s.StartAsync()
	// Output:
	// 3
	// 0
}

func ExampleJob_LimitRunsTo() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every(1).Second().Do(task)
	job.LimitRunsTo(2)
	s.StartAsync()
}

func ExampleJob_LastRun() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every(1).Second().Do(task)
	go func() {
		for {
			fmt.Println("Last run", job.LastRun())
			time.Sleep(time.Second)
		}
	}()
	<-s.StartAsync()
}

func ExampleJob_NextRun() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every(1).Second().Do(task)
	go func() {
		for {
			fmt.Println("Next run", job.NextRun())
			time.Sleep(time.Second)
		}
	}()
	<-s.StartAsync()
}

func ExampleJob_RunCount() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every(1).Second().Do(task)
	go func() {
		for {
			fmt.Println("Run count", job.RunCount())
			time.Sleep(time.Second)
		}
	}()
	<-s.StartAsync()
}

func ExampleJob_RemoveAfterLastRun() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every(1).Second().Do(task)
	job.LimitRunsTo(1)
	job.RemoveAfterLastRun()
	s.StartAsync()
}
