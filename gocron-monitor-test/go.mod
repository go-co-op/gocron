module test

go 1.21.4

require github.com/go-co-op/gocron/v2 v2.17.0

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/jonboulle/clockwork v0.5.0 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
)

replace github.com/go-co-op/gocron/v2 => ../
