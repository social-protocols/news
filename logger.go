package main

import (
	"log"
	"os"

	kitlog "github.com/go-kit/log"
)

func newLogger(levelString string) leveledLogger {
	// logger := kitlog.NewLogfmtLogger(kitlog.NewSyncWriter(os.Stderr))

	logger := kitlog.NewLogfmtLogger(kitlog.NewSyncWriter(os.Stdout))
	log.SetOutput(kitlog.NewStdlibAdapter(logger))

	logLevels := map[string]logLevel{
		"DEBUG": logLevelDebug,
		"INFO":  logLevelInfo,
		"WARN":  logLevelWarn,
		"ERROR": logLevelError,
	}

	l, ok := logLevels[levelString]
	if !ok {
		panic("Unrecognized log level: " + levelString)
	}

	return leveledLogger{
		logger: logger,
		level:  l,
	}
}

type leveledLogger struct {
	logger kitlog.Logger
	level  logLevel
}

type logLevel int

const (
	logLevelDebug logLevel = 0
	logLevelInfo  logLevel = 1
	logLevelWarn  logLevel = 2
	logLevelError logLevel = 3
)

func (l leveledLogger) Fatal(err error, keysAndValues ...interface{}) {
	l.Err(err, keysAndValues...)
	os.Exit(1)
}

func (l leveledLogger) Err(err error, keysAndValues ...interface{}) {
	if l.level > logLevelError {
		return
	}
	k := append(keysAndValues, "message", err.Error(), "level", "ERROR")
	l.logger.Log(k...)
}

func (l leveledLogger) Error(msg string, keysAndValues ...interface{}) {
	if l.level > logLevelError {
		return
	}
	k := append(keysAndValues, "message", msg, "level", "ERROR")
	l.logger.Log(k...)
}

func (l leveledLogger) Warn(msg string, keysAndValues ...interface{}) {
	if l.level > logLevelWarn {
		return
	}
	k := append(keysAndValues, "message", msg, "level", "WARN")
	l.logger.Log(k...)
}

func (l leveledLogger) Info(msg string, keysAndValues ...interface{}) {
	if l.level > logLevelInfo {
		return
	}
	k := append(keysAndValues, "message", msg, "level", "INFO")
	l.logger.Log(k...)
}

func (l leveledLogger) Debug(msg string, keysAndValues ...interface{}) {
	if l.level > logLevelDebug {
		return
	}
	k := append(keysAndValues, "message", msg, "level", "DEBUG")
	l.logger.Log(k...)
}
