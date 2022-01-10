package server

import (
	"errors"
	"heckel.io/ntfy/util"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

var (
	fileIDRegex      = regexp.MustCompile(`^[-_A-Za-z0-9]+$`)
	errInvalidFileID = errors.New("invalid file ID")
	errFileExists    = errors.New("file exists")
)

type fileCache struct {
	dir              string
	totalSizeCurrent int64
	totalSizeLimit   int64
	fileSizeLimit    int64
	mu               sync.Mutex
}

func newFileCache(dir string, totalSizeLimit int64, fileSizeLimit int64) (*fileCache, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	size, err := dirSize(dir)
	if err != nil {
		return nil, err
	}
	return &fileCache{
		dir:              dir,
		totalSizeCurrent: size,
		totalSizeLimit:   totalSizeLimit,
		fileSizeLimit:    fileSizeLimit,
	}, nil
}

func (c *fileCache) Write(id string, in io.Reader, limiters ...*util.Limiter) (int64, error) {
	if !fileIDRegex.MatchString(id) {
		return 0, errInvalidFileID
	}
	file := filepath.Join(c.dir, id)
	if _, err := os.Stat(file); err == nil {
		return 0, errFileExists
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	limiters = append(limiters, util.NewLimiter(c.Remaining()), util.NewLimiter(c.fileSizeLimit))
	limitWriter := util.NewLimitWriter(f, limiters...)
	size, err := io.Copy(limitWriter, in)
	if err != nil {
		os.Remove(file)
		return 0, err
	}
	if err := f.Close(); err != nil {
		os.Remove(file)
		return 0, err
	}
	c.mu.Lock()
	c.totalSizeCurrent += size
	c.mu.Unlock()
	return size, nil
}

func (c *fileCache) Remove(ids ...string) error {
	for _, id := range ids {
		if !fileIDRegex.MatchString(id) {
			return errInvalidFileID
		}
		file := filepath.Join(c.dir, id)
		_ = os.Remove(file) // Best effort delete
	}
	size, err := dirSize(c.dir)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.totalSizeCurrent = size
	c.mu.Unlock()
	return nil
}

func (c *fileCache) Size() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.totalSizeCurrent
}

func (c *fileCache) Remaining() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	remaining := c.totalSizeLimit - c.totalSizeCurrent
	if remaining < 0 {
		return 0
	}
	return remaining
}

func dirSize(dir string) (int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	var size int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			return 0, err
		}
		size += info.Size()
	}
	return size, nil
}
