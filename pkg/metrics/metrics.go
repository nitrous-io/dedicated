package metrics

import (
	"github.com/rcrowley/go-librato"
	"time"
)

func New(user, token, name string) librato.Metrics {
	return librato.NewSimpleMetrics(user, token, name)
}

func Duration(m librato.Metrics, name string, f func()) {
	g := m.GetGauge(name)
	defer func(start time.Time) {
		g <- int64(time.Since(start).Seconds() * 1000)
	}(time.Now())
	f()
}

func Gauge(m librato.Metrics, start time.Time, name string) {
	g := m.GetGauge(name)
	g <- int64(time.Since(start).Seconds() * 1000)
}

func Count(m librato.Metrics, count int64, name string) {
	c := m.GetCounter(name)
	c <- count
}
