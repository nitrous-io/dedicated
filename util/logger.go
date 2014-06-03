package util

import (
	"fmt"
	"log"
	"os"
	"time"
)

type Logger struct {
	Level int
}

const (
	LogLevelQuiet = iota
	LogLevelUnknown
	LogLevelFatal
	LogLevelError
	LogLevelWarn
	LogLevelInfo
	LogLevelDebug
)

func (self *Logger) output(level int, tag string, v ...interface{}) {
	if self.Level >= level {
		w := append([]interface{}{tag}, v...)
		log.Println(w...)
	}
}

func (self *Logger) outputf(level int, tag string, format string, v ...interface{}) {
	if self.Level >= level {
		log.Printf(fmt.Sprintf("%v %v\n", tag, format), v...)
	}
}

func (self *Logger) Debug(v ...interface{}) {
	self.output(LogLevelDebug, "[DEBUG]", v...)
}

func (self *Logger) Debugf(format string, v ...interface{}) {
	self.outputf(LogLevelDebug, "[DEBUG]", format, v...)
}

func (self *Logger) Info(v ...interface{}) {
	self.output(LogLevelInfo, "[INFO]", v...)
}

func (self *Logger) Infof(format string, v ...interface{}) {
	self.outputf(LogLevelInfo, "[INFO]", format, v...)
}

func (self *Logger) Warn(v ...interface{}) {
	self.output(LogLevelWarn, "[WARN]", v...)
}

func (self *Logger) Warnf(format string, v ...interface{}) {
	self.outputf(LogLevelWarn, "[WARN]", format, v...)
}

func (self *Logger) Error(v ...interface{}) {
	self.output(LogLevelError, "[ERROR]", v...)
}

func (self *Logger) Errorf(format string, v ...interface{}) {
	self.outputf(LogLevelError, "[ERROR]", format, v...)
}

func (self *Logger) Fatal(v ...interface{}) {
	self.output(LogLevelFatal, "[FATAL]", v...)
}

func (self *Logger) Fatalf(format string, v ...interface{}) {
	self.outputf(LogLevelFatal, "[FATAL]", format, v...)
}

func (self *Logger) Unknown(v ...interface{}) {
	self.output(LogLevelUnknown, "[UNKNOWN]", v...)
}

func (self *Logger) Unknownf(format string, v ...interface{}) {
	self.outputf(LogLevelUnknown, "[UNKNOWN]", format, v...)
}

func (self *Logger) FatalExit(v ...interface{}) {
	self.Fatal(v...)
	os.Exit(1)
}

func (self *Logger) FatalfExit(format string, v ...interface{}) {
	self.Fatalf(format, v...)
	os.Exit(1)
}

func (self *Logger) TimeInfo(start time.Time, v ...interface{}) {
	elapsed := time.Since(start)
	self.Info(append(v, elapsed)...)
}
