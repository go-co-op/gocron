package gocron_test

import (
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
)

var task = func() {}

// ---------------------------------------------------------------------
// ----------------------JOB-FUNCTIONS----------------------------------
// ---------------------------------------------------------------------

func ExampleJob_Error() {
	s := gocron.NewScheduler(time.UTC)
	s.Every(1).Day().At("bad time")
	j := s.Jobs()[0]
	fmt.Println(j.Error())
	// Output:
	// the given time format is not supported
}

func ExampleJob_LastRun() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every(1).Second().Do(task)
	s.StartAsync()

	fmt.Println("Last run:", job.LastRun())
}

func ExampleJob_LimitRunsTo() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every(1).Second().Do(task)
	job.LimitRunsTo(2)
	s.StartAsync()
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
	s.StartAsync()
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
	s.StartAsync()
}

func ExampleJob_ScheduledAtTime() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every(1).Day().At("10:30").Do(task)
	s.StartAsync()
	fmt.Println(job.ScheduledAtTime())
	// Output:
	// 10:30
}

func ExampleJob_ScheduledTime() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every(1).Day().At("10:30").Do(task)
	s.StartAsync()
	fmt.Println(job.ScheduledTime())
}

func ExampleJob_SingletonMode() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every(1).Second().Do(task)
	job.SingletonMode()
}

func ExampleJob_Tag() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every("1s").Do(task)

	job.Tag("tag1", "tag2", "tag3")
	s.StartAsync()
	fmt.Println(job.Tags())
	// Output:
	// [tag1 tag2 tag3]
}

func ExampleJob_Tags() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every("1s").Do(task)

	job.Tag("tag1", "tag2", "tag3")
	s.StartAsync()
	fmt.Println(job.Tags())
	// Output:
	// [tag1 tag2 tag3]
}

func ExampleJob_Untag() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every("1s").Do(task)

	job.Tag("tag1", "tag2", "tag3")
	s.StartAsync()
	fmt.Println(job.Tags())
	job.Untag("tag2")
	fmt.Println(job.Tags())
	// Output:
	// [tag1 tag2 tag3]
	// [tag1 tag3]
}

func ExampleJob_Weekday() {
	s := gocron.NewScheduler(time.UTC)
	weeklyJob, _ := s.Every(1).Week().Monday().Do(task)
	weekday, _ := weeklyJob.Weekday()
	fmt.Println(weekday)

	dailyJob, _ := s.Every(1).Day().Do(task)
	_, err := dailyJob.Weekday()
	fmt.Println(err)
	// Output:
	// Monday
	// job not scheduled weekly on a weekday
}

// ---------------------------------------------------------------------
// -------------------SCHEDULER-FUNCTIONS-------------------------------
// ---------------------------------------------------------------------

func ExampleScheduler_At() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Day().At("10:30").Do(task)
	_, _ = s.Every(1).Monday().At("10:30:01").Do(task)
}

func ExampleScheduler_ChangeLocation() {
	s := gocron.NewScheduler(time.UTC)
	fmt.Println(s.Location())

	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}
	s.ChangeLocation(location)
	fmt.Println(s.Location())
	// Output:
	// UTC
	// America/Los_Angeles
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

func ExampleScheduler_Cron() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Cron("*/1 * * * *").Do(task) // every minute
	_, _ = s.Cron("0 1 * * *").Do(task)   // every day at 1 am
	_, _ = s.Cron("0 0 * * 6,0").Do(task) // weekends only
}

func ExampleScheduler_Day() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every("24h").Do(task)
	_, _ = s.Every(1).Day().Do(task)
	_, _ = s.Every(1).Days().Do(task)
}

func ExampleScheduler_Days() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every("24h").Do(task)
	_, _ = s.Every(1).Day().Do(task)
	_, _ = s.Every(1).Days().Do(task)
}

