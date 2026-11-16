package sprig

import (
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
)

// toFloat64 converts a value to a 64-bit float.
// It handles various input types:
// - string: parsed as a float, returns 0 if parsing fails
// - integer types: converted to float64
// - unsigned integer types: converted to float64
// - float types: returned as is
// - bool: true becomes 1.0, false becomes 0.0
// - other types: returns 0.0
//
// Parameters:
//   - v: The value to convert to float64
//
// Returns:
//   - float64: The converted value
func toFloat64(v any) float64 {
	if str, ok := v.(string); ok {
		iv, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return 0
		}
		return iv
	}

	val := reflect.Indirect(reflect.ValueOf(v))
	switch val.Kind() {
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		return float64(val.Int())
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return float64(val.Uint())
	case reflect.Uint, reflect.Uint64:
		return float64(val.Uint())
	case reflect.Float32, reflect.Float64:
		return val.Float()
	case reflect.Bool:
		if val.Bool() {
			return 1
		}
		return 0
	default:
		return 0
	}
}

// toInt converts a value to a 32-bit integer.
// This is a wrapper around toInt64 that casts the result to int.
//
// Parameters:
//   - v: The value to convert to int
//
// Returns:
//   - int: The converted value
func toInt(v any) int {
	// It's not optimal. But I don't want duplicate toInt64 code.
	return int(toInt64(v))
}

// toInt64 converts a value to a 64-bit integer.
// It handles various input types:
// - string: parsed as an integer, returns 0 if parsing fails
// - integer types: converted to int64
// - unsigned integer types: converted to int64 (values > MaxInt64 become MaxInt64)
// - float types: truncated to int64
// - bool: true becomes 1, false becomes 0
// - other types: returns 0
func toInt64(v any) int64 {
	if str, ok := v.(string); ok {
		iv, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			return 0
		}
		return iv
	}
	val := reflect.Indirect(reflect.ValueOf(v))
	switch val.Kind() {
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		return val.Int()
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return int64(val.Uint())
	case reflect.Uint, reflect.Uint64:
		tv := val.Uint()
		if tv <= math.MaxInt64 {
			return int64(tv)
		}
		// TODO: What is the sensible thing to do here?
		return math.MaxInt64
	case reflect.Float32, reflect.Float64:
		return int64(val.Float())
	case reflect.Bool:
		if val.Bool() {
			return 1
		}
		return 0
	default:
		return 0
	}
}

// add1 increments a value by 1.
// The input is first converted to int64 using toInt64.
//
// Parameters:
//   - i: The value to increment
//
// Returns:
//   - int64: The incremented value
func add1(i any) int64 {
	return toInt64(i) + 1
}

// add sums all the provided values.
// All inputs are converted to int64 using toInt64 before addition.
//
// Parameters:
//   - i: A variadic list of values to sum
//
// Returns:
//   - int64: The sum of all values
func add(i ...any) int64 {
	var a int64
	for _, b := range i {
		a += toInt64(b)
	}
	return a
}

// sub subtracts the second value from the first.
// Both inputs are converted to int64 using toInt64 before subtraction.
//
// Parameters:
//   - a: The value to subtract from
//   - b: The value to subtract
//
// Returns:
//   - int64: The result of a - b
func sub(a, b any) int64 {
	return toInt64(a) - toInt64(b)
}

// div divides the first value by the second.
// Both inputs are converted to int64 using toInt64 before division.
// Note: This performs integer division, so the result is truncated.
//
// Parameters:
//   - a: The dividend
//   - b: The divisor
//
// Returns:
//   - int64: The result of a / b
//
// Panics:
//   - If b evaluates to 0 (division by zero)
func div(a, b any) int64 {
	return toInt64(a) / toInt64(b)
}

// mod returns the remainder of dividing the first value by the second.
// Both inputs are converted to int64 using toInt64 before the modulo operation.
//
// Parameters:
//   - a: The dividend
//   - b: The divisor
//
// Returns:
//   - int64: The remainder of a / b
//
// Panics:
//   - If b evaluates to 0 (modulo by zero)
func mod(a, b any) int64 {
	return toInt64(a) % toInt64(b)
}

