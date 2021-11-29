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

func (e CachingEmbedFS) Open(name string) (fs.File, error) {
	f, err := e.FS.Open(name)
	if err != nil {
		return nil, err
	}
	return &cachingEmbedFile{f, e.ModTime}, nil
}

type cachingEmbedFile struct {
	file    fs.File
	modTime time.Time
}

func (f cachingEmbedFile) Stat() (fs.FileInfo, error) {
	s, err := f.file.Stat()
	if err != nil {
		return nil, err
	}
	return &etagEmbedFileInfo{s, f.modTime}, nil
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

func (f cachingEmbedFile) Close() error {
	return f.file.Close()
}

type etagEmbedFileInfo struct {
	file    fs.FileInfo
	modTime time.Time
}

func (e etagEmbedFileInfo) Name() string {
	return e.file.Name()
}

func (e etagEmbedFileInfo) Size() int64 {
	return e.file.Size()
}

func (e etagEmbedFileInfo) Mode() fs.FileMode {
	return e.file.Mode()
}

func (e etagEmbedFileInfo) ModTime() time.Time {
	return e.modTime // We override this!
}

func (e etagEmbedFileInfo) IsDir() bool {
	return e.file.IsDir()
}

func (e etagEmbedFileInfo) Sys() interface{} {
	return e.file.Sys()
}
