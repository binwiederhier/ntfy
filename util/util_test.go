package util

import (
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestParsePriority_HTTPSpecPriority(t *testing.T) {
	priorities := []string{"u=1", "u=3", "u=7, i"} // see https://datatracker.ietf.org/doc/html/draft-ietf-httpbis-priority
	for _, priority := range priorities {
		actual, err := ParsePriority(priority)
		require.Nil(t, err)
		require.Equal(t, 3, actual) // Always expect 3!
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
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, int64(10*1024*1024*1024), s)
}

func TestParseSize_10MUpperCaseSuccess(t *testing.T) {
	s, err := ParseSize("10M")
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, int64(10*1024*1024), s)
}

func TestParseSize_10kLowerCaseSuccess(t *testing.T) {
	s, err := ParseSize("10k")
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, int64(10*1024), s)
}

func TestParseSize_FailureInvalid(t *testing.T) {
	_, err := ParseSize("not a size")
	if err == nil {
		t.Fatalf("expected error, but got none")
	}
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
	v, err := UnmarshalJSONWithLimit[testJSON](io.NopCloser(strings.NewReader(`{"name":"some name","something":99}`)), 100)
	require.Nil(t, err)
	require.Equal(t, "some name", v.Name)
	require.Equal(t, 99, v.Something)
}

func TestReadJSONWithLimit_FailureTooLong(t *testing.T) {
	_, err := UnmarshalJSONWithLimit[testJSON](io.NopCloser(strings.NewReader(`{"name":"some name","something":99}`)), 10)
	require.Equal(t, ErrTooLargeJSON, err)
}
