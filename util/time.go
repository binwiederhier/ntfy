package util

import (
	"errors"
	"github.com/olebedev/when"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	errUnparsableTime = errors.New("unable to parse time")
	durationStrRegex  = regexp.MustCompile(`(?i)^(\d+)\s*(d|days?|h|hours?|m|mins?|minutes?|s|secs?|seconds?)$`)
)

const (
	timestampFormat = "2006-01-02T15:04:05.999Z07:00" // Like RFC3339, but with milliseconds
)

// FormatTime formats a time.Time in a RFC339-like format that includes milliseconds
func FormatTime(t time.Time) string {
	return t.Format(timestampFormat)
}

// NextOccurrenceUTC takes a time of day (e.g. 9:00am), and returns the next occurrence
// of that time from the current time (in UTC).
func NextOccurrenceUTC(timeOfDay, base time.Time) time.Time {
	hour, minute, seconds := timeOfDay.UTC().Clock()
	now := base.UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, seconds, 0, time.UTC)
	if next.Before(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// ParseFutureTime parses a date/time string to a time.Time. It supports unix timestamps, durations
// and natural language dates
func ParseFutureTime(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	t, err := parseUnixTime(s, now)
	if err == nil {
		return t, nil
	}
	t, err = parseFromDuration(s, now)
	if err == nil {
		return t, nil
	}
	t, err = parseNaturalTime(s, now)
	if err == nil {
		return t, nil
	}
	return time.Time{}, errUnparsableTime
}

// ParseDuration is like time.ParseDuration, except that it also understands days (d), which
// translates to 24 hours, e.g. "2d" or "20h".
func ParseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}
	matches := durationStrRegex.FindStringSubmatch(s)
	if matches != nil {
		number, err := strconv.Atoi(matches[1])
		if err != nil {
			return 0, errUnparsableTime
		}
		switch unit := matches[2][0:1]; unit {
		case "d":
			return time.Duration(number) * 24 * time.Hour, nil
		case "h":
			return time.Duration(number) * time.Hour, nil
		case "m":
			return time.Duration(number) * time.Minute, nil
		case "s":
			return time.Duration(number) * time.Second, nil
		default:
			return 0, errUnparsableTime
		}
	}
	return 0, errUnparsableTime
}

func FormatDuration(d time.Duration) string {
	if d >= 24*time.Hour {
		return strconv.Itoa(int(d/(24*time.Hour))) + "d"
	}
	if d >= time.Hour {
		return strconv.Itoa(int(d/time.Hour)) + "h"
	}
	if d >= time.Minute {
		return strconv.Itoa(int(d/time.Minute)) + "m"
	}
	if d >= time.Second {
		return strconv.Itoa(int(d/time.Second)) + "s"
	}
	return "0s"
}

func parseFromDuration(s string, now time.Time) (time.Time, error) {
	d, err := ParseDuration(s)
	if err == nil {
		return now.Add(d), nil
	}
	return time.Time{}, errUnparsableTime
}

func parseUnixTime(s string, now time.Time) (time.Time, error) {
	t, err := strconv.Atoi(s)
	if err != nil {
		return time.Time{}, err
	} else if int64(t) < now.Unix() {
		return time.Time{}, errUnparsableTime
	}
	return time.Unix(int64(t), 0).UTC(), nil
}

func parseNaturalTime(s string, now time.Time) (time.Time, error) {
	r, err := when.EN.Parse(s, now) // returns "nil, nil" if no matches!
	if err != nil || r == nil {
		return time.Time{}, errUnparsableTime
	} else if r.Time.After(now) {
		return r.Time, nil
	}
	// Hack: If the time is parsable, but not in the future,
	// simply append "tomorrow, " to it.
	r, err = when.EN.Parse("tomorrow, "+s, now) // returns "nil, nil" if no matches!
	if err != nil || r == nil {
		return time.Time{}, errUnparsableTime
	} else if r.Time.After(now) {
		return r.Time, nil
	}
	return time.Time{}, errUnparsableTime
}
