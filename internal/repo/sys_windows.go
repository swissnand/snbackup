//go:build windows

package repo

import (
	"io/fs"
	"path/filepath"
	"strings"
)

func statExtra(fi fs.FileInfo) (uid, gid int, inode, dev uint64) { return 0, 0, 0, 0 }

// StoreName converts "C:\Users\x" into "C/Users/x" and UNC paths
// "\\server\share\x" into "UNC/server/share/x".
func StoreName(abs string) string {
	if strings.HasPrefix(abs, `\\`) {
		return "UNC/" + filepath.ToSlash(abs[2:])
	}
	vol := filepath.VolumeName(abs)
	rest := filepath.ToSlash(strings.TrimPrefix(abs, vol))
	return strings.TrimSuffix(vol, ":") + rest
}

// OSPath converts a stored name back into an absolute Windows path.
func OSPath(stored string) string {
	if strings.HasPrefix(stored, "UNC/") {
		return `\\` + filepath.FromSlash(stored[4:])
	}
	i := strings.Index(stored, "/")
	if i < 0 {
		return stored + `:\`
	}
	return stored[:i] + ":" + filepath.FromSlash(stored[i:])
}

func lchown(p string, uid, gid int) error { return nil }

func isRoot() bool { return false }