func ExampleScheduler_Do() {
	s := gocron.NewScheduler(time.UTC)
	j, err := s.Every(1).Second().Do(task)
	s.StartAsync()
	fmt.Printf("Job: %v, Error: %v", j, err)
}

func ExampleScheduler_Every() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Second().Do(task)
	_, _ = s.Every(1 * time.Second).Do(task)
	_, _ = s.Every("1s").Do(task)
	s.StartAsync()
}

func ExampleScheduler_Friday() {
	s := gocron.NewScheduler(time.UTC)
	j, _ := s.Every(1).Day().Friday().Do(task)
	s.StartAsync()
	wd, _ := j.Weekday()
	fmt.Println(wd)
	// Output:
	// Friday
}

func ExampleScheduler_Hour() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every("1h").Do(task)
	_, _ = s.Every(1).Hour().Do(task)
	_, _ = s.Every(1).Hours().Do(task)
}

func ExampleScheduler_Hours() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every("1h").Do(task)
	_, _ = s.Every(1).Hour().Do(task)
	_, _ = s.Every(1).Hours().Do(task)
}

func ExampleScheduler_IsRunning() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every("1s").Do(task)
	fmt.Println(s.IsRunning())
	s.StartAsync()
	fmt.Println(s.IsRunning())
	// Output:
	// false
	// true
}

func ExampleScheduler_Job() {
	s := gocron.NewScheduler(time.UTC)
	j, _ := s.Every("1s").Do(func() {})
	s.StartAsync()

	time.Sleep(10 * time.Second)
	_, _ = s.Job(j).Every("10m").Update()

	time.Sleep(30 * time.Minute)
	_, _ = s.Job(j).Every(1).Day().At("02:00").Update()
}

func ExampleScheduler_Jobs() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every("1s").Do(task)
	_, _ = s.Every("1s").Do(task)
	_, _ = s.Every("1s").Do(task)
	fmt.Println(len(s.Jobs()))
	// Output:
	// 3
}

func ExampleScheduler_Len() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every("1s").Do(task)
	_, _ = s.Every("1s").Do(task)
	_, _ = s.Every("1s").Do(task)
	fmt.Println(s.Len())
	// Output:
	// 3
}

func ExampleScheduler_Less() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every("1s").Do(task)
	_, _ = s.Every("2s").Do(task)
	s.StartAsync()
	fmt.Println(s.Less(0, 1))
	// Output:
	// true
}

func ExampleScheduler_LimitRunsTo() {
	s := gocron.NewScheduler(time.UTC)

	j, _ := s.Every(1).Second().LimitRunsTo(1).Do(task)
	s.StartAsync()

	fmt.Println(j.RunCount())
	// Output:
	// 1
}

func ExampleScheduler_Location() {
	s := gocron.NewScheduler(time.UTC)
	fmt.Println(s.Location())
	// Output:
	// UTC
}

func ExampleScheduler_Millisecond() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every(1).Millisecond().Do(task)
	_, _ = s.Every(1).Milliseconds().Do(task)
	_, _ = s.Every("1ms").Seconds().Do(task)
	_, _ = s.Every(time.Millisecond).Seconds().Do(task)
}

func ExampleScheduler_Milliseconds() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every(1).Millisecond().Do(task)
	_, _ = s.Every(1).Milliseconds().Do(task)
	_, _ = s.Every("1ms").Seconds().Do(task)
	_, _ = s.Every(time.Millisecond).Seconds().Do(task)
}

func ExampleScheduler_Minute() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every("1m").Do(task)
	_, _ = s.Every(1).Minute().Do(task)
	_, _ = s.Every(1).Minutes().Do(task)
}

func ExampleScheduler_Minutes() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every("1m").Do(task)
	_, _ = s.Every(1).Minute().Do(task)
	_, _ = s.Every(1).Minutes().Do(task)
}

