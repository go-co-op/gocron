package v2

func main() {
	s := &scheduler{}
	err := s.NewJob(
		CronJob(
			"* * * * *",
			false,
			SingletonMode(),
			LimitRunsTo(1),
		),
	)
	if err != nil {
		return
	}

}
