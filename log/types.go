package log

import (
	"encoding/json"
	"strings"
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
	FatalLevel
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
	case FatalLevel:
		return "FATAL"
	}
	return "unknown"
}

func (l Level) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.String())
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

// Format is a well-known log format
type Format int

// Log formats
const (
	TextFormat Format = iota
	JSONFormat
)

func (f Format) String() string {
	switch f {
	case TextFormat:
		return "text"
	case JSONFormat:
		return "json"
	}
	return "unknown"
}

// ToFormat converts a string to a Format. It returns TextFormat if the string
// does not match any known log formats.
func ToFormat(s string) Format {
	switch strings.ToLower(s) {
	case "text":
		return TextFormat
	case "json":
		return JSONFormat
	default:
		return TextFormat
	}
}

// Contexter allows structs to export a key-value pairs in the form of a Context
type Contexter interface {
	// Context returns the object context as key-value pairs
	Context() Context
}

// Context represents an object's state in the form of key-value pairs
type Context map[string]any

type levelOverride struct {
	value any
	level Level
}
