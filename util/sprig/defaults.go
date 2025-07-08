package sprig

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"reflect"
	"strings"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// dfault checks whether `given` is set, and returns default if not set.
//
// This returns `d` if `given` appears not to be set, and `given` otherwise.
//
// For numeric types 0 is unset.
// For strings, maps, arrays, and slices, len() = 0 is considered unset.
// For bool, false is unset.
// Structs are never considered unset.
//
// For everything else, including pointers, a nil value is unset.
func dfault(d interface{}, given ...interface{}) interface{} {

	if empty(given) || empty(given[0]) {
		return d
	}
	return given[0]
}

// empty returns true if the given value has the zero value for its type.
func empty(given interface{}) bool {
	g := reflect.ValueOf(given)
	if !g.IsValid() {
		return true
	}

	// Basically adapted from text/template.isTrue
	switch g.Kind() {
	default:
		return g.IsNil()
	case reflect.Array, reflect.Slice, reflect.Map, reflect.String:
		return g.Len() == 0
	case reflect.Bool:
		return !g.Bool()
	case reflect.Complex64, reflect.Complex128:
		return g.Complex() == 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return g.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return g.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return g.Float() == 0
	case reflect.Struct:
		return false
	}
}

// coalesce returns the first non-empty value.
func coalesce(v ...interface{}) interface{} {
	for _, val := range v {
		if !empty(val) {
			return val
		}
	}
	return nil
}

// all returns true if empty(x) is false for all values x in the list.
// If the list is empty, return true.
func all(v ...interface{}) bool {
	for _, val := range v {
		if empty(val) {
			return false
		}
	}
	return true
}

// any returns true if empty(x) is false for any x in the list.
// If the list is empty, return false.
func any(v ...interface{}) bool {
	for _, val := range v {
		if !empty(val) {
			return true
		}
	}
	return false
}

// fromJSON decodes JSON into a structured value, ignoring errors.
func fromJSON(v string) interface{} {
	output, _ := mustFromJSON(v)
	return output
}

// mustFromJSON decodes JSON into a structured value, returning errors.
func mustFromJSON(v string) (interface{}, error) {
	var output interface{}
	err := json.Unmarshal([]byte(v), &output)
	return output, err
}

// toJSON encodes an item into a JSON string
func toJSON(v interface{}) string {
	output, _ := json.Marshal(v)
	return string(output)
}

func mustToJSON(v interface{}) (string, error) {
	output, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// toPrettyJSON encodes an item into a pretty (indented) JSON string
func toPrettyJSON(v interface{}) string {
	output, _ := json.MarshalIndent(v, "", "  ")
	return string(output)
}

func mustToPrettyJSON(v interface{}) (string, error) {
	output, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// toRawJSON encodes an item into a JSON string with no escaping of HTML characters.
func toRawJSON(v interface{}) string {
	output, err := mustToRawJSON(v)
	if err != nil {
		panic(err)
	}
	return string(output)
}

// mustToRawJSON encodes an item into a JSON string with no escaping of HTML characters.
func mustToRawJSON(v interface{}) (string, error) {
	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	err := enc.Encode(&v)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// ternary returns the first value if the last value is true, otherwise returns the second value.
func ternary(vt interface{}, vf interface{}, v bool) interface{} {
	if v {
		return vt
	}

	return vf
}
