package gocron

import (
	"bytes"
	"log"
	"strings"
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
		{
			"Less than error",
			-1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var results bytes.Buffer
			l := &logger{
				level: tt.level,
				log:   log.New(&results, "", log.LstdFlags),
			}

			var noArgs []any
			oneArg := []any{"arg1"}
			twoArgs := []any{"arg1", "arg2"}
			var noArgsStr []string
			oneArgStr := []string{"arg1"}
			twoArgsStr := []string{"arg1", "arg2"}

			for _, args := range []struct {
				argsAny []any
				argsStr []string
			}{
				{noArgs, noArgsStr},
				{oneArg, oneArgStr},
				{twoArgs, twoArgsStr},
			} {
				l.Debug("debug", args.argsAny...)
				if tt.level >= LogLevelDebug {
					r := results.String()
					assert.Contains(t, r, "DEBUG: debug")
					assert.Contains(t, r, strings.Join(args.argsStr, "="))
				} else {
					assert.Empty(t, results.String())
				}
				results.Reset()

				l.Info("info", args.argsAny...)
				if tt.level >= LogLevelInfo {
					r := results.String()
					assert.Contains(t, r, "INFO: info")
					assert.Contains(t, r, strings.Join(args.argsStr, "="))
				} else {
					assert.Empty(t, results.String())
				}
				results.Reset()

				l.Warn("warn", args.argsAny...)
				if tt.level >= LogLevelWarn {
					r := results.String()
					assert.Contains(t, r, "WARN: warn")
					assert.Contains(t, r, strings.Join(args.argsStr, "="))
				} else {
					assert.Empty(t, results.String())
				}
				results.Reset()

				l.Error("error", args.argsAny...)
				if tt.level >= LogLevelError {
					r := results.String()
					assert.Contains(t, r, "ERROR: error")
					assert.Contains(t, r, strings.Join(args.argsStr, "="))
				} else {
					assert.Empty(t, results.String())
				}
				results.Reset()
			}
		})
	}
}
