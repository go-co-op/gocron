package main

import (
	"fmt"
	"log"
	"time"

	"github.com/go-co-op/gocron"
)

func main() {
	task := func(in string, job gocron.Job) {
		fmt.Printf("this job's last run: %s this job's next run: %s\n", job.LastRun(), job.NextRun())
		fmt.Printf("in argument is %s\n", in)

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-job.Context().Done():
				fmt.Printf("function has been canceled, performing cleanup and exiting gracefully\n")
				return
			case <-ticker.C:
				fmt.Printf("performing a hard job that takes a long time that I want to kill whenever I want\n")
			}
		}
	}

	var err error
	s := gocron.NewScheduler(time.UTC)
	s.SingletonModeAll()
	j, err := s.Every(1).Hour().Tag("myJob").DoWithJobDetails(task, "foo")
	if err != nil {
		log.Fatalln("error scheduling job", err)
	}

	s.StartAsync()

	// Simulate some more work
	time.Sleep(time.Second)

	// I want to stop the job, together with the underlying goroutine
	fmt.Printf("now I want to kill the job\n")
	err = s.RemoveByTag("myJob")
	if err != nil {
		log.Fatalln("error removing job by tag", err)
	}

	// Wait a bit so that we can see that the job is exiting gracefully
	time.Sleep(time.Second)
	fmt.Printf("Job: %#v, Error: %#v", j, err)
}
