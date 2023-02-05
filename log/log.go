package log

import (
	"log"
	"sync"
)

var (
	level     = InfoLevel
	format    = TextFormat
	overrides = make(map[string]*levelOverride)
	mu        = &sync.Mutex{}
)

// Fatal prints the given message, and exits the program
func Fatal(message string, v ...any) {
	newEvent().Fatal(message, v...)
}

// Error prints the given message, if the current log level is ERROR or lower
func Error(message string, v ...any) {
	newEvent().Error(message, v...)
}

// Warn prints the given message, if the current log level is WARN or lower
func Warn(message string, v ...any) {
	newEvent().Warn(message, v...)
}

// Info prints the given message, if the current log level is INFO or lower
func Info(message string, v ...any) {
	newEvent().Info(message, v...)
}

// Debug prints the given message, if the current log level is DEBUG or lower
func Debug(message string, v ...any) {
	newEvent().Debug(message, v...)
}

// Trace prints the given message, if the current log level is TRACE
func Trace(message string, v ...any) {
	newEvent().Trace(message, v...)
}

func Context(contexts ...Contexter) *Event {
	return newEvent().Context(contexts...)
}

func Field(key string, value any) *Event {
	return newEvent().Field(key, value)
}

func Fields(fields map[string]any) *Event {
	return newEvent().Fields(fields)
}

func Tag(tag string) *Event {
	return newEvent().Tag(tag)
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

// SetLevelOverride adds a log override for the given field
func SetLevelOverride(field string, value any, level Level) {
	mu.Lock()
	defer mu.Unlock()
	overrides[field] = &levelOverride{value: value, level: level}
}

// ResetLevelOverride removes all log level overrides
func ResetLevelOverride() {
	mu.Lock()
	defer mu.Unlock()
	overrides = make(map[string]*levelOverride)
}

// CurrentFormat returns the current log formt
func CurrentFormat() Format {
	mu.Lock()
	defer mu.Unlock()
	return format
}

// SetFormat sets a new log format
func SetFormat(newFormat Format) {
	mu.Lock()
	defer mu.Unlock()
	format = newFormat
	if newFormat == JSONFormat {
		DisableDates()
	}
}

// DisableDates disables the date/time prefix
func DisableDates() {
	log.SetFlags(0)
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
