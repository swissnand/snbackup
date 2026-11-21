// Package backend abstracts the storage targets a repository can live on:
// local disks / NAS mounts, S3-compatible object storage, WebDAV and rclone.
package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// ErrNotFound is returned when an object does not exist.
var ErrNotFound = errors.New("object not found")

// FileInfo describes a stored object.
type FileInfo struct {
	Name string
	Size int64
}

// Backend is the minimal object-store contract used by the repository layer.
// Names are slash separated, relative to the repository root
// (e.g. "config", "keys/<id>", "data/ab/<id>", "snapshots/<id>").
type Backend interface {
	Save(ctx context.Context, name string, data []byte) error
	Load(ctx context.Context, name string) ([]byte, error)
	Exists(ctx context.Context, name string) (bool, error)
	Delete(ctx context.Context, name string) error
	// List returns all objects below the given directory prefix (recursive).
	List(ctx context.Context, dir string) ([]FileInfo, error)
	Location() string
	Close() error
}

// Config describes how to reach a backend. Only the fields of the selected
// Kind are used.
type Config struct {
	Kind string `json:"kind"` // local | s3 | webdav | rclone

	Path string `json:"path,omitempty"` // local

	S3 S3Config `json:"s3,omitempty"`

	WebDAVURL      string `json:"webdavUrl,omitempty"`
	WebDAVUser     string `json:"webdavUser,omitempty"`
	WebDAVPassword string `json:"webdavPassword,omitempty"`

	RcloneRemote string `json:"rcloneRemote,omitempty"` // e.g. "sftp-nas:backups/server1"

	BandwidthKBs int `json:"bandwidthKBs,omitempty"` // upload limit, 0 = unlimited
}

// Open creates a backend from its configuration.
func Open(cfg Config) (Backend, error) {
	var b Backend
	var err error
	switch cfg.Kind {
	case "local", "":
		if cfg.Path == "" {
			return nil, errors.New("local backend: path is required")
		}
		b, err = NewLocal(cfg.Path)
	case "s3":
		b, err = NewS3(cfg.S3)
	case "webdav":
		b, err = NewWebDAV(cfg.WebDAVURL, cfg.WebDAVUser, cfg.WebDAVPassword)
	case "rclone":
		b, err = NewRclone(cfg.RcloneRemote)
	default:
		return nil, fmt.Errorf("unknown backend kind %q", cfg.Kind)
	}
	if err != nil {
		return nil, err
	}
	if cfg.BandwidthKBs > 0 {
		b = NewLimited(b, cfg.BandwidthKBs*1024)
	}
	return Retrying(b, 4), nil
}

// ---------------------------------------------------------------------------
// Bandwidth limiting (token bucket on Save).

type limited struct {
	Backend
	mu       sync.Mutex
	rate     float64 // bytes per second
	tokens   float64
	lastFill time.Time
}

// NewLimited wraps a backend with an upload bandwidth limit in bytes/s.
func NewLimited(b Backend, bytesPerSec int) Backend {
	return &limited{Backend: b, rate: float64(bytesPerSec), tokens: float64(bytesPerSec), lastFill: time.Now()}
}

func (l *limited) wait(ctx context.Context, n int) error {
	need := float64(n)
	for need > 0 {
		l.mu.Lock()
		now := time.Now()
		l.tokens += now.Sub(l.lastFill).Seconds() * l.rate
		if l.tokens > l.rate {
			l.tokens = l.rate
		}
		l.lastFill = now
		take := need
		if take > l.tokens {
			take = l.tokens
		}
		l.tokens -= take
		need -= take
		deficit := 0.0
		if need > 0 {
			deficit = need
			if deficit > l.rate {
				deficit = l.rate
			}
		}
		l.mu.Unlock()
		if deficit > 0 {
			d := time.Duration(deficit / l.rate * float64(time.Second))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
			}
		}
	}
	return nil
}

func (l *limited) Save(ctx context.Context, name string, data []byte) error {
	if err := l.wait(ctx, len(data)); err != nil {
		return err
	}
	return l.Backend.Save(ctx, name, data)
}

// ---------------------------------------------------------------------------
// Retry wrapper with exponential backoff for transient errors.

type retrying struct {
	Backend
	attempts int
}

// Retrying wraps a backend so that failing operations are retried.
func Retrying(b Backend, attempts int) Backend { return &retrying{Backend: b, attempts: attempts} }

func (r *retrying) do(ctx context.Context, fn func() error) error {
	var err error
	delay := 500 * time.Millisecond
	for i := 0; i < r.attempts; i++ {
		err = fn()
		if err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, context.Canceled) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
	return err
}

func (r *retrying) Save(ctx context.Context, name string, data []byte) error {
	return r.do(ctx, func() error { return r.Backend.Save(ctx, name, data) })
}
func (r *retrying) Load(ctx context.Context, name string) (out []byte, err error) {
	err = r.do(ctx, func() error { var e error; out, e = r.Backend.Load(ctx, name); return e })
	return
}
func (r *retrying) Exists(ctx context.Context, name string) (ok bool, err error) {
	err = r.do(ctx, func() error { var e error; ok, e = r.Backend.Exists(ctx, name); return e })
	return
}
func (r *retrying) Delete(ctx context.Context, name string) error {
	return r.do(ctx, func() error { return r.Backend.Delete(ctx, name) })
}
func (r *retrying) List(ctx context.Context, dir string) (out []FileInfo, err error) {
	err = r.do(ctx, func() error { var e error; out, e = r.Backend.List(ctx, dir); return e })
	return
}

// drain reads and closes a body, used to allow connection reuse.
func drain(rc io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(rc, 1<<20))
	_ = rc.Close()
}
