package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Rclone delegates storage to an installed rclone binary. This opens up
// 70+ providers: SFTP, FTP, SMB, Google Drive, OneDrive, Dropbox, Azure Blob,
// Google Cloud Storage, Swift, pCloud, Proton Drive ...
type Rclone struct {
	remote string // "name:path"
	bin    string
}

// NewRclone creates an rclone backend for a configured remote ("remote:path").
func NewRclone(remote string) (*Rclone, error) {
	if !strings.Contains(remote, ":") {
		return nil, errors.New("rclone: remote must look like name:path")
	}
	bin, err := exec.LookPath("rclone")
	if err != nil {
		return nil, errors.New("rclone: binary not found in PATH (install from https://rclone.org)")
	}
	return &Rclone{remote: strings.TrimRight(remote, "/"), bin: bin}, nil
}

func (r *Rclone) p(name string) string { return r.remote + "/" + strings.TrimPrefix(name, "/") }

func (r *Rclone) run(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.bin, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if strings.Contains(msg, "not found") || strings.Contains(msg, "doesn't exist") {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("rclone %s: %v: %s", args[0], err, msg)
	}
	return out.Bytes(), nil
}

func (r *Rclone) Save(ctx context.Context, name string, data []byte) error {
	_, err := r.run(ctx, data, "rcat", r.p(name))
	return err
}
func (r *Rclone) Load(ctx context.Context, name string) ([]byte, error) {
	return r.run(ctx, nil, "cat", r.p(name))
}
func (r *Rclone) Exists(ctx context.Context, name string) (bool, error) {
	out, err := r.run(ctx, nil, "lsf", r.p(name))
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return len(bytes.TrimSpace(out)) > 0, nil
}
func (r *Rclone) Delete(ctx context.Context, name string) error {
	_, err := r.run(ctx, nil, "deletefile", r.p(name))
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
func (r *Rclone) List(ctx context.Context, dir string) ([]FileInfo, error) {
	out, err := r.run(ctx, nil, "lsjson", "-R", "--files-only", "--no-mimetype", "--no-modtime", r.p(dir))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var items []struct {
		Path string `json:"Path"`
		Size int64  `json:"Size"`
	}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, err
	}
	res := make([]FileInfo, 0, len(items))
	d := strings.Trim(dir, "/")
	for _, it := range items {
		res = append(res, FileInfo{Name: d + "/" + it.Path, Size: it.Size})
	}
	return res, nil
}
func (r *Rclone) Location() string { return "rclone:" + r.remote }
func (r *Rclone) Close() error     { return nil }
