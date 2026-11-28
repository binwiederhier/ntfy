// Package template contains ntfy's additions to message templating that are not tied to a single
// function library. The vendored text/template copy lives in template/gotext, and the function
// library in template/sprig.
package template

import (
	"errors"
	"reflect"
)

// Limits for CheckPrintable. A template renders against a JSON body of at most 128 KB, and real
// webhook payloads nest far fewer than 100 levels, so these leave room for any accepted body.
const (
	printableMaxDepth   = 100        // Maximum nesting depth of a printable value
	printableMaxSize    = 256 * 1024 // Maximum approximate expanded size of printable values (one per value, plus printed lengths)
	printableScalarSize = 24         // Longest %v of a number, e.g. "-1.2345678901234567e+308"
)

// Errors returned by CheckPrintable
var (
	errValueCyclic   = errors.New("value references itself")
	errValueTooDeep  = errors.New("value is nested too deeply")
	errValueTooLarge = errors.New("value is too large")
)

// CheckPrintable returns an error if any of the values references itself, or if printing or
// JSON-encoding them would expand to more than printableMaxSize (see checkPrintable).
func CheckPrintable(values ...any) error {
	return checkPrintable(printableMaxDepth, printableMaxSize, values...)
}

// checkPrintable walks the values the way fmt and encoding/json would, without printing them. It
// counts one per visited value plus the printed length of every string and number, and visits a
// shared value once per reference, so a list holding the same value several times is charged its
// fully expanded size. Values on the current path are tracked to detect cycles, which would
// otherwise recurse forever.
func checkPrintable(maxDepth, maxSize int, values ...any) error {
	w := &printableWalker{
		maxDepth: maxDepth,
		budget:   maxSize,
		path:     make(map[printableRef]struct{}),
	}
	for _, v := range values {
		if err := w.walk(reflect.ValueOf(v), 0); err != nil {
			return err
		}
	}
	return nil
}

// printableRef identifies a map, slice or pointer on the current walk path
type printableRef struct {
	kind reflect.Kind
	ptr  uintptr
	len  int
}

type printableWalker struct {
	maxDepth int
	budget   int
	path     map[printableRef]struct{}
}

func (w *printableWalker) walk(v reflect.Value, depth int) error {
	for v.Kind() == reflect.Interface && !v.IsNil() {
		v = v.Elem() // Elements of []any and map[string]any are interfaces; charge only the value inside
	}
	if depth > w.maxDepth {
		return errValueTooDeep
	}
	w.budget--
	if w.budget < 0 {
		return errValueTooLarge
	}
	switch v.Kind() {
	case reflect.String:
		w.budget -= v.Len()
		if w.budget < 0 {
			return errValueTooLarge
		}
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		w.budget -= printableScalarSize // A list of numbers prints far more than one byte per element
		if w.budget < 0 {
			return errValueTooLarge
		}
	case reflect.Pointer, reflect.Map, reflect.Slice:
		if v.IsNil() {
			return nil
		}
		ref := printableRef{
			kind: v.Kind(),
			ptr:  v.Pointer(),
		}
		if v.Kind() == reflect.Slice {
			ref.len = v.Len() // A sub-slice shares the pointer, but not the length
		}
		if _, ok := w.path[ref]; ok {
			return errValueCyclic
		}
		w.path[ref] = struct{}{}
		defer delete(w.path, ref)
		return w.walkChildren(v, depth)
	case reflect.Array, reflect.Struct:
		return w.walkChildren(v, depth)
	}
	return nil
}

func (w *printableWalker) walkChildren(v reflect.Value, depth int) error {
	switch v.Kind() {
	case reflect.Pointer:
		return w.walk(v.Elem(), depth+1)
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			if err := w.walk(iter.Key(), depth+1); err != nil {
				return err
			}
			if err := w.walk(iter.Value(), depth+1); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := w.walk(v.Index(i), depth+1); err != nil {
				return err
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if err := w.walk(v.Field(i), depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
