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
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

func (l Level) String() string {
	switch l {
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

// Debug prints the given message, if the current log level is DEBUG
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
	switch strings.ToLower(s) {
	case "debug":
		return DebugLevel
	case "info":
		return InfoLevel
	case "warn", "warning":
		return WarnLevel
	case "error":
		return ErrorLevel
	default:
		return InfoLevel
	}
}

func logIf(l Level, message string, v ...interface{}) {
	if CurrentLevel() <= l {
		log.Printf(l.String()+" "+message, v...)
	}
}
