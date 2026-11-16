package template

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckPrintable_Success(t *testing.T) {
	shared := []any{"a", "b"}
	value := map[string]any{
		"list":  []any{1, 2.5, true, nil, "x"},
		"map":   map[string]any{"k": "v"},
		"left":  shared,
		"right": shared, // Shared, but not cyclic
	}
	require.Nil(t, CheckPrintable(value, "str", 1, nil))
}

func TestCheckPrintable_Cycle(t *testing.T) {
	a := map[string]any{}
	b := []any{a}
	a["b"] = b // a -> b -> a
	require.ErrorIs(t, CheckPrintable(a), errValueCyclic)
	require.ErrorIs(t, CheckPrintable("ok", b), errValueCyclic)
}

func TestCheckPrintable_SharedReferencesTooLarge(t *testing.T) {
	// Each level references the previous one twice, so the expanded size doubles per level:
	// 2^6 = 64 leaves fit a budget of 200, 2^8 = 256 leaves do not
	value := any("x")
	for range 6 {
		value = []any{value, value}
	}
	require.Nil(t, checkPrintable(100, 200, value))
	value = []any{[]any{value, value}, []any{value, value}}
	require.ErrorIs(t, checkPrintable(100, 200, value), errValueTooLarge)
}

func TestCheckPrintable_StringBytesCount(t *testing.T) {
	require.Nil(t, checkPrintable(100, 200, strings.Repeat("x", 150)))
	require.ErrorIs(t, checkPrintable(100, 200, strings.Repeat("x", 150), strings.Repeat("x", 150)), errValueTooLarge)
}

func TestCheckPrintable_NumbersCountPrintedSize(t *testing.T) {
	// A number prints up to 24 bytes, so it is charged like a string of that length
	require.Nil(t, checkPrintable(100, 200, []any{1, 2, 3, 4, 5}))
	require.ErrorIs(t, checkPrintable(100, 200, []any{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}), errValueTooLarge)
}

func TestCheckPrintable_TooDeep(t *testing.T) {
	value := any("x")
	for range 10 {
		value = []any{value}
	}
	require.Nil(t, checkPrintable(20, 1000, value))
	require.ErrorIs(t, checkPrintable(5, 1000, value), errValueTooDeep)
}

func TestCheckPrintable_Limits(t *testing.T) {
	// A template renders against a JSON body of at most 128 KB, so the limits leave room for any
	// accepted body, but not much more
	require.Nil(t, CheckPrintable(strings.Repeat("x", 200*1024)))
	require.ErrorIs(t, CheckPrintable(strings.Repeat("x", 300*1024)), errValueTooLarge)
	nested := func(depth int) any {
		value := any("x")
		for range depth {
			value = []any{value}
		}
		return value
	}
	require.Nil(t, CheckPrintable(nested(90)))
	require.ErrorIs(t, CheckPrintable(nested(150)), errValueTooDeep)
}
