//go:build !windows

package cacheify

import (
	"os"
)

// atomicRename performs an atomic rename operation
// On Unix systems, os.Rename is atomic when source and destination are on the same filesystem
func atomicRename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
