package gitfs

import (
	"io"
	"io/fs"
	"time"

	"github.com/go-git/go-git/v6/plumbing/object"
)

// dirFile is an fs.ReadDirFile backed by a git tree.
type dirFile struct {
	tree    *object.Tree
	info    *fileInfo
	entries []fs.DirEntry
	offset  int
}

var (
	_ fs.File        = (*dirFile)(nil)
	_ fs.ReadDirFile = (*dirFile)(nil)
)

func newDirFile(tree *object.Tree, name string, modTime time.Time) *dirFile {
	return &dirFile{
		tree: tree,
		info: &fileInfo{
			name:    name,
			mode:    fs.ModeDir | 0o555,
			modTime: modTime,
		},
	}
}

func (d *dirFile) Stat() (fs.FileInfo, error) { return d.info, nil }

func (d *dirFile) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.info.name, Err: errIsDirectory}
}

func (d *dirFile) Close() error { return nil }

func (d *dirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.entries == nil {
		d.entries = dirEntries(d.tree, d.info.modTime)
	}

	remaining := len(d.entries) - d.offset

	if n <= 0 {
		entries := d.entries[d.offset:]
		d.offset = len(d.entries)
		return entries, nil
	}

	if remaining == 0 {
		return nil, io.EOF
	}

	if n > remaining {
		n = remaining
	}

	entries := d.entries[d.offset : d.offset+n]
	d.offset += n

	return entries, nil
}
