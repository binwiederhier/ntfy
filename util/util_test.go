package util

import (
	"errors"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/stretchr/testify/require"
)

func TestRandomString(t *testing.T) {
	s1 := RandomString(10)
	s2 := RandomString(10)
	s3 := RandomString(12)
	require.Equal(t, 10, len(s1))
	require.Equal(t, 10, len(s2))
	require.Equal(t, 12, len(s3))
	require.NotEqual(t, s1, s2)
}

func TestFileExists(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "somefile.txt")
	require.Nil(t, os.WriteFile(filename, []byte{0x25, 0x86}, 0600))
	require.True(t, FileExists(filename))
	require.False(t, FileExists(filename+".doesnotexist"))
}

func TestInStringList(t *testing.T) {
	s := []string{"one", "two"}
	require.True(t, Contains(s, "two"))
	require.False(t, Contains(s, "three"))
}

func TestInStringListAll(t *testing.T) {
	s := []string{"one", "two", "three", "four"}
	require.True(t, ContainsAll(s, []string{"two", "four"}))
	require.False(t, ContainsAll(s, []string{"three", "five"}))
}

func TestContains(t *testing.T) {
	s := []int{1, 2}
	require.True(t, Contains(s, 2))
	require.False(t, Contains(s, 3))
}

func TestContainsAll(t *testing.T) {
	require.True(t, ContainsAll([]int{1, 2, 3}, []int{2, 3}))
	require.False(t, ContainsAll([]int{1, 1}, []int{1, 2}))
}

func TestContainsIP(t *testing.T) {
	require.True(t, ContainsIP([]netip.Prefix{netip.MustParsePrefix("fd00::/8"), netip.MustParsePrefix("1.1.0.0/16")}, netip.MustParseAddr("1.1.1.1")))
	require.True(t, ContainsIP([]netip.Prefix{netip.MustParsePrefix("fd00::/8"), netip.MustParsePrefix("1.1.0.0/16")}, netip.MustParseAddr("fd12:1234:5678::9876")))
	require.False(t, ContainsIP([]netip.Prefix{netip.MustParsePrefix("fd00::/8"), netip.MustParsePrefix("1.1.0.0/16")}, netip.MustParseAddr("1.2.0.1")))
	require.False(t, ContainsIP([]netip.Prefix{netip.MustParsePrefix("fd00::/8"), netip.MustParsePrefix("1.1.0.0/16")}, netip.MustParseAddr("fc00::1")))
}

func TestSplitNoEmpty(t *testing.T) {
	require.Equal(t, []string{}, SplitNoEmpty("", ","))
	require.Equal(t, []string{}, SplitNoEmpty(",,,", ","))
	require.Equal(t, []string{"tag1", "tag2"}, SplitNoEmpty("tag1,tag2", ","))
	require.Equal(t, []string{"tag1", "tag2"}, SplitNoEmpty("tag1,tag2,", ","))
}

func TestParsePriority(t *testing.T) {
	priorities := []string{"", "1", "2", "3", "4", "5", "min", "LOW", "   default ", "HIgh", "max", "urgent"}
	expected := []int{0, 1, 2, 3, 4, 5, 1, 2, 3, 4, 5, 5}
	for i, priority := range priorities {
		actual, err := ParsePriority(priority)
		require.Nil(t, err)
		require.Equal(t, expected[i], actual)
	}
}

func TestParsePriority_Invalid(t *testing.T) {
	priorities := []string{"-1", "6", "aa", "-", "o=1"}
	for _, priority := range priorities {
		_, err := ParsePriority(priority)
		require.Equal(t, errInvalidPriority, err)
	}
}

func TestPriorityString(t *testing.T) {
	priorities := []int{0, 1, 2, 3, 4, 5}
	expected := []string{"default", "min", "low", "default", "high", "max"}
	for i, priority := range priorities {
		actual, err := PriorityString(priority)
		require.Nil(t, err)
		require.Equal(t, expected[i], actual)
	}
}

func TestPriorityString_Invalid(t *testing.T) {
	_, err := PriorityString(99)
	require.Equal(t, err, errInvalidPriority)
}

func TestShortTopicURL(t *testing.T) {
	require.Equal(t, "ntfy.sh/mytopic", ShortTopicURL("https://ntfy.sh/mytopic"))
	require.Equal(t, "ntfy.sh/mytopic", ShortTopicURL("http://ntfy.sh/mytopic"))
	require.Equal(t, "lalala", ShortTopicURL("lalala"))
}

func TestParseSize_10GSuccess(t *testing.T) {
	s, err := ParseSize("10G")
	require.Nil(t, err)
	require.Equal(t, int64(10*1024*1024*1024), s)
}

func TestParseSize_10MUpperCaseSuccess(t *testing.T) {
	s, err := ParseSize("10M")
	require.Nil(t, err)
	require.Equal(t, int64(10*1024*1024), s)
}

func TestParseSize_10kLowerCaseSuccess(t *testing.T) {
	s, err := ParseSize("10k")
	require.Nil(t, err)
	require.Equal(t, int64(10*1024), s)
}

func TestParseSize_FailureInvalid(t *testing.T) {
	_, err := ParseSize("not a size")
	require.Nil(t, err)
}

func TestFormatSize(t *testing.T) {
	values := []struct {
		size     int64
		expected string
	}{
		{10, "10"},
		{10 * 1024, "10K"},
		{10 * 1024 * 1024, "10M"},
		{10 * 1024 * 1024 * 1024, "10G"},
	}
	for _, value := range values {
		require.Equal(t, value.expected, FormatSize(value.size))
		s, err := ParseSize(FormatSize(value.size))
		require.Nil(t, err)
		require.Equalf(t, value.size, s, "size does not match: %d != %d", value.size, s)
	}
}

