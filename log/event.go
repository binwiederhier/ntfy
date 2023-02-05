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
	tagField   = "tag"
	errorField = "error"
)

type Event struct {
	Time    int64  `json:"time"`
	Level   Level  `json:"level"`
	Message string `json:"message"`
	fields  map[string]any
}

func newEvent() *Event {
	return &Event{
		Time:   time.Now().UnixMilli(),
		fields: make(map[string]any),
	}
}

func (e *Event) Fatal(message string, v ...any) {
	e.Log(FatalLevel, message, v...)
	os.Exit(1)
}

func (e *Event) Error(message string, v ...any) {
	e.Log(ErrorLevel, message, v...)
}

func (e *Event) Warn(message string, v ...any) {
	e.Log(WarnLevel, message, v...)
}

func (e *Event) Info(message string, v ...any) {
	e.Log(InfoLevel, message, v...)
}

func (e *Event) Debug(message string, v ...any) {
	e.Log(DebugLevel, message, v...)
}

func (e *Event) Trace(message string, v ...any) {
	e.Log(TraceLevel, message, v...)
}

func (e *Event) Tag(tag string) *Event {
	e.fields[tagField] = tag
	return e
}

func (e *Event) Err(err error) *Event {
	e.fields[errorField] = err
	return e
}

func (e *Event) Field(key string, value any) *Event {
	e.fields[key] = value
	return e
}

func (e *Event) Fields(fields map[string]any) *Event {
	for k, v := range fields {
		e.fields[k] = v
	}
	return e
}

func (e *Event) Context(contexts ...Contexter) *Event {
	for _, c := range contexts {
		e.Fields(c.Context())
	}
	return e
}

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

func (e *Event) JSON() string {
	b, _ := json.Marshal(e)
	s := string(b)
	if len(e.fields) > 0 {
		b, _ := json.Marshal(e.fields)
		s = fmt.Sprintf("{%s,%s}", s[1:len(s)-1], string(b[1:len(b)-1]))
	}
	return s
}

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
