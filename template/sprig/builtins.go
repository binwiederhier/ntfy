package sprig

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"heckel.io/ntfy/v2/template"
)

var (
	// printfDirectiveRegex matches the printf directives a template may use: %%, or flags, a width
	// and a precision of at most three digits each (so below 1000), and a verb. Stars (a width taken
	// from an argument), explicit argument indexes and %p (a memory address) are not allowed.
	printfDirectiveRegex = regexp.MustCompile(`%%|%[-+# 0]*[0-9]{0,3}(\.[0-9]{0,3})?[vTtbcdoOqxXUeEfFgGs]`)
)

// Errors returned by templatePrintf
var (
	errPrintfDirectiveNotAllowed = errors.New("printf directive not allowed, only widths and precisions below 1000 and no argument indexes")
	errPrintfListOrDictArgs      = errors.New("printf does not accept lists or dicts as arguments")
)

// templatePrint replaces the template builtin print (functions passed via Funcs take precedence
// over the builtins). fmt formats all arguments into one string inside a single call, where neither
// the render deadline nor the output limit can intervene, and it has no cycle detection, so the
// arguments are checked first: a value that references itself, or a list holding many references
// to one large string, is rejected before anything is formatted.
func templatePrint(args ...any) (string, error) {
	if err := template.CheckPrintable(args...); err != nil {
		return "", err
	}
	return fmt.Sprint(args...), nil
}

// templatePrintln replaces the template builtin println, guarded like templatePrint
func templatePrintln(args ...any) (string, error) {
	if err := template.CheckPrintable(args...); err != nil {
		return "", err
	}
	return fmt.Sprintln(args...), nil
}

// templatePrintf is the template builtin printf, guarded against memory amplification: fmt
// allows widths and precisions up to 1e6 per verb, so a small template like
// {{printf "%999999d%999999d..." ...}} can allocate gigabytes inside a single fmt call -- and the
// executor's cancellation context is only checked between template nodes, never inside one.
// Widths and precisions of 1000 or more are therefore rejected, as is the star (*) form, which
// takes the width from an argument. Combined with the template size limit, this bounds a single
// render to a few MB. Registered via Funcs, which takes precedence over the builtin, and checked
// at call time so a format string assembled during execution is covered too.
//
// A width also pads every element of a list, and an explicit argument index (%[1]s) prints one
// argument many times, so lists, dicts and argument indexes are rejected as well. Rather than
// looking for dangerous directives, the format may only contain the ones printfDirectiveRegex
// allows: fmt pads even malformed directives (e.g. "%000 9999999#" emits 10 MB), so anything
// unrecognized is rejected.
func templatePrintf(format string, args ...any) (string, error) {
	if strings.Contains(printfDirectiveRegex.ReplaceAllString(format, ""), "%") {
		return "", errPrintfDirectiveNotAllowed // A % that is not part of an allowed directive
	}
	for _, arg := range args {
		switch reflect.Indirect(reflect.ValueOf(arg)).Kind() { // fmt prints a pointer to a list as &[...]
		case reflect.Slice, reflect.Array, reflect.Map:
			return "", errPrintfListOrDictArgs
		}
	}
	if err := template.CheckPrintable(args...); err != nil {
		return "", err
	}
	return fmt.Sprintf(format, args...), nil
}
