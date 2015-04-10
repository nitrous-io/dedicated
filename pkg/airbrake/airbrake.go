package airbrake

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/airbrake/gobrake.v1"
)

var (
	notifier     *gobrake.Notifier
	notifierChan chan *gobrake.Notice
	wg           sync.WaitGroup
	isShutdown   int32
	logToStdout  int32 = 1
)

// Init initializes airbrake. Must be called before any other calls.
// apiKey is shown in airbrake dashboard.
// env should be "dev", "production", "staging"
// projectID is (I assume) the number in airbrake url e.g.
// https://actionio.airbrake.io/projects/109646/groups => 109646
func Init(projectID int64, apiKey, env string) {
	if notifier != nil {
		log.Printf("airbrake.Init(): initializing second time")
		return
	}
	notifier = gobrake.NewNotifier(projectID, apiKey)
	notifier.SetContext("environment", env)
	notifierChan = make(chan *gobrake.Notice, 100)
	// send notification in a goroutine so that logging doesn't block main logic
	go airbrakeNotifierWorker()
}

func SetLogToStdout(shouldLog bool) {
	if shouldLog {
		atomic.StoreInt32(&logToStdout, 1)
	} else {
		atomic.StoreInt32(&logToStdout, 0)
	}
}

func GetLogToStdout() bool {
	return atomic.LoadInt32(&logToStdout) != 0
}

func logNotice(notice *gobrake.Notice) {
	shouldLog := atomic.LoadInt32(&logToStdout)
	if shouldLog == 0 {
		return
	}
	if len(notice.Errors) == 0 {
		return
	}
	e := notice.Errors[0]
	msg := e.Message
	if len(e.Backtrace) > 0 {
		stackFrame := e.Backtrace[0]
		fileName := filepath.Base(stackFrame.File)
		log.Printf("%s:%d %s: %s", fileName, stackFrame.Line, stackFrame.Func, msg)
	} else {
		log.Print(msg)
	}
}

func resetTimeIfOlderThan(t time.Time, dur time.Duration) time.Time {
	if t.IsZero() {
		return t
	}
	passed := time.Since(t)
	if passed > dur {
		return time.Time{}
	}
	return t
}

func airbrakeNotifierWorker() {
	// throttle logging info about problems sending to airbrake. We want to know
	// sending to airbrake failed but we don't want to flood logs when it's
	// long-lasting error (either airbrake wasn't configured properly or there is
	// a network error and we can't talk to airbrake)
	var lastErrorTime time.Time
	seenError := false
	for notice := range notifierChan {
		logNotice(notice)

		lastErrorTime = resetTimeIfOlderThan(lastErrorTime, time.Minute*5)

		// allow to work even if Init() wasn't called. That way we can easily
		// enable/disable sending to airbrake without changing all the log
		// statements
		if notifier != nil {
			err := notifier.SendNotice(notice)
			if err != nil && !lastErrorTime.IsZero() {
				// don't want to flood the logs
				lastErrorTime = time.Now()
				log.Printf("notifier.SendNotice() failed with '%s'", err)
			}
		} else {
			// log error message once
			if !seenError {
				log.Printf("not sending to airbrake because airbrake.Init() wasn't called")
				seenError = true
			}
		}
		wg.Done()
	}
}

func sendNotice(notice *gobrake.Notice) {
	// don't try to send more message on notifierChan if we closed it in Shutdown()
	done := atomic.LoadInt32(&isShutdown)
	if done != 0 {
		return
	}
	wg.Add(1)
	notifierChan <- notice
}

// Shutdown ends sending of errors to airbrake
func Shutdown() {
	atomic.StoreInt32(&isShutdown, 1)
	close(notifierChan)
}

// WatiFinished can be called after Shutdown() to wait for all outstanding notices
// to be sent
func WaitFinished() {
	wg.Wait()
}

func notify(errToSend error, req *http.Request) {
	err := notifier.Notify(errToSend, req)
	if err != nil {
		log.Printf("notifier.Notify() failed with %s", err)
	}
}

// Error sends error to airbrake. req is optional, if given info
// from it will be sent to airbrake
func Err(err error, req *http.Request) {
	sendNotice(notifier.Notice(err, req, 3))
}

// Errorf sends error message to aribrake.
func Errorf(format string, args ...interface{}) {
	err := errors.New(fmt.Sprintf(format, args...))
	sendNotice(notifier.Notice(err, nil, 3))
}

// Printf is exactly like Errorf. It's here to make it easy to change
// log.Printf() => airbrake.Printf()
func Printf(format string, args ...interface{}) {
	err := errors.New(fmt.Sprintf(format, args...))
	sendNotice(notifier.Notice(err, nil, 3))
}

// LogWithReqErrorf sends error message tied to a given req to aribrake.
func LogWithReqErrorf(req *http.Request, format string, args ...interface{}) {
	err := errors.New(fmt.Sprintf(format, args...))
	sendNotice(notifier.Notice(err, nil, 3))
}

func shutdownAndQuit(msg string) {
	Shutdown()
	// ensure that we'll wait no longer than 5 seconds to quit, even if
	// WaitFinished() takes longer
	go func() {
		time.Sleep(time.Second * 5)
		log.Fatal(msg)
	}()
	WaitFinished()
	log.Fatal(msg)
}

// Fatalf sends error message to airbrake and exists the program, like log.Fatalf
func Fatalf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	sendNotice(notifier.Notice(errors.New(msg), nil, 3))
	shutdownAndQuit(msg)
}

func Fatal(v ...interface{}) {
	msg := fmt.Sprint(v...)
	sendNotice(notifier.Notice(errors.New(msg), nil, 3))
	shutdownAndQuit(msg)
}
