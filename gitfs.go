// Package gitfs implements an io/fs.FS over the file tree of a git commit,
// using github.com/go-git/go-git/v6.
package gitfs

import (
	"errors"
	"io"
	"io/fs"
	"path"
	"time"

	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
)

var errIsDirectory = errors.New("is a directory")

// FS is a read-only file system exposing the file tree of a single git
// commit. It implements fs.FS, fs.StatFS, fs.ReadDirFS, fs.ReadFileFS and
// fs.ReadLinkFS.
type FS struct {
	tree    *object.Tree
	modTime time.Time
}

var (
	_ fs.FS         = (*FS)(nil)
	_ fs.StatFS     = (*FS)(nil)
	_ fs.ReadDirFS  = (*FS)(nil)
	_ fs.ReadFileFS = (*FS)(nil)
	_ fs.ReadLinkFS = (*FS)(nil)
)

// OpenFromCommit returns an FS exposing the file tree of commit. All
// entries report the commit's committer time (falling back to its author
// time) as their ModTime, since git trees carry no per-file timestamps.
func OpenFromCommit(commit *object.Commit) (*FS, error) {
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	modTime := commit.Committer.When
	if modTime.IsZero() {
		modTime = commit.Author.When
	}

	return &FS{tree: tree, modTime: modTime}, nil
}

// Open implements fs.FS.
func (f *FS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	if name == "." {
		return newDirFile(f.tree, ".", f.modTime), nil
	}

	entry, err := f.tree.FindEntry(name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	base := path.Base(name)

	switch entry.Mode {
	case filemode.Dir:
		subtree, err := f.tree.Tree(name)
		if err != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: err}
		}
		return newDirFile(subtree, base, f.modTime), nil

	case filemode.Submodule:
		return newSubmoduleFile(base, entry.Hash, f.modTime), nil

	default:
		blob, err := f.tree.TreeEntryFile(entry)
		if err != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: err}
		}
		file, err := newBlobFile(blob, base, f.modTime)
		if err != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: err}
		}
		return file, nil
	}
}

// Stat implements fs.StatFS.
func (f *FS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}

	if name == "." {
		return &fileInfo{name: ".", mode: fs.ModeDir | 0o555, modTime: f.modTime}, nil
	}

	entry, err := f.tree.FindEntry(name)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}

	info, err := (&dirEntry{tree: f.tree, entry: *entry, modTime: f.modTime}).infoAt(name)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}

	return info, nil
}

// ReadDir implements fs.ReadDirFS.
func (f *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	file, err := f.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	dir, ok := file.(fs.ReadDirFile)
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: errors.New("not a directory")}
	}

	return dir.ReadDir(-1)
}

// ReadFile implements fs.ReadFileFS.
func (f *FS) ReadFile(name string) ([]byte, error) {
	file, err := f.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if info, err := file.Stat(); err == nil && info.IsDir() {
		return nil, &fs.PathError{Op: "read", Path: name, Err: errIsDirectory}
	}

	return io.ReadAll(file)
}

// ReadLink implements fs.ReadLinkFS. It returns the target of a symlink
// entry without following it.
func (f *FS) ReadLink(name string) (string, error) {
	if !fs.ValidPath(name) {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: fs.ErrInvalid}
	}

	entry, err := f.tree.FindEntry(name)
	if err != nil {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: fs.ErrNotExist}
	}

	if entry.Mode != filemode.Symlink {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: errors.New("not a symlink")}
	}

	blob, err := f.tree.TreeEntryFile(entry)
	if err != nil {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: err}
	}

	target, err := blob.Contents()
	if err != nil {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: err}
	}

	return target, nil
}

// Lstat implements fs.ReadLinkFS. Since this FS never follows symlinks
// when resolving a path, Lstat behaves identically to Stat.
func (f *FS) Lstat(name string) (fs.FileInfo, error) {
	return f.Stat(name)
}
