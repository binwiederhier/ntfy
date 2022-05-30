package log

import (
	"log"
	"strings"
)

type Level int

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
)

func Debug(message string, v ...interface{}) {
	logIf(DebugLevel, message, v...)
}

func Info(message string, v ...interface{}) {
	logIf(InfoLevel, message, v...)
}

func Warn(message string, v ...interface{}) {
	logIf(WarnLevel, message, v...)
}

func Error(message string, v ...interface{}) {
	logIf(ErrorLevel, message, v...)
}

func Fatal(v ...interface{}) {
	log.Fatalln(v...)
}

func SetLevel(newLevel Level) {
	level = newLevel
}

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
		log.Fatalf("unknown log level: %s", s)
		return 0
	}
}

func logIf(l Level, message string, v ...interface{}) {
	if level <= l {
		log.Printf(l.String()+" "+message, v...)
	}
}
