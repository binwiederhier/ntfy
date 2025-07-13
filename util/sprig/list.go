package sprig

import (
	"fmt"
	"math"
	"reflect"
	"sort"
)

// Reflection is used in these functions so that slices and arrays of strings,
// ints, and other types not implementing []any can be worked with.
// For example, this is useful if you need to work on the output of regexs.

func list(v ...any) []any {
	return v
}

func push(list any, v any) []any {
	l, err := mustPush(list, v)
	if err != nil {
		panic(err)
	}

	return l
}

func mustPush(list any, v any) ([]any, error) {
	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)

		l := l2.Len()
		nl := make([]any, l)
		for i := 0; i < l; i++ {
			nl[i] = l2.Index(i).Interface()
		}

		return append(nl, v), nil

	default:
		return nil, fmt.Errorf("cannot push on type %s", tp)
	}
}

func prepend(list any, v any) []any {
	l, err := mustPrepend(list, v)
	if err != nil {
		panic(err)
	}

	return l
}

func mustPrepend(list any, v any) ([]any, error) {
	//return append([]any{v}, list...)

	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)

		l := l2.Len()
		nl := make([]any, l)
		for i := 0; i < l; i++ {
			nl[i] = l2.Index(i).Interface()
		}

		return append([]any{v}, nl...), nil

	default:
		return nil, fmt.Errorf("cannot prepend on type %s", tp)
	}
}

func chunk(size int, list any) [][]any {
	l, err := mustChunk(size, list)
	if err != nil {
		panic(err)
	}

	return l
}

func mustChunk(size int, list any) ([][]any, error) {
	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)

		l := l2.Len()

		cs := int(math.Floor(float64(l-1)/float64(size)) + 1)
		nl := make([][]any, cs)

		for i := 0; i < cs; i++ {
			clen := size
			if i == cs-1 {
				clen = int(math.Floor(math.Mod(float64(l), float64(size))))
				if clen == 0 {
					clen = size
				}
			}

			nl[i] = make([]any, clen)

			for j := 0; j < clen; j++ {
				ix := i*size + j
				nl[i][j] = l2.Index(ix).Interface()
			}
		}

		return nl, nil

	default:
		return nil, fmt.Errorf("cannot chunk type %s", tp)
	}
}

func last(list any) any {
	l, err := mustLast(list)
	if err != nil {
		panic(err)
	}

	return l
}

func mustLast(list any) (any, error) {
	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)

		l := l2.Len()
		if l == 0 {
			return nil, nil
		}

		return l2.Index(l - 1).Interface(), nil
	default:
		return nil, fmt.Errorf("cannot find last on type %s", tp)
	}
}

func first(list any) any {
	l, err := mustFirst(list)
	if err != nil {
		panic(err)
	}

	return l
}

func mustFirst(list any) (any, error) {
	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)

		l := l2.Len()
		if l == 0 {
			return nil, nil
		}

		return l2.Index(0).Interface(), nil
	default:
		return nil, fmt.Errorf("cannot find first on type %s", tp)
	}
}

func rest(list any) []any {
	l, err := mustRest(list)
	if err != nil {
		panic(err)
	}

	return l
}

func mustRest(list any) ([]any, error) {
	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)

		l := l2.Len()
		if l == 0 {
			return nil, nil
		}

		nl := make([]any, l-1)
		for i := 1; i < l; i++ {
			nl[i-1] = l2.Index(i).Interface()
		}

		return nl, nil
	default:
		return nil, fmt.Errorf("cannot find rest on type %s", tp)
	}
}

func initial(list any) []any {
	l, err := mustInitial(list)
	if err != nil {
		panic(err)
	}

	return l
}

func mustInitial(list any) ([]any, error) {
	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)

		l := l2.Len()
		if l == 0 {
			return nil, nil
		}

		nl := make([]any, l-1)
		for i := 0; i < l-1; i++ {
			nl[i] = l2.Index(i).Interface()
		}

		return nl, nil
	default:
		return nil, fmt.Errorf("cannot find initial on type %s", tp)
	}
}

func sortAlpha(list any) []string {
	k := reflect.Indirect(reflect.ValueOf(list)).Kind()
	switch k {
	case reflect.Slice, reflect.Array:
		a := strslice(list)
		s := sort.StringSlice(a)
		s.Sort()
		return s
	}
	return []string{strval(list)}
}

