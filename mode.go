package gitfs

import (
	"io/fs"

	"github.com/go-git/go-git/v6/plumbing/filemode"
)

// toFsMode maps a git tree entry mode to an fs.FileMode. The permission
// bits are synthetic since git only tracks an executable bit; the tree
// exposed by this package is always read-only.
func toFsMode(m filemode.FileMode) fs.FileMode {
	switch m {
	case filemode.Dir:
		return fs.ModeDir | 0o555
	case filemode.Executable:
		return 0o555
	case filemode.Symlink:
		return fs.ModeSymlink | 0o444
	case filemode.Submodule:
		// Submodules are gitlinks to a commit in another repository, not
		// a blob or tree this FS can descend into.
		return fs.ModeIrregular | 0o444
	default:
		// Regular and Deprecated (and any other unknown mode) are
		// treated as plain files.
		return 0o444
	}
}
