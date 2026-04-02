package attachment

import (
	"io"
	"time"
)

// backendObject represents an object stored in a backend.
type object struct {
	ID           string
	Size         int64
	LastModified time.Time
}

// backend is a minimal I/O interface for storing and retrieving attachment files.
// It has no knowledge of size tracking, limiting, or ID validation.
type backend interface {
	Put(id string, reader io.Reader, untrustedLength int64) error
	Get(id string) (io.ReadCloser, int64, error)
	List() ([]object, error)
	Delete(ids ...string) error
	DeleteIncomplete(cutoff time.Time) error
}
