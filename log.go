package main

import (
	"fmt"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func setupZeroLog() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.TimestampFieldName = "t"
	zerolog.LevelFieldName = "l"
	zerolog.MessageFieldName = "m"

	log.Info().Msg(fmt.Sprintf("log_level %d", zerolog.GlobalLevel()))
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Info().Msg(fmt.Sprintf("log_level %d", zerolog.GlobalLevel()))
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
