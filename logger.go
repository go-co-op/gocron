package gocron

import (
	"log/slog"
	"os"
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

func (_ noOpLogger) Debug(_ string, _ ...any) {}
func (_ noOpLogger) Error(_ string, _ ...any) {}
func (_ noOpLogger) Info(_ string, _ ...any)  {}
func (_ noOpLogger) Warn(_ string, _ ...any)  {}

var _ Logger = (*slogLogger)(nil)

type slogLogger struct {
	sl *slog.Logger
}

func NewJsonSlogLogger(level slog.Level) Logger {
	return NewSlogLogger(
		slog.New(
			slog.NewJSONHandler(
				os.Stdout,
				&slog.HandlerOptions{
					Level: level,
				},
			),
		),
	)
}

func NewSlogLogger(sl *slog.Logger) Logger {
	return &slogLogger{sl: sl}
}

func (l *slogLogger) Debug(msg string, args ...any) {
	l.sl.Debug(msg, args...)
}

func (l *slogLogger) Error(msg string, args ...any) {
	l.sl.Error(msg, args...)
}

func (l *slogLogger) Info(msg string, args ...any) {
	l.sl.Info(msg, args...)
}

func (l *slogLogger) Warn(msg string, args ...any) {
	l.sl.Warn(msg, args...)
}
