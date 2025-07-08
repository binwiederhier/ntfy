package sprig

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
)

func TestUntil(t *testing.T) {
	tests := map[string]string{
		`{{range $i, $e := until 5}}{{$i}}{{$e}}{{end}}`:   "0011223344",
		`{{range $i, $e := until -5}}{{$i}}{{$e}} {{end}}`: "00 1-1 2-2 3-3 4-4 ",
	}
	for tpl, expect := range tests {
		if err := runt(tpl, expect); err != nil {
			t.Error(err)
		}
	}
}
func TestUntilStep(t *testing.T) {
	tests := map[string]string{
		`{{range $i, $e := untilStep 0 5 1}}{{$i}}{{$e}}{{end}}`:     "0011223344",
		`{{range $i, $e := untilStep 3 6 1}}{{$i}}{{$e}}{{end}}`:     "031425",
		`{{range $i, $e := untilStep 0 -10 -2}}{{$i}}{{$e}} {{end}}`: "00 1-2 2-4 3-6 4-8 ",
		`{{range $i, $e := untilStep 3 0 1}}{{$i}}{{$e}}{{end}}`:     "",
		`{{range $i, $e := untilStep 3 99 0}}{{$i}}{{$e}}{{end}}`:    "",
		`{{range $i, $e := untilStep 3 99 -1}}{{$i}}{{$e}}{{end}}`:   "",
		`{{range $i, $e := untilStep 3 0 0}}{{$i}}{{$e}}{{end}}`:     "",
	}
	for tpl, expect := range tests {
		if err := runt(tpl, expect); err != nil {
			t.Error(err)
		}
	}

}
func TestBiggest(t *testing.T) {
	tpl := `{{ biggest 1 2 3 345 5 6 7}}`
	if err := runt(tpl, `345`); err != nil {
		t.Error(err)
	}

	tpl = `{{ max 345}}`
	if err := runt(tpl, `345`); err != nil {
		t.Error(err)
	}
}
func TestMaxf(t *testing.T) {
	tpl := `{{ maxf 1 2 3 345.7 5 6 7}}`
	if err := runt(tpl, `345.7`); err != nil {
		t.Error(err)
	}

	tpl = `{{ max 345 }}`
	if err := runt(tpl, `345`); err != nil {
		t.Error(err)
	}
}
func TestMin(t *testing.T) {
	tpl := `{{ min 1 2 3 345 5 6 7}}`
	if err := runt(tpl, `1`); err != nil {
		t.Error(err)
	}

	tpl = `{{ min 345}}`
	if err := runt(tpl, `345`); err != nil {
		t.Error(err)
	}
}

func TestMinf(t *testing.T) {
	tpl := `{{ minf 1.4 2 3 345.6 5 6 7}}`
	if err := runt(tpl, `1.4`); err != nil {
		t.Error(err)
	}

	tpl = `{{ minf 345 }}`
	if err := runt(tpl, `345`); err != nil {
		t.Error(err)
	}
}

func TestToFloat64(t *testing.T) {
	target := float64(102)
	if target != toFloat64(int8(102)) {
		t.Errorf("Expected 102")
	}
	if target != toFloat64(int(102)) {
		t.Errorf("Expected 102")
	}
	if target != toFloat64(int32(102)) {
		t.Errorf("Expected 102")
	}
	if target != toFloat64(int16(102)) {
		t.Errorf("Expected 102")
	}
	if target != toFloat64(int64(102)) {
		t.Errorf("Expected 102")
	}
	if target != toFloat64("102") {
		t.Errorf("Expected 102")
	}
	if 0 != toFloat64("frankie") {
		t.Errorf("Expected 0")
	}
	if target != toFloat64(uint16(102)) {
		t.Errorf("Expected 102")
	}
	if target != toFloat64(uint64(102)) {
		t.Errorf("Expected 102")
	}
	if 102.1234 != toFloat64(float64(102.1234)) {
		t.Errorf("Expected 102.1234")
	}
	if 1 != toFloat64(true) {
		t.Errorf("Expected 102")
	}
}
func TestToInt64(t *testing.T) {
	target := int64(102)
	if target != toInt64(int8(102)) {
		t.Errorf("Expected 102")
	}
	if target != toInt64(int(102)) {
		t.Errorf("Expected 102")
	}
	if target != toInt64(int32(102)) {
		t.Errorf("Expected 102")
	}
	if target != toInt64(int16(102)) {
		t.Errorf("Expected 102")
	}
	if target != toInt64(int64(102)) {
		t.Errorf("Expected 102")
	}
	if target != toInt64("102") {
		t.Errorf("Expected 102")
	}
	if 0 != toInt64("frankie") {
		t.Errorf("Expected 0")
	}
	if target != toInt64(uint16(102)) {
		t.Errorf("Expected 102")
	}
	if target != toInt64(uint64(102)) {
		t.Errorf("Expected 102")
	}
	if target != toInt64(float64(102.1234)) {
		t.Errorf("Expected 102")
	}
	if 1 != toInt64(true) {
		t.Errorf("Expected 102")
	}
}