func reverse(v any) []any {
	l, err := mustReverse(v)
	if err != nil {
		panic(err)
	}

	return l
}

func mustReverse(v any) ([]any, error) {
	tp := reflect.TypeOf(v).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(v)

		l := l2.Len()
		// We do not sort in place because the incoming array should not be altered.
		nl := make([]any, l)
		for i := 0; i < l; i++ {
			nl[l-i-1] = l2.Index(i).Interface()
		}

		return nl, nil
	default:
		return nil, fmt.Errorf("cannot find reverse on type %s", tp)
	}
}

func compact(list any) []any {
	l, err := mustCompact(list)
	if err != nil {
		panic(err)
	}

	return l
}

func mustCompact(list any) ([]any, error) {
	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)

		l := l2.Len()
		nl := []any{}
		var item any
		for i := 0; i < l; i++ {
			item = l2.Index(i).Interface()
			if !empty(item) {
				nl = append(nl, item)
			}
		}

		return nl, nil
	default:
		return nil, fmt.Errorf("cannot compact on type %s", tp)
	}
}

func uniq(list any) []any {
	l, err := mustUniq(list)
	if err != nil {
		panic(err)
	}

	return l
}

func mustUniq(list any) ([]any, error) {
	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)

		l := l2.Len()
		dest := []any{}
		var item any
		for i := 0; i < l; i++ {
			item = l2.Index(i).Interface()
			if !inList(dest, item) {
				dest = append(dest, item)
			}
		}

		return dest, nil
	default:
		return nil, fmt.Errorf("cannot find uniq on type %s", tp)
	}
}

func inList(haystack []any, needle any) bool {
	for _, h := range haystack {
		if reflect.DeepEqual(needle, h) {
			return true
		}
	}
	return false
}

func without(list any, omit ...any) []any {
	l, err := mustWithout(list, omit...)
	if err != nil {
		panic(err)
	}

	return l
}

func mustWithout(list any, omit ...any) ([]any, error) {
	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)

		l := l2.Len()
		res := []any{}
		var item any
		for i := 0; i < l; i++ {
			item = l2.Index(i).Interface()
			if !inList(omit, item) {
				res = append(res, item)
			}
		}

		return res, nil
	default:
		return nil, fmt.Errorf("cannot find without on type %s", tp)
	}
}

func has(needle any, haystack any) bool {
	l, err := mustHas(needle, haystack)
	if err != nil {
		panic(err)
	}

	return l
}

func mustHas(needle any, haystack any) (bool, error) {
	if haystack == nil {
		return false, nil
	}
	tp := reflect.TypeOf(haystack).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(haystack)
		var item any
		l := l2.Len()
		for i := 0; i < l; i++ {
			item = l2.Index(i).Interface()
			if reflect.DeepEqual(needle, item) {
				return true, nil
			}
		}

		return false, nil
	default:
		return false, fmt.Errorf("cannot find has on type %s", tp)
	}
}

// $list := [1, 2, 3, 4, 5]
// slice $list     -> list[0:5] = list[:]
// slice $list 0 3 -> list[0:3] = list[:3]
// slice $list 3 5 -> list[3:5]
// slice $list 3   -> list[3:5] = list[3:]
func slice(list any, indices ...any) any {
	l, err := mustSlice(list, indices...)
	if err != nil {
		panic(err)
	}

	return l
}

func mustSlice(list any, indices ...any) (any, error) {
	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)

		l := l2.Len()
		if l == 0 {
			return nil, nil
		}

		var start, end int
		if len(indices) > 0 {
			start = toInt(indices[0])
		}
		if len(indices) < 2 {
			end = l
		} else {
			end = toInt(indices[1])
		}

		return l2.Slice(start, end).Interface(), nil
	default:
		return nil, fmt.Errorf("list should be type of slice or array but %s", tp)
	}
}

func concat(lists ...any) any {
	var res []any
	for _, list := range lists {
		tp := reflect.TypeOf(list).Kind()
		switch tp {
		case reflect.Slice, reflect.Array:
			l2 := reflect.ValueOf(list)
			for i := 0; i < l2.Len(); i++ {
				res = append(res, l2.Index(i).Interface())
			}
		default:
			panic(fmt.Sprintf("cannot concat type %s as list", tp))
		}
	}
	return res
}
