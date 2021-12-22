package util

import (
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
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

func TestExpandHome_WithTilde(t *testing.T) {
	require.Equal(t, os.Getenv("HOME")+"/this/is/a/path", ExpandHome("~/this/is/a/path"))
}

func TestExpandHome_NoTilde(t *testing.T) {
	require.Equal(t, "/this/is/an/absolute/path", ExpandHome("/this/is/an/absolute/path"))
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
