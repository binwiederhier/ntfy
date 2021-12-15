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
