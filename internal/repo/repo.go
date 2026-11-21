// Package repo implements the encrypted, deduplicating SWISSNAND Backup
// repository format.
package repo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/swissnand/snbackup/internal/backend"
)

// Config is the plaintext repository descriptor stored as "config".
type Config struct {
	Version int       `json:"version"`
	ID      string    `json:"id"`
	Created time.Time `json:"created"`
	Chunker string    `json:"chunker"`
	Cipher  string    `json:"cipher"`
}

type keyFile struct {
	Created    time.Time `json:"created"`
	Host       string    `json:"host"`
	Label      string    `json:"label,omitempty"`
	KDF        string    `json:"kdf"`
	Iterations int       `json:"iterations"`
	Salt       []byte    `json:"salt"`
	Data       []byte    `json:"data"`
}

// Repo is an opened repository.
type Repo struct {
	be          backend.Backend
	cfg         Config
	key         *MasterKey
	gear        *gearTable
	Compression string

	mu        sync.Mutex
	known     map[string]struct{}
	knownSize map[string]int64
	loaded    bool
}

func randomID(n int) string {
	b := make([]byte, n)
	_, _ = io.ReadFull(rand.Reader, b)
	return hex.EncodeToString(b)
}

// Init creates a new repository on the backend.
func Init(ctx context.Context, be backend.Backend, password string) (*Repo, error) {
	if len(password) < 8 {
		return nil, errors.New("repository password must have at least 8 characters")
	}
	if ok, err := be.Exists(ctx, "config"); err != nil {
		return nil, err
	} else if ok {
		return nil, errors.New("a repository already exists at this location")
	}
	key, err := newMasterKey()
	if err != nil {
		return nil, err
	}
	cfg := Config{Version: formatVersion, ID: randomID(16), Created: time.Now().UTC(), Chunker: "gear-cdc-1M", Cipher: "AES-256-GCM/HMAC-SHA256/PBKDF2-SHA256"}
	r := &Repo{be: be, cfg: cfg, key: key, gear: newGear(key.Gear), Compression: CompressionAuto}
	if err := r.addKey(ctx, password, "initial"); err != nil {
		return nil, err
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	if err := be.Save(ctx, "config", b); err != nil {
		return nil, err
	}
	return r, nil
}

// Open unlocks an existing repository.
func Open(ctx context.Context, be backend.Backend, password string) (*Repo, error) {
	raw, err := be.Load(ctx, "config")
	if errors.Is(err, backend.ErrNotFound) {
		return nil, errors.New("no repository found at this location (not initialised?)")
	}
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("invalid repository config: %w", err)
	}
	if cfg.Version != formatVersion {
		return nil, fmt.Errorf("unsupported repository version %d", cfg.Version)
	}
	keys, err := be.List(ctx, "keys")
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		raw, err := be.Load(ctx, k.Name)
		if err != nil {
			continue
		}
		var kf keyFile
		if json.Unmarshal(raw, &kf) != nil || kf.Iterations < 10000 {
			continue
		}
		kek, err := deriveKEK(password, kf.Salt, kf.Iterations)
		if err != nil {
			continue
		}
		pt, err := OpenSealed(kek, kf.Data, []byte("key:"+cfg.ID))
		if err != nil {
			continue
		}
		var mk MasterKey
		if err := json.Unmarshal(pt, &mk); err != nil {
			continue
		}
		return &Repo{be: be, cfg: cfg, key: &mk, gear: newGear(mk.Gear), Compression: CompressionAuto}, nil
	}
	return nil, ErrWrongPassword
}

func (r *Repo) addKey(ctx context.Context, password, label string) error {
	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}
	kek, err := deriveKEK(password, salt, defaultKDFIters)
	if err != nil {
		return err
	}
	pt, _ := json.Marshal(r.key)
	ct, err := Seal(kek, pt, []byte("key:"+r.cfg.ID))
	if err != nil {
		return err
	}
	host, _ := os.Hostname()
	kf := keyFile{Created: time.Now().UTC(), Host: host, Label: label, KDF: "pbkdf2-sha256", Iterations: defaultKDFIters, Salt: salt, Data: ct}
	b, _ := json.MarshalIndent(kf, "", "  ")
	return r.be.Save(ctx, "keys/"+randomID(16), b)
}

