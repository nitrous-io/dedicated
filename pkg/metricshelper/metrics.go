// metricshelper
//
// store a couple of commonly-used functions that help us write metrics
// collection routine
package metricshelper

import (
	"fmt"
	"time"

	"github.com/rcrowley/go-metrics"
)

// Timer returns a timer associate with name (which can be initialized before)
func Timer(name string) metrics.Timer {
	t := metrics.GetOrRegister(name, metrics.NewTimer()).(metrics.Timer)
	return t
}

// Timer returns a timer (which can be initialized before), that we can use to
// measure duration of an action
func Counter(name string) metrics.Counter {
	c := metrics.GetOrRegister(name, metrics.NewCounter()).(metrics.Counter)
	return c
}

// Error increment the error count and return that error. It's a helpful
// replacement for `return err`
func Error(err error, name string) error {
	Counter(fmt.Sprintf("error.%s", name)).Inc(1)
	return err
}

// UpdateSince update the named timer with duration since the time specified
func UpdateSince(name string, start time.Time) {
	t := Timer(name)
	t.UpdateSince(start)
}

// Time update the named timer with duration of the function
func Time(name string, f func()) {
	t := Timer(name)
	t.Time(f)
}
