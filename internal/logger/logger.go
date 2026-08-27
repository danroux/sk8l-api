// Package logger provides zerolog configuration and the Badger logger adapter.
package logger

import (
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// ParseLogLevel maps a LOG_LEVEL string to a zerolog.Level.
// Recognized values (case-insensitive): trace, debug, info, warn, warning,
// error, disabled, off. Returns zerolog.InfoLevel for any unrecognized or
// empty value.
func ParseLogLevel(level string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "disabled", "off":
		return zerolog.Disabled
	default:
		// Covers "info" and any unrecognized value.
		return zerolog.InfoLevel
	}
}

// SetupZeroLog configures the global zerolog logger. The log level is read
// from the LOG_LEVEL environment variable and defaults to info when unset.
func SetupZeroLog() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.TimestampFieldName = "t"
	zerolog.LevelFieldName = "l"
	zerolog.MessageFieldName = "m"

	level := ParseLogLevel(os.Getenv("LOG_LEVEL"))
	zerolog.SetGlobalLevel(level)

	log.Info().Str("log_level", level.String()).Msg("logger initialized")
}

type StandardZeroLogger struct {
	zerolog.Logger
}

func NewBadgerLogger(level zerolog.Level) *StandardZeroLogger {
	x := log.With().Str("component", "badger").Logger().Level(level)
	return &StandardZeroLogger{
		Logger: x,
	}
}

func (l *StandardZeroLogger) Errorf(f string, v ...any) {
	if l.GetLevel() <= zerolog.ErrorLevel {
		l.Printf("ERROR: "+f, v...)
	}
}

func (l *StandardZeroLogger) Warningf(f string, v ...any) {
	if l.GetLevel() <= zerolog.WarnLevel {
		l.Printf("WARNING: "+f, v...)
	}
}

func (l *StandardZeroLogger) Infof(f string, v ...any) {
	if l.GetLevel() <= zerolog.InfoLevel {
		l.Printf("INFO: "+f, v...)
	}
}

func (l *StandardZeroLogger) Debugf(f string, v ...any) {
	if l.GetLevel() <= zerolog.DebugLevel {
		l.Printf("DEBUG: "+f, v...)
	}
}
