package logger

import (
	"log"
	"os"
	"strings"
)

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

type Logger struct {
	level LogLevel
	debug *log.Logger
	info  *log.Logger
	warn  *log.Logger
	error *log.Logger
	fatal *log.Logger
}

// Log is the exported, initialized logger instance
var Log *Logger

// init function initializes Log with the log level from LOG_LEVEL environment variable
func init() {
	level := parseLogLevelFromEnv()
	Log = NewLogger(level)
}

// parseLogLevelFromEnv reads the LOG_LEVEL environment variable and returns the corresponding LogLevel.
// Defaults to INFO if LOG_LEVEL is unset or invalid.
func parseLogLevelFromEnv() LogLevel {
	logLevelStr := os.Getenv("LOG_LEVEL")
	switch strings.ToUpper(logLevelStr) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN":
		return WARN
	case "ERROR":
		return ERROR
	case "FATAL":
		return FATAL
	default:
		return INFO // Default to INFO if LOG_LEVEL is not set or invalid
	}
}

func NewLogger(level LogLevel) *Logger {
	return &Logger{
		level: level,
		debug: log.New(os.Stdout, "DEBUG: ", log.Ldate|log.Ltime),
		info:  log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime),
		warn:  log.New(os.Stdout, "WARN: ", log.Ldate|log.Ltime),
		error: log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime),
		fatal: log.New(os.Stderr, "FATAL: ", log.Ldate|log.Ltime),
	}
}

// formatMessage formats the message with an optional prefix
func formatMessage(prefix, msg string) string {
	if prefix != "" {
		return "[" + prefix + "] " + msg
	}
	return msg
}

// Debug logs debug messages with an optional prefix if the level is set to DEBUG or lower
func (l *Logger) Debug(msg string, v ...interface{}) {
	l.DebugWithPrefix("", msg, v...)
}

// DebugWithPrefix logs debug messages with a specific prefix
func (l *Logger) DebugWithPrefix(prefix, msg string, v ...interface{}) {
	if l.level <= DEBUG {
		l.debug.Printf(formatMessage(prefix, msg), v...)
	}
}

// Info logs informational messages with an optional prefix if the level is set to INFO or lower
func (l *Logger) Info(msg string, v ...interface{}) {
	l.InfoWithPrefix("", msg, v...)
}

// InfoWithPrefix logs informational messages with a specific prefix
func (l *Logger) InfoWithPrefix(prefix, msg string, v ...interface{}) {
	if l.level <= INFO {
		l.info.Printf(formatMessage(prefix, msg), v...)
	}
}

// Warn logs warning messages with an optional prefix if the level is set to WARN or lower
func (l *Logger) Warn(msg string, v ...interface{}) {
	l.WarnWithPrefix("", msg, v...)
}

// WarnWithPrefix logs warning messages with a specific prefix
func (l *Logger) WarnWithPrefix(prefix, msg string, v ...interface{}) {
	if l.level <= WARN {
		l.warn.Printf(formatMessage(prefix, msg), v...)
	}
}

// Error logs error messages with an optional prefix if the level is set to ERROR or lower
func (l *Logger) Error(msg string, v ...interface{}) {
	l.ErrorWithPrefix("", msg, v...)
}

// ErrorWithPrefix logs error messages with a specific prefix
func (l *Logger) ErrorWithPrefix(prefix, msg string, v ...interface{}) {
	if l.level <= ERROR {
		l.error.Printf(formatMessage(prefix, msg), v...)
	}
}

// Fatal logs fatal messages and exits the program
func (l *Logger) Fatal(msg string, v ...interface{}) {
	l.FatalWithPrefix("", msg, v...)
}

// FatalWithPrefix logs fatal messages with a specific prefix and exits the program
func (l *Logger) FatalWithPrefix(prefix, msg string, v ...interface{}) {
	if l.level <= FATAL {
		l.fatal.Printf(formatMessage(prefix, msg), v...)
		os.Exit(1) // Exit the program with a non-zero status code
	}
}

// Wrapper functions to simplify logging with optional prefix

func Debug(msg string, v ...interface{}) {
	Log.Debug(msg, v...)
}

func DebugWithPrefix(prefix, msg string, v ...interface{}) {
	Log.DebugWithPrefix(prefix, msg, v...)
}

func Info(msg string, v ...interface{}) {
	Log.Info(msg, v...)
}

func InfoWithPrefix(prefix, msg string, v ...interface{}) {
	Log.InfoWithPrefix(prefix, msg, v...)
}

func Warn(msg string, v ...interface{}) {
	Log.Warn(msg, v...)
}

func WarnWithPrefix(prefix, msg string, v ...interface{}) {
	Log.WarnWithPrefix(prefix, msg, v...)
}

func Error(msg string, v ...interface{}) {
	Log.Error(msg, v...)
}

func ErrorWithPrefix(prefix, msg string, v ...interface{}) {
	Log.ErrorWithPrefix(prefix, msg, v...)
}

func Fatal(msg string, v ...interface{}) {
	Log.Fatal(msg, v...)
}

func FatalWithPrefix(prefix, msg string, v ...interface{}) {
	Log.FatalWithPrefix(prefix, msg, v...)
}
