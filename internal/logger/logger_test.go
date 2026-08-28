package logger

import (
	"os"
	"testing"

	"github.com/rs/zerolog"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected zerolog.Level
	}{
		// Exact lower-case matches.
		{"trace", zerolog.TraceLevel},
		{"debug", zerolog.DebugLevel},
		{"info", zerolog.InfoLevel},
		{"warn", zerolog.WarnLevel},
		{"warning", zerolog.WarnLevel},
		{"error", zerolog.ErrorLevel},
		{"disabled", zerolog.Disabled},
		{"off", zerolog.Disabled},

		// Case variations.
		{"DEBUG", zerolog.DebugLevel},
		{"INFO", zerolog.InfoLevel},
		{"WARN", zerolog.WarnLevel},
		{"WARNING", zerolog.WarnLevel},
		{"ERROR", zerolog.ErrorLevel},
		{"DISABLED", zerolog.Disabled},
		{"OFF", zerolog.Disabled},
		{"Warn", zerolog.WarnLevel},
		{"Trace", zerolog.TraceLevel},

		// Surrounding whitespace.
		{"  debug  ", zerolog.DebugLevel},
		{"  INFO  ", zerolog.InfoLevel},

		// Unknown / empty values default to info.
		{"", zerolog.InfoLevel},
		{"verbose", zerolog.InfoLevel},
		{"42", zerolog.InfoLevel},
		{"fatal", zerolog.InfoLevel},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := ParseLogLevel(tc.input)
			if got != tc.expected {
				t.Errorf("ParseLogLevel(%q) = %v, want %v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestSetupZeroLog_DefaultsToInfo(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")
	SetupZeroLog()
	if zerolog.GlobalLevel() != zerolog.InfoLevel {
		t.Errorf("expected InfoLevel when LOG_LEVEL is empty, got %v", zerolog.GlobalLevel())
	}
}

func TestSetupZeroLog_ReadsLogLevelEnv(t *testing.T) {
	tests := []struct {
		envValue string
		expected zerolog.Level
	}{
		{"debug", zerolog.DebugLevel},
		{"warn", zerolog.WarnLevel},
		{"error", zerolog.ErrorLevel},
		{"trace", zerolog.TraceLevel},
		{"disabled", zerolog.Disabled},
	}

	for _, tc := range tests {
		t.Run(tc.envValue, func(t *testing.T) {
			t.Setenv("LOG_LEVEL", tc.envValue)
			SetupZeroLog()
			if got := zerolog.GlobalLevel(); got != tc.expected {
				t.Errorf("LOG_LEVEL=%q: expected level %v, got %v", tc.envValue, tc.expected, got)
			}
		})
	}

	// Restore to a sane level after the test suite so other tests are unaffected.
	t.Cleanup(func() {
		os.Unsetenv("LOG_LEVEL")
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	})
}
