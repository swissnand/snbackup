//go:build !windows

package repo

import (
	"io/fs"
	"os"
	"strings"
	"syscall"
)

func statExtra(fi fs.FileInfo) (uid, gid int, inode, dev uint64) {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid), int(st.Gid), uint64(st.Ino), uint64(st.Dev)
	}
	return 0, 0, 0, 0
}

// StoreName converts an absolute OS path into the stored slash form.
func StoreName(abs string) string { return strings.TrimPrefix(abs, "/") }

// OSPath converts a stored name back into an absolute OS path.
func OSPath(stored string) string { return "/" + stored }

func lchown(p string, uid, gid int) error { return os.Lchown(p, uid, gid) }

func isRoot() bool { return os.Geteuid() == 0 }