// mul multiplies all the provided values.
// All inputs are converted to int64 using toInt64 before multiplication.
//
// Parameters:
//   - a: The first value to multiply
//   - v: Additional values to multiply with a
//
// Returns:
//   - int64: The product of all values
func mul(a any, v ...any) int64 {
	val := toInt64(a)
	for _, b := range v {
		val = val * toInt64(b)
	}
	return val
}

// randInt generates a random integer between min (inclusive) and max (exclusive).
//
// Parameters:
//   - min: The lower bound (inclusive)
//   - max: The upper bound (exclusive)
//
// Returns:
//   - int: A random integer in the range [min, max)
//
// Panics:
//   - If max <= min (via rand.Intn)
func randInt(min, max int) int {
	return rand.Intn(max-min) + min
}

// maxAsInt64 returns the maximum value from a list of values as an int64.
// All inputs are converted to int64 using toInt64 before comparison.
//
// Parameters:
//   - a: The first value to compare
//   - i: Additional values to compare
//
// Returns:
//   - int64: The maximum value from all inputs
func maxAsInt64(a any, i ...any) int64 {
	aa := toInt64(a)
	for _, b := range i {
		bb := toInt64(b)
		if bb > aa {
			aa = bb
		}
	}
	return aa
}

// maxAsFloat64 returns the maximum value from a list of values as a float64.
// All inputs are converted to float64 using toFloat64 before comparison.
//
// Parameters:
//   - a: The first value to compare
//   - i: Additional values to compare
//
// Returns:
//   - float64: The maximum value from all inputs
func maxAsFloat64(a any, i ...any) float64 {
	m := toFloat64(a)
	for _, b := range i {
		m = math.Max(m, toFloat64(b))
	}
	return m
}

// minAsInt64 returns the minimum value from a list of values as an int64.
// All inputs are converted to int64 using toInt64 before comparison.
//
// Parameters:
//   - a: The first value to compare
//   - i: Additional values to compare
//
// Returns:
//   - int64: The minimum value from all inputs
func minAsInt64(a any, i ...any) int64 {
	aa := toInt64(a)
	for _, b := range i {
		bb := toInt64(b)
		if bb < aa {
			aa = bb
		}
	}
	return aa
}

// minAsFloat64 returns the minimum value from a list of values as a float64.
// All inputs are converted to float64 using toFloat64 before comparison.
//
// Parameters:
//   - a: The first value to compare
//   - i: Additional values to compare
//
// Returns:
//   - float64: The minimum value from all inputs
func minAsFloat64(a any, i ...any) float64 {
	m := toFloat64(a)
	for _, b := range i {
		m = math.Min(m, toFloat64(b))
	}
	return m
}

// until generates a sequence of integers from 0 to count (exclusive).
// If count is negative, it generates a sequence from 0 to count (inclusive) with step -1.
//
// Parameters:
//   - count: The end value (exclusive if positive, inclusive if negative)
//
// Returns:
//   - []int: A slice containing the generated sequence
func until(count int) []int {
	step := 1
	if count < 0 {
		step = -1
	}
	return untilStep(0, count, step)
}

// untilStep generates a sequence of integers from start to stop with the specified step.
// The sequence is generated as follows:
// - If step is 0, returns an empty slice
// - If stop < start and step < 0, generates a decreasing sequence from start to stop (exclusive)
// - If stop > start and step > 0, generates an increasing sequence from start to stop (exclusive)
// - Otherwise, returns an empty slice
//
// Parameters:
//   - start: The starting value (inclusive)
//   - stop: The ending value (exclusive)
//   - step: The increment between values
//
// Returns:
//   - []int: A slice containing the generated sequence
//
// Panics:
//   - If the number of iterations would exceed loopExecutionLimit
func untilStep(start, stop, step int) []int {
	var v []int
	if step == 0 {
		return v
	}
	// Divide by |step|, or a negative step yields a negative count that slips past the limit
	iterations := math.Abs(float64(stop)-float64(start)) / math.Abs(float64(step))
	if iterations > loopExecutionLimit {
		panic(fmt.Sprintf("too many iterations in untilStep; max allowed is %d, got %f", loopExecutionLimit, iterations))
	}
	if stop < start {
		if step >= 0 {
			return v
		}
		for i := start; i > stop; i += step {
			v = append(v, i)
		}
		return v
	}
	if step <= 0 {
		return v
	}
	for i := start; i < stop; i += step {
		v = append(v, i)
	}
	return v
}

