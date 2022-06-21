package util

import (
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"
)

func TestDurationToHuman_SevenDays(t *testing.T) {
	d := 7 * 24 * time.Hour
	require.Equal(t, "7d", DurationToHuman(d))
}

func TestDurationToHuman_MoreThanOneDay(t *testing.T) {
	d := 49 * time.Hour
	require.Equal(t, "2d1h", DurationToHuman(d))
}

func TestDurationToHuman_LessThanOneDay(t *testing.T) {
	d := 17*time.Hour + 15*time.Minute
	require.Equal(t, "17h15m", DurationToHuman(d))
}

func TestDurationToHuman_TenOfThings(t *testing.T) {
	d := 10*time.Hour + 10*time.Minute + 10*time.Second
	require.Equal(t, "10h10m10s", DurationToHuman(d))
}

func TestDurationToHuman_Zero(t *testing.T) {
	require.Equal(t, "0", DurationToHuman(0))
}

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
	require.Nil(t, ioutil.WriteFile(filename, []byte{0x25, 0x86}, 0600))
	require.True(t, FileExists(filename))
	require.False(t, FileExists(filename+".doesnotexist"))
}

func TestInStringList(t *testing.T) {
	s := []string{"one", "two"}
	require.True(t, InStringList(s, "two"))
	require.False(t, InStringList(s, "three"))
}

func TestInStringListAll(t *testing.T) {
	s := []string{"one", "two", "three", "four"}
	require.True(t, InStringListAll(s, []string{"two", "four"}))
	require.False(t, InStringListAll(s, []string{"three", "five"}))
}

func TestInIntList(t *testing.T) {
	s := []int{1, 2}
	require.True(t, InIntList(s, 2))
	require.False(t, InIntList(s, 3))
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
	priorities := []string{"-1", "6", "aa", "-"}
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