func TestFormatSize_Rounded(t *testing.T) {
	require.Equal(t, "10K", FormatSize(10*1024+999))
}

func TestSplitKV(t *testing.T) {
	key, value := SplitKV(" key = value ", "=")
	require.Equal(t, "key", key)
	require.Equal(t, "value", value)

	key, value = SplitKV(" value ", "=")
	require.Equal(t, "", key)
	require.Equal(t, "value", value)

	key, value = SplitKV("mykey=value=with=separator ", "=")
	require.Equal(t, "mykey", key)
	require.Equal(t, "value=with=separator", value)
}

func TestLastString(t *testing.T) {
	require.Equal(t, "last", LastString([]string{"first", "second", "last"}, "default"))
	require.Equal(t, "default", LastString([]string{}, "default"))
}

func TestQuoteCommand(t *testing.T) {
	require.Equal(t, `ls -al "Document Folder"`, QuoteCommand([]string{"ls", "-al", "Document Folder"}))
	require.Equal(t, `rsync -av /home/phil/ root@example.com:/home/phil/`, QuoteCommand([]string{"rsync", "-av", "/home/phil/", "root@example.com:/home/phil/"}))
	require.Equal(t, `/home/sweet/home "Äöü this is a test" "\a\b"`, QuoteCommand([]string{"/home/sweet/home", "Äöü this is a test", "\\a\\b"}))
}

func TestBasicAuth(t *testing.T) {
	require.Equal(t, "Basic cGhpbDpwaGls", BasicAuth("phil", "phil"))
}

func TestBearerAuth(t *testing.T) {
	require.Equal(t, "Bearer sometoken", BearerAuth("sometoken"))
}

type testJSON struct {
	Name      string `json:"name"`
	Something int    `json:"something"`
}

func TestReadJSON_Success(t *testing.T) {
	v, err := UnmarshalJSON[testJSON](io.NopCloser(strings.NewReader(`{"name":"some name","something":99}`)))
	require.Nil(t, err)
	require.Equal(t, "some name", v.Name)
	require.Equal(t, 99, v.Something)
}

func TestReadJSON_Failure(t *testing.T) {
	_, err := UnmarshalJSON[testJSON](io.NopCloser(strings.NewReader(`{"na`)))
	require.Equal(t, ErrUnmarshalJSON, err)
}

func TestReadJSONWithLimit_Success(t *testing.T) {
	v, err := UnmarshalJSONWithLimit[testJSON](io.NopCloser(strings.NewReader(`{"name":"some name","something":99}`)), 100, false)
	require.Nil(t, err)
	require.Equal(t, "some name", v.Name)
	require.Equal(t, 99, v.Something)
}

func TestReadJSONWithLimit_FailureTooLong(t *testing.T) {
	_, err := UnmarshalJSONWithLimit[testJSON](io.NopCloser(strings.NewReader(`{"name":"some name","something":99}`)), 10, false)
	require.Equal(t, ErrTooLargeJSON, err)
}

func TestReadJSONWithLimit_AllowEmpty(t *testing.T) {
	v, err := UnmarshalJSONWithLimit[testJSON](io.NopCloser(strings.NewReader(` `)), 10, true)
	require.Nil(t, err)
	require.Equal(t, "", v.Name)
	require.Equal(t, 0, v.Something)
}

func TestReadJSONWithLimit_NoAllowEmpty(t *testing.T) {
	_, err := UnmarshalJSONWithLimit[testJSON](io.NopCloser(strings.NewReader(` `)), 10, false)
	require.Equal(t, ErrUnmarshalJSON, err)
}

func TestRetry_Succeeds(t *testing.T) {
	start := time.Now()
	delays, i := []time.Duration{10 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond, time.Second}, 0
	fn := func() (*int, error) {
		i++
		if i < len(delays) {
			return nil, errors.New("error")
		}
		return Int(99), nil
	}
	result, err := Retry[int](fn, delays...)
	require.Nil(t, err)
	require.Equal(t, 99, *result)
	require.True(t, time.Since(start).Milliseconds() > 150)
}

func TestRetry_Fails(t *testing.T) {
	fn := func() (*int, error) {
		return nil, errors.New("fails")
	}
	_, err := Retry[int](fn, 10*time.Millisecond)
	require.Error(t, err)
}

func TestMinMax(t *testing.T) {
	require.Equal(t, 10, MinMax(9, 10, 99))
	require.Equal(t, 99, MinMax(100, 10, 99))
	require.Equal(t, 50, MinMax(50, 10, 99))
}

func TestMax(t *testing.T) {
	require.Equal(t, 9, Max(1, 9))
	require.Equal(t, 9, Max(9, 1))
	require.Equal(t, rate.Every(time.Minute), Max(rate.Every(time.Hour), rate.Every(time.Minute)))
}

func TestPointerFunctions(t *testing.T) {
	i, s, ti := Int(99), String("abc"), Time(time.Unix(99, 0))
	require.Equal(t, 99, *i)
	require.Equal(t, "abc", *s)
	require.Equal(t, time.Unix(99, 0), *ti)
}

func TestMaybeMarshalJSON(t *testing.T) {
	require.Equal(t, `"aa"`, MaybeMarshalJSON("aa"))
	require.Equal(t, `[
  "aa",
  "bb"
]`, MaybeMarshalJSON([]string{"aa", "bb"}))
	require.Equal(t, "<cannot serialize>", MaybeMarshalJSON(func() {}))
	require.Equal(t, `"`+strings.Repeat("x", 4999), MaybeMarshalJSON(strings.Repeat("x", 6000)))

}
