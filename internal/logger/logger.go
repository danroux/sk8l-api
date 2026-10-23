// Package logger provides zerolog configuration.
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
