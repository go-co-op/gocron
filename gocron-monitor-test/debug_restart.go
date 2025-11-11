package main

import (
	"fmt"
	"time"

	"github.com/go-co-op/gocron/v2"
)

type DebugMonitor struct {
	startCount      int
	stopCount       int
	jobRegCount     int
	jobUnregCount   int
	jobStartCount   int
	jobRunningCount int
	jobCompletCount int
	jobFailCount    int
}

func (m *DebugMonitor) SchedulerStarted() {
	m.startCount++
	fmt.Printf("✓ SchedulerStarted() called (total: %d)\n", m.startCount)
}

func (m *DebugMonitor) SchedulerShutdown() {
	m.stopCount++
	fmt.Printf("✓ SchedulerShutdown() called (total: %d)\n", m.stopCount)
}

func (m *DebugMonitor) JobRegistered(job *gocron.Job) {
	m.jobRegCount++
	fmt.Printf("✓ JobRegistered() called (total: %d) - Job ID: %s\n", m.jobRegCount, (*job).ID())
}

func (m *DebugMonitor) JobUnregistered(job *gocron.Job) {
	m.jobUnregCount++
	fmt.Printf("✓ JobUnregistered() called (total: %d) - Job ID: %s\n", m.jobUnregCount, (*job).ID())
}

func (m *DebugMonitor) JobStarted(job *gocron.Job) {
	m.jobStartCount++
	fmt.Printf("✓ JobStarted() called (total: %d) - Job ID: %s\n", m.jobStartCount, (*job).ID())
}

func (m *DebugMonitor) JobRunning(job *gocron.Job) {
	m.jobRunningCount++
	fmt.Printf("✓ JobRunning() called (total: %d) - Job ID: %s\n", m.jobRunningCount, (*job).ID())
}

func (m *DebugMonitor) JobCompleted(job *gocron.Job) {
	m.jobCompletCount++
	fmt.Printf("✓ JobCompleted() called (total: %d) - Job ID: %s\n", m.jobCompletCount, (*job).ID())
}

func (m *DebugMonitor) JobFailed(job *gocron.Job, err error) {
	m.jobFailCount++
	fmt.Printf("✓ JobFailed() called (total: %d) - Job ID: %s, Error: %v\n", m.jobFailCount, (*job).ID(), err)
}

func main() {
	// ONE monitor, multiple scheduler instances
	monitor := &DebugMonitor{}

	fmt.Println("=== Cycle 1 (Scheduler Instance 1) ===")
	s1, err := gocron.NewScheduler(
		gocron.WithSchedulerMonitor(monitor),
	)
	if err != nil {
		panic(err)
	}

	// Create and register some test jobs
	fmt.Println("Creating jobs...")
	_, err = s1.NewJob(
		gocron.DurationJob(1*time.Second),
		gocron.NewTask(func() { fmt.Println("Job 1 running") }),
	)
	if err != nil {
		panic(err)
	}

	_, err = s1.NewJob(
		gocron.DurationJob(2*time.Second),
		gocron.NewTask(func() error {
			fmt.Println("Job 2 executing and returning error")
			return fmt.Errorf("simulated job failure")
		}), // This job will fail with error
	)
	if err != nil {
		panic(err)
	}

	fmt.Println("Calling Start()...")
	s1.Start()
	time.Sleep(3 * time.Second) // Wait for jobs to execute

	fmt.Println("Calling Shutdown()...")
	err = s1.Shutdown()
	if err != nil {
		fmt.Printf("Shutdown error: %v\n", err)
	}

	fmt.Println("\n=== Cycle 2 (Job Updates) ===")
	s2, err := gocron.NewScheduler(
		gocron.WithSchedulerMonitor(monitor),
	)
	if err != nil {
		panic(err)
	}

	fmt.Println("Creating and updating jobs...")
	job3, err := s2.NewJob(
		gocron.DurationJob(1*time.Second),
		gocron.NewTask(func() { fmt.Println("Job 3 running") }),
	)
	if err != nil {
		panic(err)
	}

	// Update the job
	_, err = s2.Update(
		job3.ID(),
		gocron.DurationJob(2*time.Second),
		gocron.NewTask(func() { fmt.Println("Job 3 updated") }),
	)
	if err != nil {
		panic(err)
	}

	fmt.Println("Calling Start()...")
	s2.Start()
	time.Sleep(3 * time.Second)

	fmt.Println("Calling Shutdown()...")
	err = s2.Shutdown()
	if err != nil {
		fmt.Printf("Shutdown error: %v\n", err)
	}

	fmt.Println("\n=== Summary ===")
	fmt.Printf("Total Scheduler Starts: %d\n", monitor.startCount)
	fmt.Printf("Total Scheduler Stops: %d\n", monitor.stopCount)
	fmt.Printf("Total Jobs Registered: %d\n", monitor.jobRegCount)
	fmt.Printf("Total Jobs Unregistered: %d\n", monitor.jobUnregCount)
	fmt.Printf("Total Jobs Started: %d\n", monitor.jobStartCount)
	fmt.Printf("Total Jobs Running: %d\n", monitor.jobRunningCount)
	fmt.Printf("Total Jobs Completed: %d\n", monitor.jobCompletCount)
	fmt.Printf("Total Jobs Failed: %d\n", monitor.jobFailCount)
}
