package gitfs_test

import (
	"errors"
	"io"
	"io/fs"
	"sort"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitfs "github.com/tfaller/go-git-fs"
)

var fixedTime = time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)

const submoduleHashHex = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// buildCommit stores a small, fixed file tree directly into an in-memory
// object store and returns the resulting commit, fully wired for
// commit.Tree() to work.
//
// The tree looks like:
//
//	file.txt         "hello file\n"
//	exec.sh          "#!/bin/sh\necho hi\n"   (executable)
//	link             -> file.txt              (symlink)
//	submodule                                  (gitlink)
//	dir/
//	  nested.txt     "nested\n"
//	  deepdir/
//	    deep.txt     "deep\n"
func buildCommit(t *testing.T) *object.Commit {
	t.Helper()

	s := memory.NewStorage()

	fileHash := storeBlob(t, s, "hello file\n")
	execHash := storeBlob(t, s, "#!/bin/sh\necho hi\n")
	linkHash := storeBlob(t, s, "file.txt")
	nestedHash := storeBlob(t, s, "nested\n")
	deepHash := storeBlob(t, s, "deep\n")

	deepTreeHash := storeTree(t, s, []object.TreeEntry{
		{Name: "deep.txt", Mode: filemode.Regular, Hash: deepHash},
	})

	dirTreeHash := storeTree(t, s, []object.TreeEntry{
		{Name: "nested.txt", Mode: filemode.Regular, Hash: nestedHash},
		{Name: "deepdir", Mode: filemode.Dir, Hash: deepTreeHash},
	})

	rootTreeHash := storeTree(t, s, []object.TreeEntry{
		{Name: "file.txt", Mode: filemode.Regular, Hash: fileHash},
		{Name: "exec.sh", Mode: filemode.Executable, Hash: execHash},
		{Name: "link", Mode: filemode.Symlink, Hash: linkHash},
		{Name: "dir", Mode: filemode.Dir, Hash: dirTreeHash},
		{Name: "submodule", Mode: filemode.Submodule, Hash: plumbing.NewHash(submoduleHashHex)},
	})

	commit := &object.Commit{
		Author:    object.Signature{Name: "Test", Email: "test@example.com", When: fixedTime},
		Committer: object.Signature{Name: "Test", Email: "test@example.com", When: fixedTime},
		Message:   "test commit",
		TreeHash:  rootTreeHash,
	}

	obj := s.NewEncodedObject()
	require.NoError(t, commit.Encode(obj))
	commitHash, err := s.SetEncodedObject(obj)
	require.NoError(t, err)

	loaded, err := object.GetCommit(s, commitHash)
	require.NoError(t, err)

	return loaded
}

func storeBlob(t *testing.T, s storer.EncodedObjectStorer, content string) plumbing.Hash {
	t.Helper()

	obj := s.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)

	w, err := obj.Writer()
	require.NoError(t, err)
	_, err = io.WriteString(w, content)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	h, err := s.SetEncodedObject(obj)
	require.NoError(t, err)
	return h
}

func storeTree(t *testing.T, s storer.EncodedObjectStorer, entries []object.TreeEntry) plumbing.Hash {
	t.Helper()

	sort.Slice(entries, func(i, j int) bool {
		return treeSortName(entries[i]) < treeSortName(entries[j])
	})

	tree := &object.Tree{Entries: entries}

	obj := s.NewEncodedObject()
	require.NoError(t, tree.Encode(obj))

	h, err := s.SetEncodedObject(obj)
	require.NoError(t, err)
	return h
}

// treeSortName mirrors git's tree entry ordering, where directory names
// sort as if they had a trailing slash.
func treeSortName(e object.TreeEntry) string {
	if e.Mode == filemode.Dir {
		return e.Name + "/"
	}
	return e.Name
}

func TestOpenFromCommit(t *testing.T) {
	commit := buildCommit(t)

	fsys, err := gitfs.OpenFromCommit(commit)
	require.NoError(t, err)
	require.NotNil(t, fsys)
}

func TestReadFile(t *testing.T) {
	commit := buildCommit(t)
	fsys, err := gitfs.OpenFromCommit(commit)
	require.NoError(t, err)

	content, err := fsys.ReadFile("file.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello file\n", string(content))

	content, err = fsys.ReadFile("dir/deepdir/deep.txt")
	require.NoError(t, err)
	assert.Equal(t, "deep\n", string(content))
}

