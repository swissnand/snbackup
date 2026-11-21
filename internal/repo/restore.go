package repo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RestoreOptions control a restore.
type RestoreOptions struct {
	// Target directory. Empty = restore to the original location.
	Target string
	// Include restricts the restore to these stored paths (and below).
	Include []string
	// Overwrite policy: always | never | newer | rename
	Overwrite    string
	Verify       bool // verify SHA-256 of each restored file
	RestoreOwner bool // restore uid/gid (Linux, needs root)
	DryRun       bool
	Progress     func(Progress)
	Log          func(level, msg string)
}

// RestoreResult summarises a restore.
type RestoreResult struct {
	Files   int64 `json:"files"`
	Dirs    int64 `json:"dirs"`
	Bytes   int64 `json:"bytes"`
	Skipped int64 `json:"skipped"`
	Errors  int64 `json:"errors"`
}

func included(p string, inc []string) bool {
	if len(inc) == 0 {
		return true
	}
	for _, i := range inc {
		i = strings.Trim(i, "/")
		if i == "" || p == i || strings.HasPrefix(p, i+"/") || strings.HasPrefix(i, p+"/") {
			return true
		}
	}
	return false
}

func within(base, p string) bool {
	rel, err := filepath.Rel(base, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Restore writes the content of a snapshot to disk.
func (r *Repo) Restore(ctx context.Context, s *Snapshot, opt RestoreOptions) (*RestoreResult, error) {
	logf := func(level, f string, a ...any) {
		if opt.Log != nil {
			opt.Log(level, fmt.Sprintf(f, a...))
		}
	}
	if opt.Overwrite == "" {
		opt.Overwrite = "always"
	}
	var target string
	if opt.Target != "" {
		t, err := filepath.Abs(opt.Target)
		if err != nil {
			return nil, err
		}
		target = t
		if !opt.DryRun {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return nil, err
			}
		}
	}
	nodes, err := r.LoadTree(ctx, s)
	if err != nil {
		return nil, err
	}
	var prog Progress
	for _, n := range nodes {
		if n.Type == "f" && included(n.Path, opt.Include) {
			prog.TotalFiles++
			prog.TotalBytes += n.Size
		}
	}
	res := &RestoreResult{}
	type dirMeta struct {
		path string
		n    *Node
	}
	var dirs []dirMeta
	last := time.Now()
	for _, n := range nodes {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if !included(n.Path, opt.Include) {
			continue
		}
		if !safeRel(n.Path) {
			res.Errors++
			logf("error", "Unsicherer Pfad im Snapshot abgelehnt: %q", n.Path)
			continue
		}
		var dst string
		if target != "" {
			dst = filepath.Join(target, filepath.FromSlash(n.Path))
			if !within(target, dst) {
				res.Errors++
				logf("error", "Pfad ausserhalb des Ziels abgelehnt: %q", n.Path)
				continue
			}
		} else {
			dst = OSPath(n.Path)
		}
		if opt.DryRun {
			if n.Type == "f" {
				res.Files++
				res.Bytes += n.Size
			}
			continue
		}
		switch n.Type {
		case "d":
			if err := os.MkdirAll(dst, 0o700); err != nil {
				res.Errors++
				logf("error", "Verzeichnis %s: %v", dst, err)
				continue
			}
			res.Dirs++
			dirs = append(dirs, dirMeta{dst, n})
		case "l":
			_ = os.MkdirAll(filepath.Dir(dst), 0o700)
			if _, err := os.Lstat(dst); err == nil {
				if opt.Overwrite == "never" {
					res.Skipped++
					continue
				}
				_ = os.Remove(dst)
			}
			if err := os.Symlink(n.Link, dst); err != nil {
				res.Errors++
				logf("warn", "Symlink %s: %v", dst, err)
			}
			if opt.RestoreOwner && isRoot() {
				_ = lchown(dst, n.UID, n.GID)
			}
		case "f":
			prog.Current = dst
			if fi, err := os.Stat(dst); err == nil {
				switch opt.Overwrite {
				case "never":
					res.Skipped++
					continue
				case "newer":
					if fi.ModTime().UnixNano() >= n.MTime {
						res.Skipped++
						continue
					}
				case "rename":
					dst = uniqueName(dst)
				}
			}
			if err := r.restoreFile(ctx, n, dst, opt.Verify); err != nil {
				if ctx.Err() != nil {
					return res, ctx.Err()
				}
				res.Errors++
				logf("error", "Datei %s: %v", dst, err)
				continue
			}
			if opt.RestoreOwner && isRoot() {
				_ = lchown(dst, n.UID, n.GID)
			}
			res.Files++
			res.Bytes += n.Size
			prog.Files++
			prog.Bytes += n.Size
			if opt.Progress != nil && time.Since(last) > 500*time.Millisecond {
				last = time.Now()
				opt.Progress(prog)
			}
		}
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		d := dirs[i]
		_ = os.Chmod(d.path, os.FileMode(d.n.Mode))
		mt := time.Unix(0, d.n.MTime)
		_ = os.Chtimes(d.path, mt, mt)
		if opt.RestoreOwner && isRoot() {
			_ = lchown(d.path, d.n.UID, d.n.GID)
		}
	}
	if opt.Progress != nil {
		prog.Current = ""
		opt.Progress(prog)
	}
	return res, nil
}

func uniqueName(p string) string {
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 1; ; i++ {
		c := fmt.Sprintf("%s.restored-%d%s", base, i, ext)
		if _, err := os.Stat(c); errors.Is(err, os.ErrNotExist) {
			return c
		}
	}
}

func (r *Repo) restoreFile(ctx context.Context, n *Node, dst string, verify bool) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	tmp := dst + ".snbackup-tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	h := sha256.New()
	w := io.MultiWriter(f, h)
	for _, c := range n.Chunks {
		data, err := r.LoadBlob(ctx, c)
		if err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
		if _, err := w.Write(data); err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	if verify && n.Hash != "" && hex.EncodeToString(h.Sum(nil)) != n.Hash {
		os.Remove(tmp)
		return errors.New("verification failed: checksum mismatch")
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	_ = os.Chmod(dst, os.FileMode(n.Mode))
	mt := time.Unix(0, n.MTime)
	_ = os.Chtimes(dst, mt, mt)
	return nil
}

// WriteFile streams the content of a single file node to w.
func (r *Repo) WriteFile(ctx context.Context, n *Node, w io.Writer) error {
	if n.Type != "f" {
		return errors.New("not a regular file")
	}
	for _, c := range n.Chunks {
		data, err := r.LoadBlob(ctx, c)
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	return nil
}
