//go:generate mockgen -source=logger.go -destination=mocks/logger.go -package=gocronmocks
package gocron

import (
	"fmt"
	"log"
	"strings"
)

// Logger is the interface that wraps the basic logging methods
// used by gocron. The methods are modeled after the standard
// library slog package. The default logger is a no-op logger.
// To enable logging, use one of the provided New*Logger functions
// or implement your own Logger. The actual level of Log that is logged
// is handled by the implementation.
type Logger interface {
	Debug(msg string, args ...any)
	Error(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
}

var _ Logger = (*noOpLogger)(nil)

type noOpLogger struct{}

func (l noOpLogger) Debug(_ string, _ ...any) {}
func (l noOpLogger) Error(_ string, _ ...any) {}
func (l noOpLogger) Info(_ string, _ ...any)  {}
func (l noOpLogger) Warn(_ string, _ ...any)  {}

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

func (l *logger) Debug(msg string, args ...any) {
	if l.level < LogLevelDebug {
		return
	}
	log.Printf("DEBUG: %s%s\n", msg, logFormatArgs(args...))
}

func (l *logger) Error(msg string, args ...any) {
	if l.level < LogLevelError {
		return
	}
	log.Printf("ERROR: %s%s\n", msg, logFormatArgs(args...))
}

func (l *logger) Info(msg string, args ...any) {
	if l.level < LogLevelInfo {
		return
	}
	log.Printf("INFO: %s%s\n", msg, logFormatArgs(args...))
}

func (l *logger) Warn(msg string, args ...any) {
	if l.level < LogLevelWarn {
		return
	}
	log.Printf("WARN: %s%s\n", msg, logFormatArgs(args...))
}

func logFormatArgs(args ...any) string {
	if len(args) == 0 {
		return ""
	}
	if len(args)%2 != 0 {
		return ", " + fmt.Sprint(args...)
	}
	var pairs []string
	for i := 0; i < len(args); i += 2 {
		pairs = append(pairs, fmt.Sprintf("%s=%v", args[i], args[i+1]))
	}
	return ", " + strings.Join(pairs, ", ")
}
