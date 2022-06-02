package log

import (
	"log"
	"strings"
	"sync"
)

// Level is a well-known log level, as defined below
type Level int

// Well known log levels
const (
	TraceLevel Level = iota
	DebugLevel
	InfoLevel
	WarnLevel
	ErrorLevel
)

func (l Level) String() string {
	switch l {
	case TraceLevel:
		return "TRACE"
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	}
	return "unknown"
}

var (
	level = InfoLevel
	mu    = &sync.Mutex{}
)

// Trace prints the given message, if the current log level is TRACE
func Trace(message string, v ...interface{}) {
	logIf(TraceLevel, message, v...)
}

// Debug prints the given message, if the current log level is DEBUG or lower
func Debug(message string, v ...interface{}) {
	logIf(DebugLevel, message, v...)
}

// Info prints the given message, if the current log level is INFO or lower
func Info(message string, v ...interface{}) {
	logIf(InfoLevel, message, v...)
}

// Warn prints the given message, if the current log level is WARN or lower
func Warn(message string, v ...interface{}) {
	logIf(WarnLevel, message, v...)
}

// Error prints the given message, if the current log level is ERROR or lower
func Error(message string, v ...interface{}) {
	logIf(ErrorLevel, message, v...)
}

// Fatal prints the given message, and exits the program
func Fatal(v ...interface{}) {
	log.Fatalln(v...)
}

// CurrentLevel returns the current log level
func CurrentLevel() Level {
	mu.Lock()
	defer mu.Unlock()
	return level
}

// SetLevel sets a new log level
func SetLevel(newLevel Level) {
	mu.Lock()
	defer mu.Unlock()
	level = newLevel
}

// ToLevel converts a string to a Level. It returns InfoLevel if the string
// does not match any known log levels.
func ToLevel(s string) Level {
	switch strings.ToUpper(s) {
	case "TRACE":
		return TraceLevel
	case "DEBUG":
		return DebugLevel
	case "INFO":
		return InfoLevel
	case "WARN", "WARNING":
		return WarnLevel
	case "ERROR":
		return ErrorLevel
	default:
		return InfoLevel
	}
}

// Loggable returns true if the given log level is lower or equal to the current log level
func Loggable(l Level) bool {
	return CurrentLevel() <= l
}

// IsTrace returns true if the current log level is TraceLevel
func IsTrace() bool {
	return Loggable(TraceLevel)
}

// IsDebug returns true if the current log level is DebugLevel or below
func IsDebug() bool {
	return Loggable(DebugLevel)
}

func logIf(l Level, message string, v ...interface{}) {
	if CurrentLevel() <= l {
		log.Printf(l.String()+" "+message, v...)
	}
}
