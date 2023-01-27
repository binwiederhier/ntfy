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

func parseFromDuration(s string, now time.Time) (time.Time, error) {
	d, err := parseDuration(s)
	if err == nil {
		return now.Add(d), nil
	}
	return time.Time{}, errUnparsableTime
}

func parseDuration(s string) (time.Duration, error) {
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
