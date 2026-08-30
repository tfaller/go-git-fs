package gitfs

import (
	"io/fs"
	"path"
	"sort"
	"time"

	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
)

// dirEntry implements fs.DirEntry for a single entry of a git tree.
type dirEntry struct {
	tree    *object.Tree
	entry   object.TreeEntry
	modTime time.Time
}

var _ fs.DirEntry = (*dirEntry)(nil)

func (d *dirEntry) Name() string      { return d.entry.Name }
func (d *dirEntry) IsDir() bool       { return d.entry.Mode == filemode.Dir }
func (d *dirEntry) Type() fs.FileMode { return toFsMode(d.entry.Mode).Type() }

// Info implements fs.DirEntry. The entry's size is looked up as a child of
// d.tree, so d.tree must be the tree that directly contains this entry.
func (d *dirEntry) Info() (fs.FileInfo, error) {
	return d.infoAt(d.entry.Name)
}

// infoAt builds the fs.FileInfo for this entry as if it were reached via
// treePath from d.tree, i.e. d.tree.Size(treePath) must resolve to this
// entry's blob. This lets FS.Stat reuse dirEntry's size/mode logic for an
// entry found deep inside f.tree via a multi-component path.
func (d *dirEntry) infoAt(treePath string) (fs.FileInfo, error) {
	mode := toFsMode(d.entry.Mode)

	var size int64
	switch d.entry.Mode {
	case filemode.Dir:
		// Directories have no meaningful size.
	case filemode.Submodule:
		size = int64(len(d.entry.Hash.String()))
	default:
		s, err := d.tree.Size(treePath)
		if err != nil {
			return nil, err
		}
		size = s
	}

	return &fileInfo{
		name:    path.Base(treePath),
		size:    size,
		mode:    mode,
		modTime: d.modTime,
	}, nil
}

// dirEntries returns the sorted directory entries of tree, as required by
// the fs.ReadDirFile and fs.ReadDirFS contracts.
func dirEntries(tree *object.Tree, modTime time.Time) []fs.DirEntry {
	entries := make([]fs.DirEntry, len(tree.Entries))
	for i, e := range tree.Entries {
		entries[i] = &dirEntry{tree: tree, entry: e, modTime: modTime}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	return entries
}
