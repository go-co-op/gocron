package gocron

import (
	"log"
)

// Logger is the interface that wraps the basic logging methods
// used by gocron. The methods are modeled after the standard
// library slog package. The default logger is a no-op logger.
// To enable logging, use one of the provided New*Logger functions
// or implement your own Logger. The actual level of Log that is logged
// is handled by the implementation.
type Logger interface {
	Debug(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
}

var _ Logger = (*noOpLogger)(nil)

type noOpLogger struct{}

func (l noOpLogger) Debug(_ string, _ ...interface{}) {}
func (l noOpLogger) Error(_ string, _ ...interface{}) {}
func (l noOpLogger) Info(_ string, _ ...interface{})  {}
func (l noOpLogger) Warn(_ string, _ ...interface{})  {}

var _ Logger = (*logger)(nil)

// LogLevel is the level of logging that should be logged
// when using the basic NewLogger.
type LogLevel int

const (
	LogLevelError LogLevel = iota
	LogLevelWarn
	LogLevelInfo
	LogLevelDebug
)

type logger struct {
	level LogLevel
}

// NewLogger returns a new Logger that logs at the given level.
func NewLogger(level LogLevel) Logger {
	return &logger{level: level}
}

func (l *logger) Debug(msg string, args ...interface{}) {
	if l.level < LogLevelDebug {
		return
	}
	if len(args) == 0 {
		log.Printf("DEBUG: %s\n", msg)
		return
	}
	log.Printf("DEBUG: %s, %v\n", msg, args)
}

func (l *logger) Error(msg string, args ...interface{}) {
	if l.level < LogLevelError {
		return
	}
	if len(args) == 0 {
		log.Printf("ERROR: %s\n", msg)
		return
	}
	log.Printf("ERROR: %s, %v\n", msg, args)
}

func (l *logger) Info(msg string, args ...interface{}) {
	if l.level < LogLevelInfo {
		return
	}
	if len(args) == 0 {
		log.Printf("INFO: %s\n", msg)
		return
	}
	log.Printf("INFO: %s, %v\n", msg, args)
}

func (l *logger) Warn(msg string, args ...interface{}) {
	if l.level < LogLevelWarn {
		return
	}
	if len(args) == 0 {
		log.Printf("WARN: %s\n", msg)
		return
	}
	log.Printf("WARN: %s, %v\n", msg, args)
}
