package util

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gabriel-vasile/mimetype"
	"golang.org/x/term"
	"io"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	randomStringCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

var (
	random             = rand.New(rand.NewSource(time.Now().UnixNano()))
	randomMutex        = sync.Mutex{}
	sizeStrRegex       = regexp.MustCompile(`(?i)^(\d+)([gmkb])?$`)
	errInvalidPriority = errors.New("invalid priority")
	noQuotesRegex      = regexp.MustCompile(`^[-_./:@a-zA-Z0-9]+$`)
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

// SplitKV splits a string into a key/value pair using a separator, and trimming space. If the separator
// is not found, key is empty.
func SplitKV(s string, sep string) (key string, value string) {
	kv := strings.SplitN(strings.TrimSpace(s), sep, 2)
	if len(kv) == 2 {
		return strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
	}
	return "", strings.TrimSpace(kv[0])
}

// LastString returns the last string in a slice, or def if s is empty
func LastString(s []string, def string) string {
	if len(s) == 0 {
		return def
	}
	return s[len(s)-1]
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

// ValidRandomString returns true if the given string matches the format created by RandomString
func ValidRandomString(s string, length int) bool {
	if len(s) != length {
		return false
	}
	for _, c := range strings.Split(s, "") {
		if !strings.Contains(randomStringCharset, c) {
			return false
		}
	}
	return true
}

// ParsePriority parses a priority string into its equivalent integer value
func ParsePriority(priority string) (int, error) {
	p := strings.TrimSpace(strings.ToLower(priority))
	switch p {
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
		// Ignore new HTTP Priority header (see https://datatracker.ietf.org/doc/html/draft-ietf-httpbis-priority)
		// Cloudflare adds this to requests when forwarding to the backend (ntfy), so we just ignore it.
		if strings.HasPrefix(p, "u=") {
			return 3, nil
		}
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

// ShortTopicURL shortens the topic URL to be human-friendly, removing the http:// or https://
func ShortTopicURL(s string) string {
	return strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
}

// DetectContentType probes the byte array b and returns mime type and file extension.
// The filename is only used to override certain special cases.
func DetectContentType(b []byte, filename string) (mimeType string, ext string) {
	if strings.HasSuffix(strings.ToLower(filename), ".apk") {
		return "application/vnd.android.package-archive", ".apk"
	}
	m := mimetype.Detect(b)
	mimeType, ext = m.String(), m.Extension()
	if ext == "" {
		ext = ".bin"
	}
	return
}

// ParseSize parses a size string like 2K or 2M into bytes. If no unit is found, e.g. 123, bytes is assumed.
func ParseSize(s string) (int64, error) {
	matches := sizeStrRegex.FindStringSubmatch(s)
	if matches == nil {
		return -1, fmt.Errorf("invalid size %s", s)
	}
	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return -1, fmt.Errorf("cannot convert number %s", matches[1])
	}
	switch strings.ToUpper(matches[2]) {
	case "G":
		return int64(value) * 1024 * 1024 * 1024, nil
	case "M":
		return int64(value) * 1024 * 1024, nil
	case "K":
		return int64(value) * 1024, nil
	default:
		return int64(value), nil
	}
}

// ReadPassword will read a password from STDIN. If the terminal supports it, it will not print the
// input characters to the screen. If not, it'll just read using normal readline semantics (useful for testing).
func ReadPassword(in io.Reader) ([]byte, error) {
	// If in is a file and a character device (a TTY), use term.ReadPassword
	if f, ok := in.(*os.File); ok {
		stat, err := f.Stat()
		if err != nil {
			return nil, err
		}
		if (stat.Mode() & os.ModeCharDevice) == os.ModeCharDevice {
			password, err := term.ReadPassword(int(f.Fd())) // This is always going to be 0
			if err != nil {
				return nil, err
			}
			return password, nil
		}
	}

	// Fallback: Manually read util \n if found, see #69 for details why this is so manual
	password := make([]byte, 0)
	buf := make([]byte, 1)
	for {
		_, err := in.Read(buf)
		if err == io.EOF || buf[0] == '\n' {
			break
		} else if err != nil {
			return nil, err
		} else if len(password) > 10240 {
			return nil, errors.New("passwords this long are not supported")
		}
		password = append(password, buf[0])
	}

	return password, nil
}

// BasicAuth encodes the Authorization header value for basic auth
func BasicAuth(user, pass string) string {
	return fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", user, pass))))
}

// MaybeMarshalJSON returns a JSON string of the given object, or "<cannot serialize>" if serialization failed.
// This is useful for logging purposes where a failure doesn't matter that much.
func MaybeMarshalJSON(v interface{}) string {
	jsonBytes, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "<cannot serialize>"
	}
	if len(jsonBytes) > 5000 {
		return string(jsonBytes)[:5000]
	}
	return string(jsonBytes)
}

// QuoteCommand combines a command array to a string, quoting arguments that need quoting.
// This function is naive, and sometimes wrong. It is only meant for lo pretty-printing a command.
//
// Warning: Never use this function with the intent to run the resulting command.
//
// Example:
//    []string{"ls", "-al", "Document Folder"} -> ls -al "Document Folder"
func QuoteCommand(command []string) string {
	var quoted []string
	for _, c := range command {
		if noQuotesRegex.MatchString(c) {
			quoted = append(quoted, c)
		} else {
			quoted = append(quoted, fmt.Sprintf(`"%s"`, c))
		}
	}
	return strings.Join(quoted, " ")
}
