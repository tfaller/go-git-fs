# go-git-fs

[![Go Reference](https://pkg.go.dev/badge/github.com/tfaller/go-git-fs.svg)](https://pkg.go.dev/github.com/tfaller/go-git-fs)

`gitfs` implements a read-only [`io/fs.FS`](https://pkg.go.dev/io/fs#FS) over
the file tree of a single git commit, built on top of
[go-git](https://github.com/go-git/go-git). It lets you browse, read, and
walk a commit's files using the standard library's `fs` APIs, without
checking the commit out to disk.

## Features

- Implements `fs.FS`, `fs.StatFS`, `fs.ReadDirFS`, `fs.ReadFileFS`, and
  `fs.ReadLinkFS`, so it works with `fs.WalkDir`, `fs.Glob`,
  `fs.ReadFile`, and other `io/fs`-based tooling out of the box.
- Directories, regular files, executable files, and symlinks are mapped to
  their natural `fs.FileMode` equivalents.
- Submodules (gitlinks) are exposed as regular files whose content is the
  hex hash of the commit they point to, so tree walks don't fail on them.
- All entries report the commit's committer time (falling back to author
  time) as `ModTime`, since git trees don't carry per-file timestamps.

## Installation

```sh
go get github.com/tfaller/go-git-fs
```

## Usage

```go
package main

import (
	"fmt"
	"io/fs"

	"github.com/go-git/go-git/v6"
	"github.com/tfaller/go-git-fs"
)

func main() {
	repo, err := git.PlainOpen(".")
	if err != nil {
		panic(err)
	}

	head, err := repo.Head()
	if err != nil {
		panic(err)
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		panic(err)
	}

	fsys, err := gitfs.OpenFromCommit(commit)
	if err != nil {
		panic(err)
	}

	// Walk the commit's file tree just like a regular directory.
	fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	})
}
```

Because `*gitfs.FS` implements `fs.FS`, it also works with anything that
accepts one, e.g. `http.FileServer(http.FS(fsys))` to serve a commit's
contents over HTTP.

## Limitations

- The file system is read-only; there is no way to modify the underlying
  git objects through it.
- Symlinks are exposed but never followed automatically — use `ReadLink`
  to resolve a symlink's target yourself.