func ExampleScheduler_Monday() {
	s := gocron.NewScheduler(time.UTC)
	j, _ := s.Every(1).Day().Monday().Do(task)
	s.StartAsync()
	wd, _ := j.Weekday()
	fmt.Println(wd)
	// Output:
	// Monday
}

func ExampleScheduler_Month() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every(1).Month(1).Do(task)
	_, _ = s.Every(1).Months(1).Do(task)
}

func ExampleScheduler_Months() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every(1).Month(1).Do(task)
	_, _ = s.Every(1).Months(1).Do(task)
}

func ExampleScheduler_NextRun() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Day().At("10:30").Do(task)
	s.StartAsync()
	_, t := s.NextRun()
	// print only the hour and minute (hh:mm)
	fmt.Println(t.Format("15:04"))
	// Output:
	// 10:30
}

func ExampleScheduler_Remove() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Week().Do(task)
	s.StartAsync()
	s.Remove(task)
	fmt.Println(s.Len())
	// Output:
	// 0
}

func ExampleScheduler_RemoveByReference() {
	s := gocron.NewScheduler(time.UTC)

	j, _ := s.Every(1).Week().Do(task)
	_, _ = s.Every(1).Week().Do(task)
	s.StartAsync()
	s.RemoveByReference(j)
	fmt.Println(s.Len())
	// Output:
	// 1
}

func ExampleScheduler_RemoveByTag() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Week().Tag("tag1").Do(task)
	_, _ = s.Every(1).Week().Tag("tag2").Do(task)
	s.StartAsync()
	_ = s.RemoveByTag("tag1")
	fmt.Println(s.Len())
	// Output:
	// 1
}

func ExampleScheduler_RunAll() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Day().At("10:00").Do(task)
	_, _ = s.Every(2).Day().At("10:00").Do(task)
	_, _ = s.Every(3).Day().At("10:00").Do(task)
	s.StartAsync()
	s.RunAll()
}

func ExampleScheduler_RunAllWithDelay() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Day().At("10:00").Do(task)
	_, _ = s.Every(2).Day().At("10:00").Do(task)
	_, _ = s.Every(3).Day().At("10:00").Do(task)
	s.StartAsync()
	s.RunAllWithDelay(10 * time.Second)
}

func ExampleScheduler_Saturday() {
	s := gocron.NewScheduler(time.UTC)
	j, _ := s.Every(1).Day().Saturday().Do(task)
	s.StartAsync()
	wd, _ := j.Weekday()
	fmt.Println(wd)
	// Output:
	// Saturday
}

func ExampleScheduler_Second() {
	s := gocron.NewScheduler(time.UTC)

	// the default unit is seconds
	// these are all the same
	_, _ = s.Every(1).Do(task)
	_, _ = s.Every(1).Second().Do(task)
	_, _ = s.Every(1).Seconds().Do(task)
	_, _ = s.Every("1s").Seconds().Do(task)
	_, _ = s.Every(time.Second).Seconds().Do(task)
}

func ExampleScheduler_Seconds() {
	s := gocron.NewScheduler(time.UTC)

	// the default unit is seconds
	// these are all the same
	_, _ = s.Every(1).Do(task)
	_, _ = s.Every(1).Second().Do(task)
	_, _ = s.Every(1).Seconds().Do(task)
	_, _ = s.Every("1s").Seconds().Do(task)
	_, _ = s.Every(time.Second).Seconds().Do(task)

}

func ExampleScheduler_SetMaxConcurrentJobs() {
	s := gocron.NewScheduler(time.UTC)
	s.SetMaxConcurrentJobs(1, gocron.RescheduleMode)
	_, _ = s.Every(1).Seconds().Do(func() {
		fmt.Println("This will run once every 5 seconds even though it is scheduled every second because maximum concurrent job limit is set.")
		time.Sleep(5 * time.Second)
	})
}

func ExampleScheduler_SingletonMode() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Second().SingletonMode().Do(task)
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
	fmt.Println(s.IsRunning())
	// Output:
	// false
}

