package log

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	tagField        = "tag"
	errorField      = "error"
	timestampFormat = "2006-01-02T15:04:05.999Z07:00"
)

// Event represents a single log event
type Event struct {
	Timestamp string `json:"time"`
	Level     Level  `json:"level"`
	Message   string `json:"message"`
	fields    Context
}

// newEvent creates a new log event
func newEvent() *Event {
	now := time.Now()
	return &Event{
		Timestamp: now.Format(timestampFormat),
		fields:    make(Context),
	}
}

// Fatal logs the event as FATAL, and exits the program with exit code 1
func (e *Event) Fatal(message string, v ...any) {
	e.Field("exit_code", 1).Log(FatalLevel, message, v...)
	fmt.Fprintf(os.Stderr, message+"\n", v...) // Always output error to stderr
	os.Exit(1)
}

// Error logs the event with log level error
func (e *Event) Error(message string, v ...any) {
	e.Log(ErrorLevel, message, v...)
}

// Warn logs the event with log level warn
func (e *Event) Warn(message string, v ...any) {
	e.Log(WarnLevel, message, v...)
}

// Info logs the event with log level info
func (e *Event) Info(message string, v ...any) {
	e.Log(InfoLevel, message, v...)
}

// Debug logs the event with log level debug
func (e *Event) Debug(message string, v ...any) {
	e.Log(DebugLevel, message, v...)
}

// Trace logs the event with log level trace
func (e *Event) Trace(message string, v ...any) {
	e.Log(TraceLevel, message, v...)
}

// Tag adds a "tag" field to the log event
func (e *Event) Tag(tag string) *Event {
	e.fields[tagField] = tag
	return e
}

// Time sets the time field
func (e *Event) Time(t time.Time) *Event {
	e.Timestamp = t.Format(timestampFormat)
	return e
}

// Err adds an "error" field to the log event
func (e *Event) Err(err error) *Event {
	if c, ok := err.(Contexter); ok {
		e.Fields(c.Context())
	} else {
		e.fields[errorField] = err.Error()
	}
	return e
}

// Field adds a custom field and value to the log event
func (e *Event) Field(key string, value any) *Event {
	e.fields[key] = value
	return e
}

// Fields adds a map of fields to the log event
func (e *Event) Fields(fields Context) *Event {
	for k, v := range fields {
		e.fields[k] = v
	}
	return e
}

// With adds the fields of the given Contexter structs to the log event by calling their With method
func (e *Event) With(contexts ...Contexter) *Event {
	for _, c := range contexts {
		e.Fields(c.Context())
	}
	return e
}

// Log logs a message with the given log level
func (e *Event) Log(l Level, message string, v ...any) {
	e.Message = fmt.Sprintf(message, v...)
	e.Level = l
	if e.shouldPrint() {
		if CurrentFormat() == JSONFormat {
			log.Println(e.JSON())
		} else {
			log.Println(e.String())
		}
	}
}

// Loggable returns true if the given log level is lower or equal to the current log level
func (e *Event) Loggable(l Level) bool {
	return e.globalLevelWithOverride() <= l
}

// IsTrace returns true if the current log level is TraceLevel
func (e *Event) IsTrace() bool {
	return e.Loggable(TraceLevel)
}

// IsDebug returns true if the current log level is DebugLevel or below
func (e *Event) IsDebug() bool {
	return e.Loggable(DebugLevel)
}

// JSON returns the event as a JSON representation
func (e *Event) JSON() string {
	b, _ := json.Marshal(e)
	s := string(b)
	if len(e.fields) > 0 {
		b, _ := json.Marshal(e.fields)
		s = fmt.Sprintf("{%s,%s}", s[1:len(s)-1], string(b[1:len(b)-1]))
	}
	return s
}

// String returns the event as a string
func (e *Event) String() string {
	if len(e.fields) == 0 {
		return fmt.Sprintf("%s %s", e.Level.String(), e.Message)
	}
	fields := make([]string, 0)
	for k, v := range e.fields {
		fields = append(fields, fmt.Sprintf("%s=%v", k, v))
	}
	sort.Strings(fields)
	return fmt.Sprintf("%s %s (%s)", e.Level.String(), e.Message, strings.Join(fields, ", "))
}

func (e *Event) shouldPrint() bool {
	return e.globalLevelWithOverride() <= e.Level
}

func (e *Event) globalLevelWithOverride() Level {
	mu.Lock()
	l, ov := level, overrides
	mu.Unlock()
	for field, override := range ov {
		value, exists := e.fields[field]
		if exists && value == override.value {
			return override.level
		}
	}
	return l
}