// AddPassword adds another password that unlocks this repository.
func (r *Repo) AddPassword(ctx context.Context, password, label string) error {
	if len(password) < 8 {
		return errors.New("password must have at least 8 characters")
	}
	return r.addKey(ctx, password, label)
}

// ChangePassword replaces all key files with one for the new password.
func (r *Repo) ChangePassword(ctx context.Context, newPassword string) error {
	old, err := r.be.List(ctx, "keys")
	if err != nil {
		return err
	}
	if err := r.AddPassword(ctx, newPassword, "changed "+time.Now().Format("2006-01-02")); err != nil {
		return err
	}
	for _, k := range old {
		if err := r.be.Delete(ctx, k.Name); err != nil {
			return err
		}
	}
	return nil
}

// WithBackend returns a handle sharing the keys but using another backend
// (e.g. a bandwidth-limited wrapper for one job).
func (r *Repo) WithBackend(be backend.Backend) *Repo {
	return &Repo{be: be, cfg: r.cfg, key: r.key, gear: r.gear, Compression: r.Compression}
}

// ID returns the repository ID.
func (r *Repo) ID() string { return r.cfg.ID }

// Config returns the repository descriptor.
func (r *Repo) Config() Config { return r.cfg }

// Backend returns the underlying backend.
func (r *Repo) Backend() backend.Backend { return r.be }

func blobName(id string) string { return "data/" + id[:2] + "/" + id }

// ResetIndex forces the chunk index to be re-read on next use (another
// process may have added or pruned data in the meantime).
func (r *Repo) ResetIndex() {
	r.mu.Lock()
	r.loaded = false
	r.mu.Unlock()
}

