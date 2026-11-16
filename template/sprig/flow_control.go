package sprig

import "errors"

// fail is a function that always returns an error with the given message.
func fail(msg string) (string, error) {
	return "", errors.New(msg)
}