func TestToInt(t *testing.T) {
	target := int(102)
	if target != toInt(int8(102)) {
		t.Errorf("Expected 102")
	}
	if target != toInt(int(102)) {
		t.Errorf("Expected 102")
	}
	if target != toInt(int32(102)) {
		t.Errorf("Expected 102")
	}
	if target != toInt(int16(102)) {
		t.Errorf("Expected 102")
	}
	if target != toInt(int64(102)) {
		t.Errorf("Expected 102")
	}
	if target != toInt("102") {
		t.Errorf("Expected 102")
	}
	if 0 != toInt("frankie") {
		t.Errorf("Expected 0")
	}
	if target != toInt(uint16(102)) {
		t.Errorf("Expected 102")
	}
	if target != toInt(uint64(102)) {
		t.Errorf("Expected 102")
	}
	if target != toInt(float64(102.1234)) {
		t.Errorf("Expected 102")
	}
	if 1 != toInt(true) {
		t.Errorf("Expected 102")
	}
}

func TestToDecimal(t *testing.T) {
	tests := map[interface{}]int64{
		"777": 511,
		777:   511,
		770:   504,
		755:   493,
	}

	for input, expectedResult := range tests {
		result := toDecimal(input)
		if result != expectedResult {
			t.Errorf("Expected %v but got %v", expectedResult, result)
		}
	}
}

func TestAdd1(t *testing.T) {
	tpl := `{{ 3 | add1 }}`
	if err := runt(tpl, `4`); err != nil {
		t.Error(err)
	}
}

func TestAdd(t *testing.T) {
	tpl := `{{ 3 | add 1 2}}`
	if err := runt(tpl, `6`); err != nil {
		t.Error(err)
	}
}

func TestDiv(t *testing.T) {
	tpl := `{{ 4 | div 5 }}`
	if err := runt(tpl, `1`); err != nil {
		t.Error(err)
	}
}

func TestMul(t *testing.T) {
	tpl := `{{ 1 | mul "2" 3 "4"}}`
	if err := runt(tpl, `24`); err != nil {
		t.Error(err)
	}
}

func TestSub(t *testing.T) {
	tpl := `{{ 3 | sub 14 }}`
	if err := runt(tpl, `11`); err != nil {
		t.Error(err)
	}
}

func TestCeil(t *testing.T) {
	assert.Equal(t, 123.0, ceil(123))
	assert.Equal(t, 123.0, ceil("123"))
	assert.Equal(t, 124.0, ceil(123.01))
	assert.Equal(t, 124.0, ceil("123.01"))
}

func TestFloor(t *testing.T) {
	assert.Equal(t, 123.0, floor(123))
	assert.Equal(t, 123.0, floor("123"))
	assert.Equal(t, 123.0, floor(123.9999))
	assert.Equal(t, 123.0, floor("123.9999"))
}

func TestRound(t *testing.T) {
	assert.Equal(t, 123.556, round(123.5555, 3))
	assert.Equal(t, 123.556, round("123.55555", 3))
	assert.Equal(t, 124.0, round(123.500001, 0))
	assert.Equal(t, 123.0, round(123.49999999, 0))
	assert.Equal(t, 123.23, round(123.2329999, 2, .3))
	assert.Equal(t, 123.24, round(123.233, 2, .3))
}

func TestRandomInt(t *testing.T) {
	var tests = []struct {
		min int
		max int
	}{
		{10, 11},
		{10, 13},
		{0, 1},
		{5, 50},
	}
	for _, v := range tests {
		x, _ := runRaw(fmt.Sprintf(`{{ randInt %d %d }}`, v.min, v.max), nil)
		r, err := strconv.Atoi(x)
		assert.NoError(t, err)
		assert.True(t, func(min, max, r int) bool {
			return r >= v.min && r < v.max
		}(v.min, v.max, r))
	}
}

func TestSeq(t *testing.T) {
	tests := map[string]string{
		`{{seq 0 1 3}}`:   "0 1 2 3",
		`{{seq 0 3 10}}`:  "0 3 6 9",
		`{{seq 3 3 2}}`:   "",
		`{{seq 3 -3 2}}`:  "3",
		`{{seq}}`:         "",
		`{{seq 0 4}}`:     "0 1 2 3 4",
		`{{seq 5}}`:       "1 2 3 4 5",
		`{{seq -5}}`:      "1 0 -1 -2 -3 -4 -5",
		`{{seq 0}}`:       "1 0",
		`{{seq 0 1 2 3}}`: "",
		`{{seq 0 -4}}`:    "0 -1 -2 -3 -4",
	}
	for tpl, expect := range tests {
		if err := runt(tpl, expect); err != nil {
			t.Error(err)
		}
	}
}
