package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

var logger *Logger

type Logger struct {
	base  *log.Logger
	level LogLevel
	color bool
}

func NewLogger(output io.Writer, level LogLevel) *Logger {
	useColor := false
	if f, ok := output.(*os.File); ok {
		if fi, err := f.Stat(); err == nil {
			useColor = (fi.Mode() & os.ModeCharDevice) != 0
		}
	}
	return &Logger{
		base:  log.New(output, "", log.LstdFlags),
		level: level,
		color: useColor,
	}
}

func (l *Logger) write(level, color, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if l.color {
		l.base.Printf("%s[%s]\x1b[0m %s", color, level, msg)
		return
	}
	l.base.Printf("[%s] %s", level, msg)
}

func (l *Logger) Debug(format string, args ...any) {
	if l.level <= LevelDebug {
		l.write("DEBUG", "\x1b[34;20m", format, args...)
	}
}

func (l *Logger) Info(format string, args ...any) {
	if l.level <= LevelInfo {
		l.write("INFO", "\x1b[32;20m", format, args...)
	}
}

func (l *Logger) Warn(format string, args ...any) {
	if l.level <= LevelWarn {
		l.write("WARNING", "\x1b[33;20m", format, args...)
	}
}

func (l *Logger) Error(format string, args ...any) {
	if l.level <= LevelError {
		l.write("ERROR", "\x1b[31;20m", format, args...)
	}
}

func parseLogLevel(level string) LogLevel {
	switch strings.ToLower(level) {
	case "debug":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}
