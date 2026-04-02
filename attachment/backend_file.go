package attachment

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type fileBackend struct {
	dir string
}

var _ backend = (*fileBackend)(nil)

func newFileBackend(dir string) (*fileBackend, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &fileBackend{dir: dir}, nil
}

func (b *fileBackend) Put(id string, reader io.Reader, untrustedLength int64) error {
	if untrustedLength > 0 {
		reader = io.LimitReader(reader, untrustedLength)
	}
	file := filepath.Join(b.dir, id)
	f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := io.Copy(f, reader)
	if err != nil {
		os.Remove(file)
		return err
	} else if untrustedLength > 0 && n != untrustedLength {
		os.Remove(file)
		return fmt.Errorf("content length mismatch: claimed %d, got %d", untrustedLength, n)
	}
	if err := f.Close(); err != nil {
		os.Remove(file)
		return err
	}
	return nil
}

func (b *fileBackend) List() ([]object, error) {
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		return nil, err
	}
	objects := make([]object, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		objects = append(objects, object{
			ID:           e.Name(),
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})
	}
	return objects, nil
}

func (b *fileBackend) Get(id string) (io.ReadCloser, int64, error) {
	file := filepath.Join(b.dir, id)
	stat, err := os.Stat(file)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, 0, err
	}
	return f, stat.Size(), nil
}

func (b *fileBackend) Delete(ids ...string) error {
	for _, id := range ids {
		file := filepath.Join(b.dir, id)
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (b *fileBackend) DeleteIncomplete(_ time.Time) error {
	return nil
}
