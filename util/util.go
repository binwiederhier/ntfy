package util

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	randomStringCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

var (
	random      = rand.New(rand.NewSource(time.Now().UnixNano()))
	randomMutex = sync.Mutex{}

	errInvalidPriority = errors.New("invalid priority")
)

// FileExists checks if a file exists, and returns true if it does
func FileExists(filename string) bool {
	stat, _ := os.Stat(filename)
	return stat != nil
}

// InStringList returns true if needle is contained in haystack
func InStringList(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// InStringListAll returns true if all needles are contained in haystack
func InStringListAll(haystack []string, needles []string) bool {
	matches := 0
	for _, s := range haystack {
		for _, needle := range needles {
			if s == needle {
				matches++
			}
		}
	}
	return matches == len(needles)
}

// InIntList returns true if needle is contained in haystack
func InIntList(haystack []int, needle int) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// SplitNoEmpty splits a string using strings.Split, but filters out empty strings
func SplitNoEmpty(s string, sep string) []string {
	res := make([]string, 0)
	for _, r := range strings.Split(s, sep) {
		if r != "" {
			res = append(res, r)
		}
	}
	return res
}

// RandomString returns a random string with a given length
func RandomString(length int) string {
	randomMutex.Lock() // Who would have thought that random.Intn() is not thread-safe?!
	defer randomMutex.Unlock()
	b := make([]byte, length)
	for i := range b {
		b[i] = randomStringCharset[random.Intn(len(randomStringCharset))]
	}
	return string(b)
}

// DurationToHuman converts a duration to a human readable format
func DurationToHuman(d time.Duration) (str string) {
	if d == 0 {
		return "0"
	}

	d = d.Round(time.Second)
	days := d / time.Hour / 24
	if days > 0 {
		str += fmt.Sprintf("%dd", days)
	}
	d -= days * time.Hour * 24

	hours := d / time.Hour
	if hours > 0 {
		str += fmt.Sprintf("%dh", hours)
	}
	d -= hours * time.Hour

	minutes := d / time.Minute
	if minutes > 0 {
		str += fmt.Sprintf("%dm", minutes)
	}
	d -= minutes * time.Minute

	seconds := d / time.Second
	if seconds > 0 {
		str += fmt.Sprintf("%ds", seconds)
	}
	return
}

// ParsePriority parses a priority string into its equivalent integer value
func ParsePriority(priority string) (int, error) {
	switch strings.TrimSpace(strings.ToLower(priority)) {
	case "":
		return 0, nil
	case "1", "min":
		return 1, nil
	case "2", "low":
		return 2, nil
	case "3", "default":
		return 3, nil
	case "4", "high":
		return 4, nil
	case "5", "max", "urgent":
		return 5, nil
	default:
		return 0, errInvalidPriority
	}
}

// PriorityString converts a priority number to a string
func PriorityString(priority int) (string, error) {
	switch priority {
	case 0:
		return "default", nil
	case 1:
		return "min", nil
	case 2:
		return "low", nil
	case 3:
		return "default", nil
	case 4:
		return "high", nil
	case 5:
		return "max", nil
	default:
		return "", errInvalidPriority
	}
}

// ExpandHome replaces "~" with the user's home directory
func ExpandHome(path string) string {
	return os.ExpandEnv(strings.ReplaceAll(path, "~", "$HOME"))
}

// ShortTopicURL shortens the topic URL to be human-friendly, removing the http:// or https://
func ShortTopicURL(s string) string {
	return strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
}
