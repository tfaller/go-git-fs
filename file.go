package gitfs

import (
	"bytes"
	"io"
	"io/fs"
	"time"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
)

// blobFile is an fs.File backed by the content of a git blob (a regular
// file, an executable file, or a symlink's target text).
type blobFile struct {
	info   *fileInfo
	reader io.ReadCloser
}

var _ fs.File = (*blobFile)(nil)

func newBlobFile(f *object.File, name string, modTime time.Time) (*blobFile, error) {
	r, err := f.Reader()
	if err != nil {
		return nil, err
	}

	return &blobFile{
		reader: r,
		info: &fileInfo{
			name:    name,
			size:    f.Size,
			mode:    toFsMode(f.Mode),
			modTime: modTime,
		},
	}, nil
}

func (f *blobFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *blobFile) Read(b []byte) (int, error) { return f.reader.Read(b) }
func (f *blobFile) Close() error               { return f.reader.Close() }

// submoduleFile is an fs.File representing a submodule (gitlink) entry.
// Its content is the hex hash of the commit the submodule points to.
type submoduleFile struct {
	info   *fileInfo
	reader *bytes.Reader
}

var _ fs.File = (*submoduleFile)(nil)

func newSubmoduleFile(name string, hash plumbing.Hash, modTime time.Time) *submoduleFile {
	content := []byte(hash.String())

	return &submoduleFile{
		reader: bytes.NewReader(content),
		info: &fileInfo{
			name:    name,
			size:    int64(len(content)),
			mode:    toFsMode(filemode.Submodule),
			modTime: modTime,
		},
	}
}

func (f *submoduleFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *submoduleFile) Read(b []byte) (int, error) { return f.reader.Read(b) }
func (f *submoduleFile) Close() error               { return nil }
