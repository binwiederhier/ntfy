package sprig

import (
	"fmt"
	"reflect"
)

// typeIs returns true if the src is the type named in target.
// It compares the type name of src with the target string.
//
// Parameters:
//   - target: The type name to check against
//   - src: The value whose type will be checked
//
// Returns:
//   - bool: True if the type name of src matches target, false otherwise
func typeIs(target string, src any) bool {
	return target == typeOf(src)
}

// typeIsLike returns true if the src is the type named in target or a pointer to that type.
// This is useful when you need to check for both a type and a pointer to that type.
//
// Parameters:
//   - target: The type name to check against
//   - src: The value whose type will be checked
//
// Returns:
//   - bool: True if the type of src matches target or "*"+target, false otherwise
func typeIsLike(target string, src any) bool {
	t := typeOf(src)
	return target == t || "*"+target == t
}

// typeOf returns the type of a value as a string.
// It uses fmt.Sprintf with the %T format verb to get the type name.
//
// Parameters:
//   - src: The value whose type name will be returned
//
// Returns:
//   - string: The type name of src
func typeOf(src any) string {
	return fmt.Sprintf("%T", src)
}

// kindIs returns true if the kind of src matches the target kind.
// This checks the underlying kind (e.g., "string", "int", "map") rather than the specific type.
//
// Parameters:
//   - target: The kind name to check against
//   - src: The value whose kind will be checked
//
// Returns:
//   - bool: True if the kind of src matches target, false otherwise
func kindIs(target string, src any) bool {
	return target == kindOf(src)
}

// kindOf returns the kind of a value as a string.
// The kind represents the specific Go type category (e.g., "string", "int", "map", "slice").
//
// Parameters:
//   - src: The value whose kind will be returned
//
// Returns:
//   - string: The kind of src as a string
func kindOf(src any) string {
	return reflect.ValueOf(src).Kind().String()
}
