package quota

import "time"

// dayFor returns the usage-day bucket for the given time: the calendar date (UTC) shifted so
// that the day rolls over at the reset wall-clock time instead of midnight. resetTime carries
// only a wall-clock time of day (like the server's visitor-stats-reset-time).
func dayFor(now time.Time, resetTime time.Time) string {
	offset := time.Duration(resetTime.Hour())*time.Hour + time.Duration(resetTime.Minute())*time.Minute + time.Duration(resetTime.Second())*time.Second
	return now.UTC().Add(-offset).Format("2006-01-02")
}
