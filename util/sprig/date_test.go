package sprig

import (
	"testing"
	"time"
)

func TestHtmlDate(t *testing.T) {
	t.Skip()
	tpl := `{{ htmlDate 0}}`
	if err := runt(tpl, "1970-01-01"); err != nil {
		t.Error(err)
	}
}

func TestAgo(t *testing.T) {
	tpl := "{{ ago .Time }}"
	if err := runtv(tpl, "2m5s", map[string]interface{}{"Time": time.Now().Add(-125 * time.Second)}); err != nil {
		t.Error(err)
	}

	if err := runtv(tpl, "2h34m17s", map[string]interface{}{"Time": time.Now().Add(-(2*3600 + 34*60 + 17) * time.Second)}); err != nil {
		t.Error(err)
	}

	if err := runtv(tpl, "-5s", map[string]interface{}{"Time": time.Now().Add(5 * time.Second)}); err != nil {
		t.Error(err)
	}
}

func TestToDate(t *testing.T) {
	tpl := `{{toDate "2006-01-02" "2017-12-31" | date "02/01/2006"}}`
	if err := runt(tpl, "31/12/2017"); err != nil {
		t.Error(err)
	}
}

func TestUnixEpoch(t *testing.T) {
	tm, err := time.Parse("02 Jan 06 15:04:05 MST", "13 Jun 19 20:39:39 GMT")
	if err != nil {
		t.Error(err)
	}
	tpl := `{{unixEpoch .Time}}`

	if err = runtv(tpl, "1560458379", map[string]interface{}{"Time": tm}); err != nil {
		t.Error(err)
	}
}

func TestDateInZone(t *testing.T) {
	tm, err := time.Parse("02 Jan 06 15:04:05 MST", "13 Jun 19 20:39:39 GMT")
	if err != nil {
		t.Error(err)
	}
	tpl := `{{ date_in_zone "02 Jan 06 15:04 -0700" .Time "UTC" }}`

	// Test time.Time input
	if err = runtv(tpl, "13 Jun 19 20:39 +0000", map[string]interface{}{"Time": tm}); err != nil {
		t.Error(err)
	}

	// Test pointer to time.Time input
	if err = runtv(tpl, "13 Jun 19 20:39 +0000", map[string]interface{}{"Time": &tm}); err != nil {
		t.Error(err)
	}

	// Test no time input. This should be close enough to time.Now() we can test
	loc, _ := time.LoadLocation("UTC")
	if err = runtv(tpl, time.Now().In(loc).Format("02 Jan 06 15:04 -0700"), map[string]interface{}{"Time": ""}); err != nil {
		t.Error(err)
	}

	// Test unix timestamp as int64
	if err = runtv(tpl, "13 Jun 19 20:39 +0000", map[string]interface{}{"Time": int64(1560458379)}); err != nil {
		t.Error(err)
	}

	// Test unix timestamp as int32
	if err = runtv(tpl, "13 Jun 19 20:39 +0000", map[string]interface{}{"Time": int32(1560458379)}); err != nil {
		t.Error(err)
	}

	// Test unix timestamp as int
	if err = runtv(tpl, "13 Jun 19 20:39 +0000", map[string]interface{}{"Time": int(1560458379)}); err != nil {
		t.Error(err)
	}

	// Test case of invalid timezone
	tpl = `{{ date_in_zone "02 Jan 06 15:04 -0700" .Time "foobar" }}`
	if err = runtv(tpl, "13 Jun 19 20:39 +0000", map[string]interface{}{"Time": tm}); err != nil {
		t.Error(err)
	}
}

func TestDuration(t *testing.T) {
	tpl := "{{ duration .Secs }}"
	if err := runtv(tpl, "1m1s", map[string]interface{}{"Secs": "61"}); err != nil {
		t.Error(err)
	}
	if err := runtv(tpl, "1h0m0s", map[string]interface{}{"Secs": "3600"}); err != nil {
		t.Error(err)
	}
	// 1d2h3m4s but go is opinionated
	if err := runtv(tpl, "26h3m4s", map[string]interface{}{"Secs": "93784"}); err != nil {
		t.Error(err)
	}
}

func TestDurationRound(t *testing.T) {
	tpl := "{{ durationRound .Time }}"
	if err := runtv(tpl, "2h", map[string]interface{}{"Time": "2h5s"}); err != nil {
		t.Error(err)
	}
	if err := runtv(tpl, "1d", map[string]interface{}{"Time": "24h5s"}); err != nil {
		t.Error(err)
	}
	if err := runtv(tpl, "3mo", map[string]interface{}{"Time": "2400h5s"}); err != nil {
		t.Error(err)
	}
}
