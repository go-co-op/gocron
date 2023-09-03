package v2

import "time"

func main() {
	s := &scheduler{}
	err := s.NewJob(
		CronJob(
			"*/1 * * * * *",
			true,
			SingletonMode(),
			WithTimezone(time.UTC),
		),
	)
	if err != nil {
		return
	}

}
