package sprig

import (
	"math"
	"strconv"
	"time"
)

// date formats a date according to the provided format string.
//
// Parameters:
//   - fmt: A Go time format string (e.g., "2006-01-02 15:04:05")
//   - date: Can be a time.Time, *time.Time, or int/int32/int64 (seconds since UNIX epoch)
//
// If date is not one of the recognized types, the current time is used.
//
// Example usage in templates: {{ now | date "2006-01-02" }}
func date(fmt string, date any) string {
	return dateInZone(fmt, date, "Local")
}

// htmlDate formats a date in HTML5 date format (YYYY-MM-DD).
//
// Parameters:
//   - date: Can be a time.Time, *time.Time, or int/int32/int64 (seconds since UNIX epoch)
//
// If date is not one of the recognized types, the current time is used.
//
// Example usage in templates: {{ now | htmlDate }}
func htmlDate(date any) string {
	return dateInZone("2006-01-02", date, "Local")
}

// htmlDateInZone formats a date in HTML5 date format (YYYY-MM-DD) in the specified timezone.
//
// Parameters:
//   - date: Can be a time.Time, *time.Time, or int/int32/int64 (seconds since UNIX epoch)
//   - zone: Timezone name (e.g., "UTC", "America/New_York")
//
// If date is not one of the recognized types, the current time is used.
// If the timezone is invalid, UTC is used.
//
// Example usage in templates: {{ now | htmlDateInZone "UTC" }}
func htmlDateInZone(date any, zone string) string {
	return dateInZone("2006-01-02", date, zone)
}

// dateInZone formats a date according to the provided format string in the specified timezone.
//
// Parameters:
//   - fmt: A Go time format string (e.g., "2006-01-02 15:04:05")
//   - date: Can be a time.Time, *time.Time, or int/int32/int64 (seconds since UNIX epoch)
//   - zone: Timezone name (e.g., "UTC", "America/New_York")
//
// If date is not one of the recognized types, the current time is used.
// If the timezone is invalid, UTC is used.
//
// Example usage in templates: {{ now | dateInZone "2006-01-02 15:04:05" "UTC" }}
func dateInZone(fmt string, date any, zone string) string {
	var t time.Time
	switch date := date.(type) {
	default:
		t = time.Now()
	case time.Time:
		t = date
	case *time.Time:
		t = *date
	case int64:
		t = time.Unix(date, 0)
	case int:
		t = time.Unix(int64(date), 0)
	case int32:
		t = time.Unix(int64(date), 0)
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		loc, _ = time.LoadLocation("UTC")
	}
	return t.In(loc).Format(fmt)
}

// dateModify modifies a date by adding a duration and returns the resulting time.
//
// Parameters:
//   - fmt: A duration string (e.g., "24h", "-12h30m", "1h15m30s")
//   - date: The time.Time to modify
//
// If the duration string is invalid, the original date is returned.
//
// Example usage in templates: {{ now | dateModify "-24h" }}
func dateModify(fmt string, date time.Time) time.Time {
	d, err := time.ParseDuration(fmt)
	if err != nil {
		return date
	}
	return date.Add(d)
}

// mustDateModify modifies a date by adding a duration and returns the resulting time or an error.
//
// Parameters:
//   - fmt: A duration string (e.g., "24h", "-12h30m", "1h15m30s")
//   - date: The time.Time to modify
//
// Unlike dateModify, this function returns an error if the duration string is invalid.
//
// Example usage in templates: {{ now | mustDateModify "24h" }}
func mustDateModify(fmt string, date time.Time) (time.Time, error) {
	d, err := time.ParseDuration(fmt)
	if err != nil {
		return time.Time{}, err
	}
	return date.Add(d), nil
}

// dateAgo returns a string representing the time elapsed since the given date.
//
// Parameters:
//   - date: Can be a time.Time, int, or int64 (seconds since UNIX epoch)
//
// If date is not one of the recognized types, the current time is used.
//
// Example usage in templates: {{ "2023-01-01" | toDate "2006-01-02" | dateAgo }}
func dateAgo(date any) string {
	var t time.Time
	switch date := date.(type) {
	default:
		t = time.Now()
	case time.Time:
		t = date
	case int64:
		t = time.Unix(date, 0)
	case int:
		t = time.Unix(int64(date), 0)
	}
	return time.Since(t).Round(time.Second).String()
}

// duration converts seconds to a duration string.
//
// Parameters:
//   - sec: Can be a string (parsed as int64), or int64 representing seconds
//
// Example usage in templates: {{ 3600 | duration }} -> "1h0m0s"
func duration(sec any) string {
	var n int64
	switch value := sec.(type) {
	default:
		n = 0
	case string:
		n, _ = strconv.ParseInt(value, 10, 64)
	case int64:
		n = value
	}
	return (time.Duration(n) * time.Second).String()
}

// durationRound formats a duration in a human-readable rounded format.
//
// Parameters:
//   - duration: Can be a string (parsed as duration), int64 (nanoseconds),
//     or time.Time (time since that moment)
//
// Returns a string with the largest appropriate unit (y, mo, d, h, m, s).
//
// Example usage in templates: {{ 3600 | duration | durationRound }} -> "1h"
func durationRound(duration any) string {
	var d time.Duration
	switch duration := duration.(type) {
	default:
		d = 0
	case string:
		d, _ = time.ParseDuration(duration)
	case int64:
		d = time.Duration(duration)
	case time.Time:
		d = time.Since(duration)
	}
	u := uint64(math.Abs(float64(d)))
	var (
		year   = uint64(time.Hour) * 24 * 365
		month  = uint64(time.Hour) * 24 * 30
		day    = uint64(time.Hour) * 24
		hour   = uint64(time.Hour)
		minute = uint64(time.Minute)
		second = uint64(time.Second)
	)
	switch {
	case u > year:
		return strconv.FormatUint(u/year, 10) + "y"
	case u > month:
		return strconv.FormatUint(u/month, 10) + "mo"
	case u > day:
		return strconv.FormatUint(u/day, 10) + "d"
	case u > hour:
		return strconv.FormatUint(u/hour, 10) + "h"
	case u > minute:
		return strconv.FormatUint(u/minute, 10) + "m"
	case u > second:
		return strconv.FormatUint(u/second, 10) + "s"
	}
	return "0s"
}

// toDate parses a string into a time.Time using the specified format.
//
// Parameters:
//   - fmt: A Go time format string (e.g., "2006-01-02")
//   - str: The date string to parse
//
// If parsing fails, returns a zero time.Time.
//
// Example usage in templates: {{ "2023-01-01" | toDate "2006-01-02" }}
func toDate(fmt, str string) time.Time {
	t, _ := time.ParseInLocation(fmt, str, time.Local)
	return t
}

// mustToDate parses a string into a time.Time using the specified format or returns an error.
//
// Parameters:
//   - fmt: A Go time format string (e.g., "2006-01-02")
//   - str: The date string to parse
//
// Unlike toDate, this function returns an error if parsing fails.
//
// Example usage in templates: {{ mustToDate "2006-01-02" "2023-01-01" }}
func mustToDate(fmt, str string) (time.Time, error) {
	return time.ParseInLocation(fmt, str, time.Local)
}

// unixEpoch returns the Unix timestamp (seconds since January 1, 1970 UTC) for the given time.
//
// Parameters:
//   - date: A time.Time value
//
// Example usage in templates: {{ now | unixEpoch }}
func unixEpoch(date time.Time) string {
	return strconv.FormatInt(date.Unix(), 10)
}
