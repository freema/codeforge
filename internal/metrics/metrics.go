package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// TasksTotal counts total tasks processed by status.
	TasksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "codeforge_tasks_total",
			Help: "Total number of tasks processed",
		},
		[]string{"status"},
	)

	// TaskDuration tracks task execution duration in seconds.
	TaskDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "codeforge_tasks_duration_seconds",
			Help:    "Task execution duration in seconds",
			Buckets: []float64{10, 30, 60, 120, 300, 600, 1800},
		},
		[]string{"status"},
	)

	// TasksInProgress tracks the number of currently executing tasks.
	TasksInProgress = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "codeforge_tasks_in_progress",
			Help: "Number of tasks currently in progress",
		},
	)

	// QueueDepth tracks the number of tasks waiting in queue.
	QueueDepth = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "codeforge_queue_depth",
			Help: "Number of tasks waiting in queue",
		},
	)

	// WorkersActive tracks the number of active workers.
	WorkersActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "codeforge_workers_active",
			Help: "Number of active workers",
		},
	)

	// WorkersTotal tracks the total number of workers.
	WorkersTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "codeforge_workers_total",
			Help: "Total number of workers",
		},
	)

	// WebhookDeliveries counts webhook delivery attempts.
	WebhookDeliveries = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "codeforge_webhook_deliveries_total",
			Help: "Total number of webhook delivery attempts",
		},
		[]string{"status"},
	)

	// HTTPRequests counts total HTTP requests.
	HTTPRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "codeforge_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	// HTTPDuration tracks HTTP request duration.
	HTTPDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "codeforge_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"method", "path"},
	)

)
