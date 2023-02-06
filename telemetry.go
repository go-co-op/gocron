package gocron

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	jobLatency    *prometheus.HistogramVec
	telemetryOnce sync.Once

	// Latency buckets for job latency metrics. This can be overridden on the
	// application side
	LatencyBuckets = prometheus.ExponentialBuckets(1, 2.5, 15)
)

func initTelemetry() {
	telemetryOnce.Do(func() {
		jobLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gocron_job_latency_milliseconds",
			Help:    "time taken to execute a job in milliseconds",
			Buckets: LatencyBuckets,
		}, []string{"job_name", "scheduler_name"})

		prometheus.MustRegister(jobLatency)
	})
}

func observeJobLatency(latency float64, jobName, shdName string) {
	jobLatency.WithLabelValues(jobName, shdName).Observe(latency)
}
