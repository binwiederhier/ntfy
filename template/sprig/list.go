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

// list creates a new list (slice) containing the provided arguments.
// It accepts any number of arguments of any type and returns them as a slice.
func list(v ...any) []any {
	return v
}

// push appends an element to the end of a list (slice or array).
// It takes a list and a value, and returns a new list with the value appended.
// This function will panic if the first argument is not a slice or array.
func push(list any, v any) []any {
	l, err := mustPush(list, v)
	if err != nil {
		panic(err)
	}
	return l
}

// mustPush is the implementation of push that returns an error instead of panicking.
// It converts the input list to a slice of any type, then appends the value.
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

// prepend adds an element to the beginning of a list (slice or array).
// It takes a list and a value, and returns a new list with the value at the start.
// This function will panic if the first argument is not a slice or array.
func prepend(list any, v any) []any {
	l, err := mustPrepend(list, v)
	if err != nil {
		panic(err)
	}
	return l
}

// mustPrepend is the implementation of prepend that returns an error instead of panicking.
// It converts the input list to a slice of any type, then prepends the value.
func mustPrepend(list any, v any) ([]any, error) {
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

// chunk divides a list into sub-lists of the specified size.
// It takes a size and a list, and returns a list of lists, each containing
// up to 'size' elements from the original list.
// This function will panic if the second argument is not a slice or array.
func chunk(size int, list any) [][]any {
	l, err := mustChunk(size, list)
	if err != nil {
		panic(err)
	}
	return l
}

// mustChunk is the implementation of chunk that returns an error instead of panicking.
// It divides the input list into chunks of the specified size.
func mustChunk(size int, list any) ([][]any, error) {
	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)
		l := l2.Len()
		numChunks := int(math.Floor(float64(l-1)/float64(size)) + 1)
		if numChunks > sliceSizeLimit {
			return nil, fmt.Errorf("number of chunks %d exceeds maximum limit of %d", numChunks, sliceSizeLimit)
		}
		result := make([][]any, numChunks)
		for i := 0; i < numChunks; i++ {
			clen := size
			// Handle the last chunk which might be smaller
			if i == numChunks-1 {
				clen = int(math.Floor(math.Mod(float64(l), float64(size))))
				if clen == 0 {
					clen = size
				}
			}
			result[i] = make([]any, clen)
			for j := 0; j < clen; j++ {
				ix := i*size + j
				result[i][j] = l2.Index(ix).Interface()
			}
		}
		return result, nil

	default:
		return nil, fmt.Errorf("cannot chunk type %s", tp)
	}
}

// last returns the last element of a list (slice or array).
// If the list is empty, it returns nil.
// This function will panic if the argument is not a slice or array.
func last(list any) any {
	l, err := mustLast(list)
	if err != nil {
		panic(err)
	}

	return l
}

// mustLast is the implementation of last that returns an error instead of panicking.
// It returns the last element of the list or nil if the list is empty.
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

// first returns the first element of a list (slice or array).
// If the list is empty, it returns nil.
// This function will panic if the argument is not a slice or array.
func first(list any) any {
	l, err := mustFirst(list)
	if err != nil {
		panic(err)
	}

	return l
}

// mustFirst is the implementation of first that returns an error instead of panicking.
// It returns the first element of the list or nil if the list is empty.
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

// rest returns all elements of a list except the first one.
// If the list is empty, it returns nil.
// This function will panic if the argument is not a slice or array.
func rest(list any) []any {
	l, err := mustRest(list)
	if err != nil {
		panic(err)
	}

	return l
}

// mustRest is the implementation of rest that returns an error instead of panicking.
// It returns all elements of the list except the first one, or nil if the list is empty.
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

// initial returns all elements of a list except the last one.
// If the list is empty, it returns nil.
// This function will panic if the argument is not a slice or array.
func initial(list any) []any {
	l, err := mustInitial(list)
	if err != nil {
		panic(err)
	}

	return l
}

// mustInitial is the implementation of initial that returns an error instead of panicking.
// It returns all elements of the list except the last one, or nil if the list is empty.
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

// sortAlpha sorts a list of strings alphabetically.
// If the input is not a slice or array, it returns a single-element slice
// containing the string representation of the input.
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

// reverse returns a new list with the elements in reverse order.
// This function will panic if the argument is not a slice or array.
func reverse(v any) []any {
	l, err := mustReverse(v)
	if err != nil {
		panic(err)
	}

	return l
}

// mustReverse is the implementation of reverse that returns an error instead of panicking.
// It returns a new list with the elements in reverse order.
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

// compact returns a new list with all "empty" elements removed.
// An element is considered empty if it's nil, zero, an empty string, or an empty collection.
// This function will panic if the argument is not a slice or array.
func compact(list any) []any {
	l, err := mustCompact(list)
	if err != nil {
		panic(err)
	}
	return l
}

