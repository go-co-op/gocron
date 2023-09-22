package gocron

import (
	"log"
	"testing"
	"time"
)

func TestScheduler_Start(t *testing.T) {
	s, err := NewScheduler()
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.NewJob(
		NewCronJob(
			"* * * * * *",
			true,
			Task{
				Function: func() { log.Println("job ran") },
			},
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	s.Start()
	time.Sleep(2 * time.Second)
	lastRun, err := s.GetJobLastRun(id)
	log.Println(lastRun)
}
