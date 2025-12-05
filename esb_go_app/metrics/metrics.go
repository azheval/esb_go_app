package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	MessagesProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "esb_go_messages_processed_total",
			Help: "Total number of messages processed by worker type.",
		},
		[]string{"worker_type", "source", "destination"},
	)

	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "esb_go_errors_total",
			Help: "Total number of errors encountered by worker type.",
		},
		[]string{"worker_type"},
	)

	ActiveWorkers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "esb_go_active_workers",
			Help: "Current number of active workers by type.",
		},
		[]string{"worker_type"},
	)
)

func Register() {

}