// floor returns the greatest integer value less than or equal to the input.
// The input is first converted to float64 using toFloat64.
//
// Parameters:
//   - a: The value to floor
//
// Returns:
//   - float64: The greatest integer value less than or equal to a
func floor(a any) float64 {
	return math.Floor(toFloat64(a))
}

// ceil returns the least integer value greater than or equal to the input.
// The input is first converted to float64 using toFloat64.
//
// Parameters:
//   - a: The value to ceil
//
// Returns:
//   - float64: The least integer value greater than or equal to a
func ceil(a any) float64 {
	return math.Ceil(toFloat64(a))
}

// round rounds a number to a specified number of decimal places.
// The input is first converted to float64 using toFloat64.
//
// Parameters:
//   - a: The value to round
//   - p: The number of decimal places to round to
//   - rOpt: Optional rounding threshold (default is 0.5)
//
// Returns:
//   - float64: The rounded value
//
// Examples:
//   - round(3.14159, 2) returns 3.14
//   - round(3.14159, 2, 0.6) returns 3.14 (only rounds up if fraction ≥ 0.6)
func round(a any, p int, rOpt ...float64) float64 {
	roundOn := .5
	if len(rOpt) > 0 {
		roundOn = rOpt[0]
	}
	val := toFloat64(a)
	places := toFloat64(p)
	var round float64
	pow := math.Pow(10, places)
	digit := pow * val
	_, div := math.Modf(digit)
	if div >= roundOn {
		round = math.Ceil(digit)
	} else {
		round = math.Floor(digit)
	}
	return round / pow
}

// toDecimal converts a value from octal to decimal.
// The input is first converted to a string using fmt.Sprint, then parsed as an octal number.
// If the parsing fails, it returns 0.
//
// Parameters:
//   - v: The octal value to convert
//
// Returns:
//   - int64: The decimal representation of the octal value
func toDecimal(v any) int64 {
	checkPrintable(v)
	result, err := strconv.ParseInt(fmt.Sprint(v), 8, 64)
	if err != nil {
		return 0
	}
	return result
}

// atoi converts a string to an integer.
// If the conversion fails, it returns 0.
//
// Parameters:
//   - a: The string to convert
//
// Returns:
//   - int: The integer value of the string
func atoi(a string) int {
	i, _ := strconv.Atoi(a)
	return i
}

// seq generates a sequence of integers and returns them as a space-delimited string.
// The behavior depends on the number of parameters:
// - 0 params: Returns an empty string
// - 1 param: Generates sequence from 1 to param[0]
// - 2 params: Generates sequence from param[0] to param[1]
// - 3 params: Generates sequence from param[0] to param[2] with step param[1]
//
// If the end is less than the start, the sequence will be decreasing unless
// a positive step is explicitly provided (which would result in an empty string).
//
// Parameters:
//   - params: Variable number of integers defining the sequence
//
// Returns:
//   - string: A space-delimited string of the generated sequence
func seq(params ...int) string {
	increment := 1
	switch len(params) {
	case 0:
		return ""
	case 1:
		start := 1
		end := params[0]
		if end < start {
			increment = -1
		}
		return intArrayToString(untilStep(start, end+increment, increment), " ")
	case 3:
		start := params[0]
		end := params[2]
		step := params[1]
		if end < start {
			increment = -1
			if step > 0 {
				return ""
			}
		}
		return intArrayToString(untilStep(start, end+increment, step), " ")
	case 2:
		start := params[0]
		end := params[1]
		step := 1
		if end < start {
			step = -1
		}
		return intArrayToString(untilStep(start, end+step, step), " ")
	default:
		return ""
	}
}

// intArrayToString converts a slice of integers to a space-delimited string.
// The function removes the square brackets that would normally appear when
// converting a slice to a string.
//
// Parameters:
//   - slice: The slice of integers to convert
//   - delimiter: The delimiter to use between elements
//
// Returns:
//   - string: A delimited string representation of the integer slice
func intArrayToString(slice []int, delimiter string) string {
	return strings.Trim(strings.Join(strings.Fields(fmt.Sprint(slice)), delimiter), "[]")
}