func ExampleScheduler_Sunday() {
	s := gocron.NewScheduler(time.UTC)
	j, _ := s.Every(1).Day().Sunday().Do(task)
	s.StartAsync()
	wd, _ := j.Weekday()
	fmt.Println(wd)
	// Output:
	// Sunday
}

func ExampleScheduler_Swap() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every(1).Tag("tag1").Do(task)
	_, _ = s.Every(1).Tag("tag2").Day().Monday().Do(task)
	fmt.Println(s.Jobs()[0].Tags()[0], s.Jobs()[1].Tags()[0])
	s.Swap(0, 1)
	fmt.Println(s.Jobs()[0].Tags()[0], s.Jobs()[1].Tags()[0])
	// Output:
	// tag1 tag2
	// tag2 tag1
}

func ExampleScheduler_Tag() {
	s := gocron.NewScheduler(time.UTC)

	j, _ := s.Every(1).Week().Tag("tag").Do(task)
	fmt.Println(j.Tags())
	// Output:
	// [tag]
}

func ExampleScheduler_TagsUnique() {
	s := gocron.NewScheduler(time.UTC)
	s.TagsUnique()

	_, _ = s.Every(1).Week().Tag("foo").Do(task)
	_, err := s.Every(1).Week().Tag("foo").Do(task)

	fmt.Println(err)
	// Output:
	// a non-unique tag was set on the job: foo
}

func ExampleScheduler_TaskPresent() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every(1).Do(task)
	fmt.Println(s.TaskPresent(task))
	// Output:
	// true
}

func ExampleScheduler_Thursday() {
	s := gocron.NewScheduler(time.UTC)
	j, _ := s.Every(1).Day().Thursday().Do(task)
	s.StartAsync()
	wd, _ := j.Weekday()
	fmt.Println(wd)
	// Output:
	// Thursday
}

func ExampleScheduler_Tuesday() {
	s := gocron.NewScheduler(time.UTC)
	j, _ := s.Every(1).Day().Tuesday().Do(task)
	s.StartAsync()
	wd, _ := j.Weekday()
	fmt.Println(wd)
	// Output:
	// Tuesday
}

func ExampleScheduler_Update() {
	s := gocron.NewScheduler(time.UTC)
	j, _ := s.Every("1s").Do(func() {})
	s.StartAsync()

	time.Sleep(10 * time.Second)
	_, _ = s.Job(j).Every("10m").Update()

	time.Sleep(30 * time.Minute)
	_, _ = s.Job(j).Every(1).Day().At("02:00").Update()
}

func ExampleScheduler_Wednesday() {
	s := gocron.NewScheduler(time.UTC)
	j, _ := s.Every(1).Day().Wednesday().Do(task)
	s.StartAsync()
	wd, _ := j.Weekday()
	fmt.Println(wd)
	// Output:
	// Wednesday
}

func ExampleScheduler_Week() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every(1).Week().Do(task)
	_, _ = s.Every(1).Weeks().Do(task)
}

func ExampleScheduler_Weekday() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every(1).Week().Weekday(time.Monday).Do(task)
	_, _ = s.Every(1).Weeks().Weekday(time.Tuesday).Do(task)
}

func ExampleScheduler_Weeks() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every(1).Week().Do(task)
	_, _ = s.Every(1).Weeks().Do(task)
}

func ExampleScheduler_RunByTag() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Day().At("10:00").Do(task)
	_, _ = s.Every(2).Day().Tag("tag").At("10:00").Do(task)
	s.StartAsync()
	s.RunByTag("tag")
}

func ExampleScheduler_RunByTagWithDelay() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Day().Tag("tag").At("10:00").Do(task)
	_, _ = s.Every(2).Day().Tag("tag").At("10:00").Do(task)
	s.StartAsync()
	s.RunByTagWithDelay("tag", 2*time.Second)
}
