package gocron_test

import (
	"fmt"
	"log"
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

func ExampleJob_IsRunning() {
	s := gocron.NewScheduler(time.UTC)
	j, _ := s.Every(10).Seconds().Do(func() { time.Sleep(2 * time.Second) })

	fmt.Println(j.IsRunning())

	s.StartAsync()

	time.Sleep(time.Second)
	fmt.Println(j.IsRunning())

	time.Sleep(time.Second)
	s.Stop()

	time.Sleep(1 * time.Second)
	fmt.Println(j.IsRunning())
	// Output:
	// false
	// true
	// false
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

	// if multiple times are set, the earliest time will be returned
	job1, _ := s.Every(1).Day().At("10:30;08:00").Do(task)
	fmt.Println(job1.ScheduledAtTime())
	// Output:
	// 10:30
	// 8:0
}

func ExampleJob_ScheduledAtTimes() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every(1).Day().At("10:30;08:00").Do(task)
	s.StartAsync()
	fmt.Println(job.ScheduledAtTimes())
	// Output:
	// [8:0 10:30]
}

func ExampleJob_ScheduledTime() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every(1).Day().At("10:30").Do(task)
	s.StartAsync()
	fmt.Println(job.ScheduledTime())
}

func ExampleJob_SetEventListeners() {
	s := gocron.NewScheduler(time.UTC)
	job, _ := s.Every(1).Week().Do(task)
	job.SetEventListeners(func() {}, func() {})
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

func ExampleJob_Weekdays() {
	s := gocron.NewScheduler(time.UTC)
	j, _ := s.Every(1).Week().Monday().Wednesday().Friday().Do(task)
	fmt.Println(j.Weekdays())
	// Output:
	// [Monday Wednesday Friday]
}

// ---------------------------------------------------------------------
// -------------------SCHEDULER-FUNCTIONS-------------------------------
// ---------------------------------------------------------------------

func ExampleScheduler_At() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Day().At("10:30").Do(task)
	_, _ = s.Every(1).Monday().At("10:30:01").Do(task)
	// multiple
	_, _ = s.Every(1).Monday().At("10:30;18:00").Do(task)
	_, _ = s.Every(1).Monday().At("10:30").At("18:00").Do(task)
}

func ExampleScheduler_ChangeLocation() {
	s := gocron.NewScheduler(time.UTC)
	fmt.Println(s.Location())

	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		log.Fatalf("Error loading location: %s", err)
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

	// parsing handled by https://pkg.go.dev/github.com/robfig/cron/v3
	// which follows https://en.wikipedia.org/wiki/Cron
	_, _ = s.Cron("*/1 * * * *").Do(task) // every minute
	_, _ = s.Cron("0 1 * * *").Do(task)   // every day at 1 am
	_, _ = s.Cron("0 0 * * 6,0").Do(task) // weekends only
}

func ExampleScheduler_CronWithSeconds() {
	s := gocron.NewScheduler(time.UTC)

	// parsing handled by https://pkg.go.dev/github.com/robfig/cron/v3
	// which follows https://en.wikipedia.org/wiki/Cron
	_, _ = s.CronWithSeconds("*/1 * * * * *").Do(task)  // every second
	_, _ = s.CronWithSeconds("0-30 * * * * *").Do(task) // every second 0-30
}

func ExampleScheduler_CustomTime() {
	// Implement your own custom time struct
	//
	// type myCustomTime struct{}
	//
	// var _ gocron.TimeWrapper = (*myCustomTime)(nil)
	//
	// func (m myCustomTime) Now(loc *time.Location) time.Time {
	//	 panic("implement me")
	// }
	//
	// func (m myCustomTime) Sleep(duration time.Duration) {
	//	 panic("implement me")
	// }
	//
	// func (m myCustomTime) Unix(sec int64, nsec int64) time.Time {
	//	 panic("implement me")
	// }
	//
	// mct := myCustomTime{}
	//
	// s := gocron.NewScheduler(time.UTC)
	// s.CustomTime(mct)
}

func ExampleScheduler_CustomTimer() {
	s := gocron.NewScheduler(time.UTC)
	s.CustomTimer(func(d time.Duration, f func()) *time.Timer {
		// force jobs with 1 minute interval to run every second
		if d == time.Minute {
			d = time.Second
		}
		return time.AfterFunc(d, f)
	})
	// this job will run every 1 second
	_, _ = s.Every("1m").Do(task)
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
	j1, err := s.Every(1).Second().Do(task)
	fmt.Printf("Job: %v, Error: %v", j1, err)

	taskWithParameters := func(param1, param2 string) {}
	j2, err := s.Every(1).Second().Do(taskWithParameters, "param1", "param2")
	fmt.Printf("Job: %v, Error: %v", j2, err)
	s.StartAsync()
}

