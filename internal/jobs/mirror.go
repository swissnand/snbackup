package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/swissnand/snbackup/internal/repo"
)

// mirror keeps a 1:1 copy of the sources in a target folder (NAS, USB disk).
// Changed files are detected by size and modification time. With
// MirrorDelete, files removed at the source are removed at the target too –
// or moved into .snbackup-trash/<timestamp> when MirrorTrash is enabled.
func (rc *runCtx) mirror(ctx context.Context) (*outcome, error) {
	j := rc.job
	if j.TargetPath == "" {
		return nil, errors.New("Spiegelung braucht einen Zielordner")
	}
	ex, err := repo.NewMatcher(j.Source.Excludes)
	if err != nil {
		return nil, err
	}
	target, err := filepath.Abs(j.TargetPath)
	if err != nil {
		return nil, err
	}
	trash := filepath.Join(target, ".snbackup-trash", time.Now().Format("2006-01-02_150405"))
	st := &repo.Stats{}
	var prog repo.Progress
	rc.phase("Spiegeln nach " + target)
	for _, src := range j.Source.Paths {
		root, err := filepath.Abs(src)
		if err != nil {
			return nil, err
		}
		dstRoot := filepath.Join(target, filepath.FromSlash(repo.StoreName(root)))
		seen := map[string]bool{}
		err = filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if werr != nil {
				rc.logf2("warn", "Übersprungen: %v", werr)
				return nil
			}
			if p != root && ex.Match(repo.StoreName(p)) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			dst := filepath.Join(dstRoot, rel)
			seen[dst] = true
			fi, err := d.Info()
			if err != nil {
				return nil
			}
			if d.IsDir() {
				st.Dirs++
				return os.MkdirAll(dst, 0o755)
			}
			if !fi.Mode().IsRegular() {
				return nil
			}
			st.Files++
			st.Bytes += fi.Size()
			prog.Files++
			prog.Bytes += fi.Size()
			prog.Current = p
			rc.progress(prog)
			if dfi, err := os.Stat(dst); err == nil && dfi.Size() == fi.Size() && dfi.ModTime().Equal(fi.ModTime()) {
				st.Unchanged++
				return nil
			}
			if err := copyFile(p, dst, fi); err != nil {
				st.Errors++
				rc.logf2("warn", "Kopieren fehlgeschlagen: %s: %v", p, err)
				return nil
			}
			st.NewBytes += fi.Size()
			st.ChangedFile++
			return nil
		})
		if err != nil {
			return nil, err
		}
		if j.Options.MirrorDelete {
			var extra []string
			_ = filepath.WalkDir(dstRoot, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if !seen[p] && p != dstRoot {
					extra = append(extra, p)
					if d.IsDir() {
						return fs.SkipDir
					}
				}
				return nil
			})
			for _, p := range extra {
				if j.Options.MirrorTrash {
					rel, _ := filepath.Rel(target, p)
					t := filepath.Join(trash, rel)
					_ = os.MkdirAll(filepath.Dir(t), 0o700)
					if err := os.Rename(p, t); err != nil {
						rc.logf2("warn", "Papierkorb: %v", err)
					}
				} else if err := os.RemoveAll(p); err != nil {
					rc.logf2("warn", "Löschen: %v", err)
				}
			}
			if len(extra) > 0 {
				rc.logf2("info", "%d Einträge am Ziel entfernt (an der Quelle gelöscht)", len(extra))
			}
		}
	}
	return &outcome{stats: st, message: fmt.Sprintf("%s · %d kopiert · %s", files(st.Files), st.ChangedFile, human(st.NewBytes))}, nil
}

func copyFile(src, dst string, fi fs.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".snbackup-tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Chtimes(dst, fi.ModTime(), fi.ModTime())
}
