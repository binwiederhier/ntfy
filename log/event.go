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
	fieldTag        = "tag"
	fieldError      = "error"
	fieldTimeTaken  = "time_taken_ms"
	fieldExitCode   = "exit_code"
	tagStdLog       = "stdlog"
	timestampFormat = "2006-01-02T15:04:05.999Z07:00"
)

// Event represents a single log event
type Event struct {
	Timestamp  string `json:"time"`
	Level      Level  `json:"level"`
	Message    string `json:"message"`
	time       time.Time
	contexters []Contexter
	fields     Context
}

// newEvent creates a new log event
//
// We delay allocations and processing for efficiency, because most log events
// are never actually rendered, so we don't format the time, or allocate a fields map.
func newEvent() *Event {
	return &Event{
		time: time.Now(),
	}
}

// Fatal logs the event as FATAL, and exits the program with exit code 1
func (e *Event) Fatal(message string, v ...any) {
	e.Field(fieldExitCode, 1).maybeLog(FatalLevel, message, v...)
	fmt.Fprintf(os.Stderr, message+"\n", v...) // Always output error to stderr
	os.Exit(1)
}

// Error logs the event with log level error
func (e *Event) Error(message string, v ...any) {
	e.maybeLog(ErrorLevel, message, v...)
}

// Warn logs the event with log level warn
func (e *Event) Warn(message string, v ...any) {
	e.maybeLog(WarnLevel, message, v...)
}

// Info logs the event with log level info
func (e *Event) Info(message string, v ...any) {
	e.maybeLog(InfoLevel, message, v...)
}

// Debug logs the event with log level debug
func (e *Event) Debug(message string, v ...any) {
	e.maybeLog(DebugLevel, message, v...)
}

// Trace logs the event with log level trace
func (e *Event) Trace(message string, v ...any) {
	e.maybeLog(TraceLevel, message, v...)
}

// Tag adds a "tag" field to the log event
func (e *Event) Tag(tag string) *Event {
	return e.Field(fieldTag, tag)
}

// Time sets the time field
func (e *Event) Time(t time.Time) *Event {
	e.time = t
	return e
}

// Timing runs f and records the time if took to execute it in "time_taken_ms"
func (e *Event) Timing(f func()) *Event {
	start := time.Now()
	f()
	return e.Field(fieldTimeTaken, time.Since(start).Milliseconds())
}

// Err adds an "error" field to the log event
func (e *Event) Err(err error) *Event {
	if err == nil {
		return e
	} else if c, ok := err.(Contexter); ok {
		return e.With(c)
	}
	return e.Field(fieldError, err.Error())
}

// Field adds a custom field and value to the log event
func (e *Event) Field(key string, value any) *Event {
	if e.fields == nil {
		e.fields = make(Context)
	}
	e.fields[key] = value
	return e
}

// Fields adds a map of fields to the log event
func (e *Event) Fields(fields Context) *Event {
	if e.fields == nil {
		e.fields = make(Context)
	}
	for k, v := range fields {
		e.fields[k] = v
	}
	return e
}

// With adds the fields of the given Contexter structs to the log event by calling their Context method
func (e *Event) With(contexters ...Contexter) *Event {
	if e.contexters == nil {
		e.contexters = contexters
	} else {
		e.contexters = append(e.contexters, contexters...)
	}
	return e
}

// Render returns the rendered log event as a string, or an empty string. The event is only rendered,
// if either the global log level is >= l, or if the log level in one of the overrides matches
// the level.
//
// If no overrides are defined (default), the Contexter array is not applied unless the event
// is actually logged. If overrides are defined, then Contexters have to be applied in any case
// to determine if they match. This is super complicated, but required for efficiency.
func (e *Event) Render(l Level, message string, v ...any) string {
	appliedContexters := e.maybeApplyContexters()
	if !e.shouldLog(l) {
		return ""
	}
	e.Message = fmt.Sprintf(message, v...)
	e.Level = l
	e.Timestamp = e.time.Format(timestampFormat)
	if !appliedContexters {
		e.applyContexters()
	}
	if CurrentFormat() == JSONFormat {
		return e.JSON()
	}
	return e.String()
}

// maybeLog logs the event to the defined output, or does nothing if Render returns an empty string
func (e *Event) maybeLog(l Level, message string, v ...any) {
	if m := e.Render(l, message, v...); m != "" {
		log.Println(m)
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

func (e *Event) shouldLog(l Level) bool {
	return e.globalLevelWithOverride() <= l
}

func (e *Event) globalLevelWithOverride() Level {
	mu.RLock()
	l, ov := level, overrides
	mu.RUnlock()
	if e.fields == nil {
		return l
	}
	for field, override := range ov {
		value, exists := e.fields[field]
		if exists {
			if override.value == "" || override.value == value || override.value == fmt.Sprintf("%v", value) {
				return override.level
			}
		}
	}
	return l
}

func (e *Event) maybeApplyContexters() bool {
	mu.RLock()
	hasOverrides := len(overrides) > 0
	mu.RUnlock()
	if hasOverrides {
		e.applyContexters()
	}
	return hasOverrides // = applied
}

func (e *Event) applyContexters() {
	for _, c := range e.contexters {
		e.Fields(c.Context())
	}
}