// mustCompact is the implementation of compact that returns an error instead of panicking.
// It returns a new list with all "empty" elements removed.
func mustCompact(list any) ([]any, error) {
	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)
		l := l2.Len()
		var nl []any
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

// uniq returns a new list with duplicate elements removed.
// The first occurrence of each element is kept.
// This function will panic if the argument is not a slice or array.
func uniq(list any) []any {
	l, err := mustUniq(list)
	if err != nil {
		panic(err)
	}
	return l
}

// mustUniq is the implementation of uniq that returns an error instead of panicking.
// It returns a new list with duplicate elements removed.
func mustUniq(list any) ([]any, error) {
	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)
		l := l2.Len()
		if l > loopExecutionLimit {
			return nil, fmt.Errorf("uniq list length %d exceeds limit of %d", l, loopExecutionLimit)
		}
		var dest []any
		var item any
		comparisons := 0
		for i := 0; i < l; i++ {
			item = l2.Index(i).Interface()
			comparisons += len(dest)
			if comparisons > comparisonLimit {
				return nil, fmt.Errorf("uniq number of comparisons exceeds limit of %d", comparisonLimit)
			}
			if !inList(dest, item) {
				dest = append(dest, item)
			}
		}
		return dest, nil
	default:
		return nil, fmt.Errorf("cannot find uniq on type %s", tp)
	}
}

// inList checks if a value is present in a list.
// It uses deep equality comparison to check for matches.
// Returns true if the value is found, false otherwise.
func inList(haystack []any, needle any) bool {
	for _, h := range haystack {
		if reflect.DeepEqual(needle, h) {
			return true
		}
	}
	return false
}

// without returns a new list with all occurrences of the specified values removed.
// This function will panic if the first argument is not a slice or array.
func without(list any, omit ...any) []any {
	l, err := mustWithout(list, omit...)
	if err != nil {
		panic(err)
	}
	return l
}

// mustWithout is the implementation of without that returns an error instead of panicking.
// It returns a new list with all occurrences of the specified values removed.
func mustWithout(list any, omit ...any) ([]any, error) {
	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)
		l := l2.Len()
		if l > loopExecutionLimit {
			return nil, fmt.Errorf("without list length %d exceeds limit of %d", l, loopExecutionLimit)
		} else if l*len(omit) > comparisonLimit {
			return nil, fmt.Errorf("without number of comparisons exceeds limit of %d", comparisonLimit)
		}
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

// has checks if a value is present in a list.
// Returns true if the value is found, false otherwise.
// This function will panic if the second argument is not a slice or array.
func has(needle any, haystack any) bool {
	l, err := mustHas(needle, haystack)
	if err != nil {
		panic(err)
	}
	return l
}

// mustHas is the implementation of has that returns an error instead of panicking.
// It checks if a value is present in a list.
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
		if l > loopExecutionLimit {
			return false, fmt.Errorf("has list length %d exceeds limit of %d", l, loopExecutionLimit)
		}
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

// slice extracts a portion of a list based on the provided indices.
// Usage examples:
// $list := [1, 2, 3, 4, 5]
// slice $list     -> list[0:5] = list[:]
// slice $list 0 3 -> list[0:3] = list[:3]
// slice $list 3 5 -> list[3:5]
// slice $list 3   -> list[3:5] = list[3:]
//
// This function will panic if the first argument is not a slice or array.
func slice(list any, indices ...any) any {
	l, err := mustSlice(list, indices...)
	if err != nil {
		panic(err)
	}
	return l
}

// mustSlice is the implementation of slice that returns an error instead of panicking.
// It extracts a portion of a list based on the provided indices.
func mustSlice(list any, indices ...any) (any, error) {
	tp := reflect.TypeOf(list).Kind()
	switch tp {
	case reflect.Slice, reflect.Array:
		l2 := reflect.ValueOf(list)
		l := l2.Len()
		if l == 0 {
			return nil, nil
		}
		// Determine start and end indices
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

// concat combines multiple lists into a single list.
// It takes any number of lists and returns a new list containing all elements.
// This function will panic if any argument is not a slice or array.
// It also panics if the combined length exceeds sliceSizeLimit, since passing the same list many
// times would otherwise multiply it in a single call.
func concat(lists ...any) any {
	total := 0
	for _, list := range lists {
		switch reflect.TypeOf(list).Kind() {
		case reflect.Slice, reflect.Array:
			total += reflect.ValueOf(list).Len()
		}
	}
	if total > sliceSizeLimit {
		panic(fmt.Sprintf("concat result size %d exceeds limit of %d", total, sliceSizeLimit))
	}
	res := make([]any, 0, total)
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
