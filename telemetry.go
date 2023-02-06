package gocron

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	jobLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "gocron_job_latency_milliseconds",
		Help: "time taken to execute a job in milliseconds",
	}, []string{"job_name", "scheduler_name"})

	telemetryOnce sync.Once
)

func initTelemetry() {
	telemetryOnce.Do(func() {
		prometheus.MustRegister(jobLatency)
	})
}

func observeJobLatency(latency float64, jobName, shdName string) {
	jobLatency.WithLabelValues(jobName, shdName).Observe(latency)
}