// LoadIndex lists all stored chunks so new data can be deduplicated.
func (r *Repo) LoadIndex(ctx context.Context) error {
	r.mu.Lock()
	if r.loaded {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()
	files, err := r.be.List(ctx, "data")
	if err != nil {
		return err
	}
	known := make(map[string]struct{}, len(files))
	sizes := make(map[string]int64, len(files))
	for _, f := range files {
		i := strings.LastIndex(f.Name, "/")
		id := f.Name[i+1:]
		if len(id) == 64 {
			known[id] = struct{}{}
			sizes[id] = f.Size
		}
	}
	r.mu.Lock()
	r.known, r.knownSize, r.loaded = known, sizes, true
	r.mu.Unlock()
	return nil
}

// HasBlob reports whether the chunk is already stored.
func (r *Repo) HasBlob(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.known[id]
	return ok
}

// SaveBlob stores a chunk if it is not yet present. It returns the chunk ID
// and the number of bytes written to the backend (0 when deduplicated).
func (r *Repo) SaveBlob(ctx context.Context, data []byte) (string, int64, error) {
	id := r.key.chunkID(data)
	r.mu.Lock()
	if r.known == nil {
		r.known, r.knownSize = map[string]struct{}{}, map[string]int64{}
	}
	if _, ok := r.known[id]; ok {
		r.mu.Unlock()
		return id, 0, nil
	}
	r.known[id] = struct{}{} // reserve to avoid duplicate concurrent uploads
	r.mu.Unlock()

	ct, err := Seal(r.key.Enc, compress(data, r.Compression), []byte(id))
	if err == nil {
		err = r.be.Save(ctx, blobName(id), ct)
	}
	r.mu.Lock()
	if err != nil {
		delete(r.known, id)
	} else {
		r.knownSize[id] = int64(len(ct))
	}
	r.mu.Unlock()
	if err != nil {
		return "", 0, err
	}
	return id, int64(len(ct)), nil
}

// LoadBlob loads, decrypts, decompresses and verifies a chunk.
func (r *Repo) LoadBlob(ctx context.Context, id string) ([]byte, error) {
	if len(id) != 64 {
		return nil, errors.New("invalid chunk id")
	}
	ct, err := r.be.Load(ctx, blobName(id))
	if err != nil {
		return nil, fmt.Errorf("chunk %s: %w", id[:12], err)
	}
	pt, err := OpenSealed(r.key.Enc, ct, []byte(id))
	if err != nil {
		return nil, fmt.Errorf("chunk %s: %w", id[:12], err)
	}
	data, err := decompress(pt)
	if err != nil {
		return nil, err
	}
	if r.key.chunkID(data) != id {
		return nil, fmt.Errorf("chunk %s: content hash mismatch", id[:12])
	}
	return data, nil
}

// ---------------------------------------------------------------------------
// Snapshots

// Stats summarises a backup run.
type Stats struct {
	Files       int64 `json:"files"`
	Dirs        int64 `json:"dirs"`
	Links       int64 `json:"links"`
	Bytes       int64 `json:"bytes"`
	NewFiles    int64 `json:"newFiles"`
	ChangedFile int64 `json:"changedFiles"`
	Unchanged   int64 `json:"unchangedFiles"`
	NewBytes    int64 `json:"newBytes"`
	StoredBytes int64 `json:"storedBytes"`
	NewChunks   int64 `json:"newChunks"`
	Errors      int64 `json:"errors"`
	DurationMs  int64 `json:"durationMs"`
}

// Snapshot is a point-in-time backup.
type Snapshot struct {
	ID      string    `json:"id"`
	Time    time.Time `json:"time"`
	Host    string    `json:"host"`
	OS      string    `json:"os"`
	JobID   string    `json:"jobId,omitempty"`
	JobName string    `json:"jobName,omitempty"`
	Kind    string    `json:"kind"`
	Mode    string    `json:"mode,omitempty"`
	Paths   []string  `json:"paths"`
	Tags    []string  `json:"tags,omitempty"`
	Tree    []string  `json:"tree"`
	Parent  string    `json:"parent,omitempty"`
	Origin  string    `json:"origin,omitempty"`
	Stats   Stats     `json:"stats"`
}

// SaveSnapshot writes an encrypted snapshot object.
func (r *Repo) SaveSnapshot(ctx context.Context, s *Snapshot) error {
	if s.ID == "" {
		s.ID = randomID(16)
	}
	b, _ := json.Marshal(s)
	ct, err := Seal(r.key.Enc, b, []byte("snapshot:"+s.ID))
	if err != nil {
		return err
	}
	return r.be.Save(ctx, "snapshots/"+s.ID, ct)
}

// LoadSnapshot loads a snapshot by ID (a unique prefix is accepted).
func (r *Repo) LoadSnapshot(ctx context.Context, id string) (*Snapshot, error) {
	if id == "latest" {
		all, err := r.Snapshots(ctx)
		if err != nil {
			return nil, err
		}
		if len(all) == 0 {
			return nil, errors.New("no snapshots")
		}
		return all[0], nil
	}
	if len(id) < 32 {
		all, err := r.Snapshots(ctx)
		if err != nil {
			return nil, err
		}
		var match *Snapshot
		for _, s := range all {
			if strings.HasPrefix(s.ID, id) {
				if match != nil {
					return nil, errors.New("snapshot id prefix is ambiguous")
				}
				match = s
			}
		}
		if match == nil {
			return nil, errors.New("snapshot not found")
		}
		return match, nil
	}
	if strings.ContainsAny(id, "/\\.") {
		return nil, errors.New("invalid snapshot id")
	}
	ct, err := r.be.Load(ctx, "snapshots/"+id)
	if err != nil {
		return nil, err
	}
	pt, err := OpenSealed(r.key.Enc, ct, []byte("snapshot:"+id))
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(pt, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Snapshots returns all snapshots sorted by time (newest first).
func (r *Repo) Snapshots(ctx context.Context) ([]*Snapshot, error) {
	files, err := r.be.List(ctx, "snapshots")
	if err != nil {
		return nil, err
	}
	var out []*Snapshot
	for _, f := range files {
		id := f.Name[strings.LastIndex(f.Name, "/")+1:]
		s, err := r.LoadSnapshot(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out, nil
}

// DeleteSnapshot removes a snapshot (data is freed by Prune).
func (r *Repo) DeleteSnapshot(ctx context.Context, id string) error {
	if len(id) != 32 || strings.ContainsAny(id, "/\\.") {
		return errors.New("invalid snapshot id")
	}
	return r.be.Delete(ctx, "snapshots/"+id)
}

// Usage returns the total stored size and number of chunks.
func (r *Repo) Usage(ctx context.Context) (bytes int64, chunks int, err error) {
	files, err := r.be.List(ctx, "data")
	if err != nil {
		return 0, 0, err
	}
	for _, f := range files {
		bytes += f.Size
	}
	return bytes, len(files), nil
}
