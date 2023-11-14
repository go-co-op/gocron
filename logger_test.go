package gocron

import (
	"bytes"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoOpLogger(_ *testing.T) {
	noOp := noOpLogger{}
	noOp.Debug("debug", "arg1", "arg2")
	noOp.Error("error", "arg1", "arg2")
	noOp.Info("info", "arg1", "arg2")
	noOp.Warn("warn", "arg1", "arg2")
}

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name  string
		level LogLevel
	}{
		{
			"debug",
			LogLevelDebug,
		},
		{
			"info",
			LogLevelInfo,
		},
		{
			"warn",
			LogLevelWarn,
		},
		{
			"error",
			LogLevelError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var results bytes.Buffer
			log.SetOutput(&results)
			l := NewLogger(tt.level)

			l.Debug("debug", "arg1", "arg2")
			if tt.level >= LogLevelDebug {
				assert.Contains(t, results.String(), "DEBUG: debug, arg1=arg2\n")
			} else {
				assert.Empty(t, results.String())
			}

			l.Info("info", "arg1", "arg2")
			if tt.level >= LogLevelInfo {
				assert.Contains(t, results.String(), "INFO: info, arg1=arg2\n")
			} else {
				assert.Empty(t, results.String())
			}

			l.Warn("warn", "arg1", "arg2")
			if tt.level >= LogLevelWarn {
				assert.Contains(t, results.String(), "WARN: warn, arg1=arg2\n")
			} else {
				assert.Empty(t, results.String())
			}

			l.Error("error", "arg1", "arg2")
			if tt.level >= LogLevelError {
				assert.Contains(t, results.String(), "ERROR: error, arg1=arg2\n")
			} else {
				assert.Empty(t, results.String())
			}
		})
	}
}
