package iso

import (
	"errors"
	"io"
	"io/fs"
	"time"
)

const (
	dirPermissions  fs.FileMode = 0o555
	filePermissions fs.FileMode = 0o444
)

type fileInfo struct{ item entry }

func (info fileInfo) Name() string { return info.item.name }
func (info fileInfo) Size() int64 {
	if info.item.dir {
		return 0
	}
	return info.item.size
}

func (info fileInfo) Mode() fs.FileMode {
	if info.item.dir {
		return fs.ModeDir | dirPermissions
	}
	return filePermissions
}
func (info fileInfo) ModTime() time.Time { return time.Time{} }
func (info fileInfo) IsDir() bool        { return info.item.dir }
func (info fileInfo) Sys() any           { return nil }

type dirEntry struct {
	img  *Image
	item entry
}

func (de dirEntry) Name() string { return de.item.name }
func (de dirEntry) IsDir() bool  { return de.item.dir }
func (de dirEntry) Type() fs.FileMode {
	return fileInfo{item: de.item}.Mode().Type()
}

func (de dirEntry) Info() (fs.FileInfo, error) {
	item := de.item
	if err := de.img.vol.resolve(de.img.ctx, &item); err != nil {
		return nil, err
	}
	return fileInfo{item: item}, nil
}

// file is a regular file; it implements io.Reader, io.ReaderAt and io.Seeker.
type file struct {
	img  *Image
	item entry
	pos  int64
}

func (f *file) Stat() (fs.FileInfo, error) { return fileInfo{item: f.item}, nil }
func (f *file) Close() error               { return nil }

// ReadAt implements io.ReaderAt.
func (f *file) ReadAt(buf []byte, offset int64) (int, error) {
	if offset < 0 {
		return 0, errors.New("negative offset")
	}
	if offset >= f.item.size {
		if len(buf) == 0 {
			return 0, nil
		}
		return 0, io.EOF
	}
	return f.img.src.readExtents(f.img.ctx, f.item.extents, buf, offset)
}

func (f *file) Read(buf []byte) (int, error) {
	count, err := f.ReadAt(buf, f.pos)
	f.pos += int64(count)
	if count > 0 && errors.Is(err, io.EOF) {
		err = nil
	}
	return count, err
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	var target int64
	switch whence {
	case io.SeekStart:
		target = offset
	case io.SeekCurrent:
		target = f.pos + offset
	case io.SeekEnd:
		target = f.item.size + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if target < 0 {
		return 0, errors.New("negative position")
	}
	f.pos = target
	return target, nil
}

// dirFile is an open directory.
type dirFile struct {
	item     entry
	children []fs.DirEntry
}

func (d *dirFile) Stat() (fs.FileInfo, error) { return fileInfo{item: d.item}, nil }
func (d *dirFile) Close() error               { return nil }
func (d *dirFile) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.item.name, Err: errors.New("is a directory")}
}

// ReadDir implements fs.ReadDirFile.
func (d *dirFile) ReadDir(count int) ([]fs.DirEntry, error) {
	if count <= 0 {
		out := d.children
		d.children = nil
		return out, nil
	}
	if len(d.children) == 0 {
		return nil, io.EOF
	}
	count = min(count, len(d.children))
	out := d.children[:count]
	d.children = d.children[count:]
	return out, nil
}
