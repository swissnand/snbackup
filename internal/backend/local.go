package backend

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Local stores objects on a local filesystem (internal disk, USB disk,
// mounted NAS share via NFS/SMB, iSCSI volume ...).
type Local struct{ root string }

// NewLocal returns a local backend rooted at path.
func NewLocal(path string) (*Local, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, err
	}
	return &Local{root: abs}, nil
}

func (l *Local) full(name string) (string, error) {
	clean := filepath.FromSlash(strings.TrimPrefix(name, "/"))
	if strings.Contains(name, "..") {
		return "", errors.New("invalid object name")
	}
	return filepath.Join(l.root, clean), nil
}

// Save writes atomically: temp file, fsync, rename.
func (l *Local) Save(_ context.Context, name string, data []byte) error {
	p, err := l.full(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	_ = os.Chmod(tmpName, 0o600)
	return os.Rename(tmpName, p)
}

func (l *Local) Load(_ context.Context, name string) ([]byte, error) {
	p, err := l.full(name)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return b, err
}

func (l *Local) Exists(_ context.Context, name string) (bool, error) {
	p, err := l.full(name)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (l *Local) Delete(_ context.Context, name string) error {
	p, err := l.full(name)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (l *Local) List(_ context.Context, dir string) ([]FileInfo, error) {
	p, err := l.full(dir)
	if err != nil {
		return nil, err
	}
	var out []FileInfo
	err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".tmp-") {
			return nil
		}
		rel, _ := filepath.Rel(l.root, path)
		info, ierr := d.Info()
		var size int64
		if ierr == nil {
			size = info.Size()
		}
		out = append(out, FileInfo{Name: filepath.ToSlash(rel), Size: size})
		return nil
	})
	return out, err
}

func (l *Local) Location() string { return l.root }
func (l *Local) Close() error     { return nil }