func TestReadFile_NotExist(t *testing.T) {
	commit := buildCommit(t)
	fsys, err := gitfs.OpenFromCommit(commit)
	require.NoError(t, err)

	_, err = fsys.ReadFile("missing.txt")
	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestReadFile_Directory(t *testing.T) {
	commit := buildCommit(t)
	fsys, err := gitfs.OpenFromCommit(commit)
	require.NoError(t, err)

	_, err = fsys.ReadFile("dir")
	require.Error(t, err)
}

func TestStat(t *testing.T) {
	commit := buildCommit(t)
	fsys, err := gitfs.OpenFromCommit(commit)
	require.NoError(t, err)

	info, err := fsys.Stat(".")
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, ".", info.Name())

	info, err = fsys.Stat("file.txt")
	require.NoError(t, err)
	assert.False(t, info.IsDir())
	assert.Equal(t, "file.txt", info.Name())
	assert.EqualValues(t, len("hello file\n"), info.Size())
	assert.True(t, fixedTime.Equal(info.ModTime()), "expected %v, got %v", fixedTime, info.ModTime())
	assert.Zero(t, info.Mode()&0o111, "regular file should not be executable")

	info, err = fsys.Stat("exec.sh")
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o111, "exec.sh should be executable")

	info, err = fsys.Stat("dir")
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	info, err = fsys.Stat("link")
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&fs.ModeSymlink)

	info, err = fsys.Stat("submodule")
	require.NoError(t, err)
	assert.False(t, info.IsDir())
	assert.NotZero(t, info.Mode()&fs.ModeIrregular)
}

func TestStat_NotExist(t *testing.T) {
	commit := buildCommit(t)
	fsys, err := gitfs.OpenFromCommit(commit)
	require.NoError(t, err)

	_, err = fsys.Stat("does/not/exist")
	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestReadDir(t *testing.T) {
	commit := buildCommit(t)
	fsys, err := gitfs.OpenFromCommit(commit)
	require.NoError(t, err)

	entries, err := fsys.ReadDir(".")
	require.NoError(t, err)

	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"dir", "exec.sh", "file.txt", "link", "submodule"}, names)
	assert.True(t, sort.StringsAreSorted(names))

	entries, err = fsys.ReadDir("dir")
	require.NoError(t, err)
	names = nil
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"deepdir", "nested.txt"}, names)
}

func TestReadDir_NotADirectory(t *testing.T) {
	commit := buildCommit(t)
	fsys, err := gitfs.OpenFromCommit(commit)
	require.NoError(t, err)

	_, err = fsys.ReadDir("file.txt")
	require.Error(t, err)
}

func TestOpen_DirRead(t *testing.T) {
	commit := buildCommit(t)
	fsys, err := gitfs.OpenFromCommit(commit)
	require.NoError(t, err)

	f, err := fsys.Open("dir")
	require.NoError(t, err)
	defer f.Close()

	buf := make([]byte, 1)
	_, err = f.Read(buf)
	require.Error(t, err)
}

func TestReadLink(t *testing.T) {
	commit := buildCommit(t)
	fsys, err := gitfs.OpenFromCommit(commit)
	require.NoError(t, err)

	target, err := fsys.ReadLink("link")
	require.NoError(t, err)
	assert.Equal(t, "file.txt", target)

	info, err := fsys.Lstat("link")
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&fs.ModeSymlink)
}

func TestReadLink_NotASymlink(t *testing.T) {
	commit := buildCommit(t)
	fsys, err := gitfs.OpenFromCommit(commit)
	require.NoError(t, err)

	_, err = fsys.ReadLink("file.txt")
	require.Error(t, err)
}

func TestSubmodule(t *testing.T) {
	commit := buildCommit(t)
	fsys, err := gitfs.OpenFromCommit(commit)
	require.NoError(t, err)

	content, err := fsys.ReadFile("submodule")
	require.NoError(t, err)
	assert.Equal(t, submoduleHashHex, string(content))
}

func TestWalkDir(t *testing.T) {
	commit := buildCommit(t)
	fsys, err := gitfs.OpenFromCommit(commit)
	require.NoError(t, err)

	var files []string
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	require.NoError(t, err)

	sort.Strings(files)
	assert.Equal(t, []string{
		"dir/deepdir/deep.txt",
		"dir/nested.txt",
		"exec.sh",
		"file.txt",
		"link",
		"submodule",
	}, files)
}

func TestGlob(t *testing.T) {
	commit := buildCommit(t)
	fsys, err := gitfs.OpenFromCommit(commit)
	require.NoError(t, err)

	matches, err := fs.Glob(fsys, "dir/*")
	require.NoError(t, err)
	assert.Equal(t, []string{"dir/deepdir", "dir/nested.txt"}, matches)
}

func TestFS_Conformance(t *testing.T) {
	commit := buildCommit(t)
	fsys, err := gitfs.OpenFromCommit(commit)
	require.NoError(t, err)

	err = fstest.TestFS(fsys,
		"file.txt", "exec.sh", "link", "submodule",
		"dir/nested.txt", "dir/deepdir/deep.txt",
	)
	assert.NoError(t, err)
}
