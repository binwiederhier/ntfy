package sprig

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
)

// defaultValue checks whether `given` is set, and returns default if not set.
//
// This returns `d` if `given` appears not to be set, and `given` otherwise.
//
// For numeric types 0 is unset.
// For strings, maps, arrays, and slices, len() = 0 is considered unset.
// For bool, false is unset.
// Structs are never considered unset.
//
// For everything else, including pointers, a nil value is unset.
func defaultValue(d any, given ...any) any {
	if empty(given) || empty(given[0]) {
		return d
	}
	return given[0]
}

// empty returns true if the given value has the zero value for its type.
// This is a helper function used by defaultValue, coalesce, all, and anyNonEmpty.
//
// The following values are considered empty:
// - Invalid values
// - nil values
// - Zero-length arrays, slices, maps, and strings
// - Boolean false
// - Zero for all numeric types
// - Structs are never considered empty
//
// Parameters:
//   - given: The value to check for emptiness
//
// Returns:
//   - bool: True if the value is considered empty, false otherwise
func empty(given any) bool {
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

// coalesce returns the first non-empty value from a list of values.
// If all values are empty, it returns nil.
//
// This is useful for providing a series of fallback values.
//
// Parameters:
//   - v: A variadic list of values to check
//
// Returns:
//   - any: The first non-empty value, or nil if all values are empty
func coalesce(v ...any) any {
	for _, val := range v {
		if !empty(val) {
			return val
		}
	}
	return nil
}

// all checks if all values in a list are non-empty.
// Returns true if every value in the list is non-empty.
// If the list is empty, returns true (vacuously true).
//
// Parameters:
//   - v: A variadic list of values to check
//
// Returns:
//   - bool: True if all values are non-empty, false otherwise
func all(v ...any) bool {
	return !slices.ContainsFunc(v, empty)
}

// anyNonEmpty checks if at least one value in a list is non-empty.
// Returns true if any value in the list is non-empty.
// If the list is empty, returns false.
//
// Parameters:
//   - v: A variadic list of values to check
//
// Returns:
//   - bool: True if at least one value is non-empty, false otherwise
func anyNonEmpty(v ...any) bool {
	for _, val := range v {
		if !empty(val) {
			return true
		}
	}
	return false
}

// fromJSON decodes a JSON string into a structured value.
// This function ignores any errors that occur during decoding.
// If the JSON is invalid, it returns nil.
//
// Parameters:
//   - v: The JSON string to decode
//
// Returns:
//   - any: The decoded value, or nil if decoding failed
func fromJSON(v string) any {
	output, _ := mustFromJSON(v)
	return output
}

// mustFromJSON decodes a JSON string into a structured value.
// Unlike fromJSON, this function returns any errors that occur during decoding.
//
// Parameters:
//   - v: The JSON string to decode
//
// Returns:
//   - any: The decoded value
//   - error: Any error that occurred during decoding
func mustFromJSON(v string) (any, error) {
	var output any
	err := json.Unmarshal([]byte(v), &output)
	return output, err
}

// toJSON encodes a value into a JSON string.
// This function ignores any errors that occur during encoding.
// If the value cannot be encoded, it returns an empty string.
//
// Parameters:
//   - v: The value to encode to JSON
//
// Returns:
//   - string: The JSON string representation of the value
func toJSON(v any) string {
	output, _ := json.Marshal(v)
	return string(output)
}

// mustToJSON encodes a value into a JSON string.
// Unlike toJSON, this function returns any errors that occur during encoding.
//
// Parameters:
//   - v: The value to encode to JSON
//
// Returns:
//   - string: The JSON string representation of the value
//   - error: Any error that occurred during encoding
func mustToJSON(v any) (string, error) {
	output, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// toPrettyJSON encodes a value into a pretty (indented) JSON string.
// This function ignores any errors that occur during encoding.
// If the value cannot be encoded, it returns an empty string.
//
// Parameters:
//   - v: The value to encode to JSON
//
// Returns:
//   - string: The indented JSON string representation of the value
func toPrettyJSON(v any) string {
	output, _ := json.MarshalIndent(v, "", "  ")
	return string(output)
}

// mustToPrettyJSON encodes a value into a pretty (indented) JSON string.
// Unlike toPrettyJSON, this function returns any errors that occur during encoding.
//
// Parameters:
//   - v: The value to encode to JSON
//
// Returns:
//   - string: The indented JSON string representation of the value
//   - error: Any error that occurred during encoding
func mustToPrettyJSON(v any) (string, error) {
	output, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// toRawJSON encodes a value into a JSON string with no escaping of HTML characters.
// This function panics if an error occurs during encoding.
// Unlike toJSON, HTML characters like <, >, and & are not escaped.
//
// Parameters:
//   - v: The value to encode to JSON
//
// Returns:
//   - string: The JSON string representation of the value without HTML escaping
func toRawJSON(v any) string {
	output, err := mustToRawJSON(v)
	if err != nil {
		panic(err)
	}
	return output
}

// mustToRawJSON encodes a value into a JSON string with no escaping of HTML characters.
// Unlike toRawJSON, this function returns any errors that occur during encoding.
// HTML characters like <, >, and & are not escaped in the output.
//
// Parameters:
//   - v: The value to encode to JSON
//
// Returns:
//   - string: The JSON string representation of the value without HTML escaping
//   - error: Any error that occurred during encoding
func mustToRawJSON(v any) (string, error) {
	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(&v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// ternary implements a conditional (ternary) operator.
// It returns the first value if the condition is true, otherwise returns the second value.
// This is similar to the ?: operator in many programming languages.
//
// Parameters:
//   - vt: The value to return if the condition is true
//   - vf: The value to return if the condition is false
//   - v: The boolean condition to evaluate
//
// Returns:
//   - any: Either vt or vf depending on the value of v
func ternary(vt any, vf any, v bool) any {
	if v {
		return vt
	}
	return vf
}
