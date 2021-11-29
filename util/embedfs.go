package util

import (
	"embed"
	"errors"
	"io"
	"io/fs"
	"time"
)

type CachingEmbedFS struct {
	ModTime time.Time
	FS      embed.FS
}

func (f CachingEmbedFS) Open(name string) (fs.File, error) {
	file, err := f.FS.Open(name)
	if err != nil {
		return nil, err
	}
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	return &cachingEmbedFile{file, f.ModTime, stat}, nil
}

type cachingEmbedFile struct {
	file    fs.File
	modTime time.Time
	fs.FileInfo
}

func (f cachingEmbedFile) Stat() (fs.FileInfo, error) {
	return f, nil
}

func (f cachingEmbedFile) Read(bytes []byte) (int, error) {
	return f.file.Read(bytes)
}

func (f *cachingEmbedFile) Seek(offset int64, whence int) (int64, error) {
	if seeker, ok := f.file.(io.Seeker); ok {
		return seeker.Seek(offset, whence)
	}
	return 0, errors.New("io.Seeker not implemented")
}

func (f cachingEmbedFile) ModTime() time.Time {
	return f.modTime // We override this!
}

func (f cachingEmbedFile) Close() error {
	return f.file.Close()
}