func ExampleScheduler_DoWithJobDetails() {
	task := func(in string, job gocron.Job) {
		fmt.Printf("this job's last run: %s\nthis job's next run: %s", job.LastRun(), job.NextRun())
	}

	s := gocron.NewScheduler(time.UTC)
	j, err := s.Every(1).Second().DoWithJobDetails(task, "foo")
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

func ExampleScheduler_EveryRandom() {
	s := gocron.NewScheduler(time.UTC)

	// every 1 - 5 seconds randomly
	_, _ = s.EveryRandom(1, 5).Seconds().Do(task)

	// every 5 - 10 hours randomly
	_, _ = s.EveryRandom(5, 10).Hours().Do(task)

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

func ExampleScheduler_Midday() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Day().Midday().Do(task)
	s.StartAsync()
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

	_, _ = s.Every(1).Month().Do(task)
	_, _ = s.Every(1).Month(1).Do(task)
	_, _ = s.Every(1).Months(1).Do(task)
	_, _ = s.Every(1).Month(1, 2).Do(task)
	_, _ = s.Month(1, 2).Every(1).Do(task)
}

func ExampleScheduler_MonthFirstWeekday() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.MonthFirstWeekday(time.Monday).Do(task)
	s.StartAsync()
}

func ExampleScheduler_MonthLastDay() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every(1).MonthLastDay().Do(task)
	_, _ = s.Every(2).MonthLastDay().Do(task)
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

func ExampleScheduler_RemoveByTags() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Week().Tag("tag1", "tag2", "tag3").Do(task)
	_, _ = s.Every(1).Week().Tag("tag1", "tag2").Do(task)
	_, _ = s.Every(1).Week().Tag("tag1").Do(task)
	s.StartAsync()
	_ = s.RemoveByTags("tag1", "tag2")
	fmt.Println(s.Len())
	// Output:
	// 1
}

func ExampleScheduler_RemoveByTagsAny() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Week().Tag("tag1", "tag2", "tag3").Do(task)
	_, _ = s.Every(1).Week().Tag("tag1").Do(task)
	_, _ = s.Every(1).Week().Tag("tag2").Do(task)
	_, _ = s.Every(1).Week().Tag("tag3").Do(task)
	s.StartAsync()
	_ = s.RemoveByTagsAny("tag1", "tag3")
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

func ExampleScheduler_RunByTag() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Day().At("10:00").Do(task)
	_, _ = s.Every(2).Day().Tag("tag").At("10:00").Do(task)
	s.StartAsync()
	_ = s.RunByTag("tag")
}

func ExampleScheduler_RunByTagWithDelay() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Day().Tag("tag").At("10:00").Do(task)
	_, _ = s.Every(2).Day().Tag("tag").At("10:00").Do(task)
	s.StartAsync()
	_ = s.RunByTagWithDelay("tag", 2*time.Second)
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

func ExampleScheduler_SingletonModeAll() {
	s := gocron.NewScheduler(time.UTC)
	s.SingletonModeAll()

	_, _ = s.Every(1).Second().Do(task)
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

func ExampleScheduler_StartBlocking() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(3).Seconds().Do(task)
	s.StartBlocking()
}

func ExampleScheduler_StartImmediately() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Cron("0 0 * * 6,0").StartImmediately().Do(task)
	s.StartBlocking()
}

func ExampleScheduler_Stop() {
	s := gocron.NewScheduler(time.UTC)
	_, _ = s.Every(1).Second().Do(task)
	s.StartAsync()
	s.Stop()
	fmt.Println(s.IsRunning())

	s = gocron.NewScheduler(time.UTC)

	go func() {
		time.Sleep(1 * time.Second)
		s.Stop()
	}()

	s.StartBlocking()
	fmt.Println(".Stop() stops the blocking start")

	// Output:
	// false
	// .Stop() stops the blocking start
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
	j, _ := s.Every("1s").Do(task)
	s.StartAsync()

	time.Sleep(10 * time.Second)
	_, _ = s.Job(j).Every("10m").Update()

	time.Sleep(30 * time.Minute)
	_, _ = s.Job(j).Every(1).Day().At("02:00").Update()
}

func ExampleScheduler_WaitForSchedule() {
	s := gocron.NewScheduler(time.UTC)

	// job will run 5 minutes from the scheduler starting
	_, _ = s.Every("5m").WaitForSchedule().Do(task)

	// job will run immediately and 5 minutes from the scheduler starting
	_, _ = s.Every("5m").Do(task)
	s.StartAsync()
}

func ExampleScheduler_WaitForScheduleAll() {
	s := gocron.NewScheduler(time.UTC)
	s.WaitForScheduleAll()

	// all jobs will run 5 minutes from the scheduler starting
	_, _ = s.Every("5m").Do(task)
	_, _ = s.Every("5m").Do(task)
	s.StartAsync()
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

	_, _ = s.Every(1).Week().Monday().Wednesday().Friday().Do(task)
}

func ExampleScheduler_Weekday() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every(1).Week().Weekday(time.Monday).Do(task)
	_, _ = s.Every(1).Weeks().Weekday(time.Tuesday).Weekday(time.Friday).Do(task)
}

func ExampleScheduler_Weeks() {
	s := gocron.NewScheduler(time.UTC)

	_, _ = s.Every(1).Week().Do(task)
	_, _ = s.Every(1).Weeks().Do(task)

	_, _ = s.Every(2).Weeks().Monday().Wednesday().Friday().Do(task)
}

// ---------------------------------------------------------------------
// ---------------------OTHER-FUNCTIONS---------------------------------
// ---------------------------------------------------------------------

func ExampleSetPanicHandler() {
	gocron.SetPanicHandler(func(jobName string, recoverData interface{}) {
		fmt.Printf("Panic in job: %s", jobName)
		fmt.Println("do something to handle the panic")
	})
}
