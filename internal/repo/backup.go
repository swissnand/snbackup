package repo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Stream is a virtual file produced by a command (database dump, docker
// export, any program writing to stdout).
type Stream struct {
	Name string // stored path, e.g. "_streams/postgres/shop.sql"
	Open func(ctx context.Context) (io.ReadCloser, func() error, error)
}

// Progress reports backup progress.
type Progress struct {
	Files       int64  `json:"files"`
	Bytes       int64  `json:"bytes"`
	TotalFiles  int64  `json:"totalFiles"`
	TotalBytes  int64  `json:"totalBytes"`
	NewBytes    int64  `json:"newBytes"`
	StoredBytes int64  `json:"storedBytes"`
	Current     string `json:"current"`
	Errors      int64  `json:"errors"`
}

// BackupOptions control a backup run.
type BackupOptions struct {
	Paths             []string
	Excludes          []string
	ExcludeLargerThan int64
	ExcludeCaches     bool
	OneFileSystem     bool
	Full              bool // ignore parent: re-read every file ("active full")
	// Differential compares against the last FULL snapshot of the job
	// instead of the last snapshot (classic differential backup).
	Differential  bool
	Workers       int
	Host, JobID   string
	JobName, Kind string
	Tags          []string
	Streams       []Stream
	// ReadPath maps an original absolute path to the path that is actually
	// read (used for VSS shadow copies / LVM snapshots). nil = identity.
	ReadPath func(string) string
	Progress func(Progress)
	Log      func(level, msg string)
}

// ---------------------------------------------------------------------------
// Exclude patterns

type matcher struct{ res []*regexp.Regexp }

// Matcher matches stored paths against exclude patterns.
type Matcher interface{ Match(stored string) bool }

// NewMatcher compiles exclude patterns (see newMatcher).
func NewMatcher(patterns []string) (Matcher, error) { return newMatcher(patterns) }

// Match implements Matcher.
func (m *matcher) Match(stored string) bool { return m.match(stored) }

func globToRegex(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch c {
		case '*':
			if i+1 < len(p) && p[i+1] == '*' {
				b.WriteString(".*")
				i++
				if i+1 < len(p) && p[i+1] == '/' {
					i++
					b.WriteString("/?")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}

// newMatcher compiles exclude patterns. Patterns without a slash match any
// path component ("*.tmp", "node_modules"); patterns with a slash match the
// full path ("/var/log/**", "**/cache/*"). Matching is case-insensitive on
// Windows.
func newMatcher(patterns []string) (*matcher, error) {
	m := &matcher{}
	for _, p := range patterns {
		p = strings.TrimSpace(filepath.ToSlash(p))
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		var expr string
		if !strings.Contains(strings.TrimSuffix(p, "/"), "/") {
			expr = `(^|/)` + globToRegex(strings.TrimSuffix(p, "/")) + `$`
		} else {
			p = strings.TrimPrefix(p, "/")
			if len(p) > 1 && p[1] == ':' { // C:/...
				p = p[:1] + p[2:]
			}
			expr = `^` + globToRegex(strings.TrimSuffix(p, "/")) + `(/.*)?$`
		}
		if runtime.GOOS == "windows" {
			expr = `(?i)` + expr
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("invalid exclude pattern %q: %w", p, err)
		}
		m.res = append(m.res, re)
	}
	return m, nil
}

func (m *matcher) match(stored string) bool {
	for _, re := range m.res {
		if re.MatchString(stored) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------

type backupRun struct {
	r      *Repo
	ctx    context.Context
	opt    BackupOptions
	ex     *matcher
	parent map[string]*Node
	stats  Stats
	prog   Progress
	tw     *treeWriter
	sem    chan struct{}
	wg     sync.WaitGroup
	errMu  sync.Mutex
	err    error
	lastP  time.Time
}

func (b *backupRun) log(level, format string, a ...any) {
	if b.opt.Log != nil {
		b.opt.Log(level, fmt.Sprintf(format, a...))
	}
}

func (b *backupRun) progress(force bool) {
	if b.opt.Progress == nil {
		return
	}
	if !force && time.Since(b.lastP) < 500*time.Millisecond {
		return
	}
	b.lastP = time.Now()
	p := b.prog
	p.NewBytes = atomic.LoadInt64(&b.stats.NewBytes)
	p.StoredBytes = atomic.LoadInt64(&b.stats.StoredBytes)
	b.opt.Progress(p)
}

func (b *backupRun) fail(err error) {
	b.errMu.Lock()
	if b.err == nil {
		b.err = err
	}
	b.errMu.Unlock()
}

func (b *backupRun) getErr() error {
	b.errMu.Lock()
	defer b.errMu.Unlock()
	return b.err
}

// Backup creates a new snapshot.
func (r *Repo) Backup(ctx context.Context, opt BackupOptions) (*Snapshot, error) {
	start := time.Now()
	if opt.Workers <= 0 {
		opt.Workers = 4
	}
	if opt.Host == "" {
		opt.Host, _ = os.Hostname()
	}
	if opt.Kind == "" {
		opt.Kind = "files"
	}
	ex, err := newMatcher(opt.Excludes)
	if err != nil {
		return nil, err
	}
	r.ResetIndex()
	if err := r.LoadIndex(ctx); err != nil {
		return nil, fmt.Errorf("loading index: %w", err)
	}
	b := &backupRun{r: r, ctx: ctx, opt: opt, ex: ex, sem: make(chan struct{}, opt.Workers)}
	b.tw = &treeWriter{r: r, ctx: ctx}

	// Parent snapshot for change detection (forever incremental).
	var parentID string
	if !opt.Full {
		if snaps, err := r.Snapshots(ctx); err == nil {
			for _, s := range snaps {
				if s.Host == opt.Host && s.JobID == opt.JobID && s.Kind == opt.Kind && s.Origin == "" &&
					(!opt.Differential || s.Mode == "full" || s.Mode == "") {
					if nodes, err := r.LoadTree(ctx, s); err == nil {
						b.parent = make(map[string]*Node, len(nodes))
						for _, n := range nodes {
							if n.Type == "f" {
								b.parent[n.Path] = n
							}
						}
						parentID = s.ID
						if opt.Differential {
							b.log("info", "Differenziell: Basis ist das letzte Vollbackup %s (%s)", s.ID[:8], s.Time.Local().Format("02.01.2006 15:04"))
						} else {
							b.log("info", "Inkrementell: Basis-Snapshot %s (%s) – nur Änderungen seit dem letzten Lauf werden gelesen", s.ID[:8], s.Time.Local().Format("02.01.2006 15:04"))
						}
					}
					break
				}
			}
		}
	} else {
		b.log("info", "Aktives Vollbackup: alle Dateien werden neu gelesen")
	}

	var roots []string
	for _, p := range opt.Paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		roots = append(roots, abs)
	}
	sort.Strings(roots)

	if opt.Progress != nil {
		for _, root := range roots {
			b.prescan(root)
		}
		b.progress(true)
	}

	// Parent directories of each root are recorded so restores recreate them.
	written := map[string]bool{}
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		stored := StoreName(root)
		parts := strings.Split(stored, "/")
		for i := 1; i < len(parts); i++ {
			dir := strings.Join(parts[:i], "/")
			if written[dir] || dir == "" {
				continue
			}
			written[dir] = true
			if fi, err := os.Stat(OSPath(dir)); err == nil {
				b.addMeta(dir, fi)
			}
		}
		if err := b.walk(root); err != nil {
			return nil, err
		}
	}
	for _, s := range opt.Streams {
		if err := b.stream(s); err != nil {
			return nil, err
		}
	}
	b.wg.Wait()
	if e := b.getErr(); e != nil {
		return nil, e
	}
	ids, err := b.tw.Close()
	if err != nil {
		return nil, err
	}
	b.stats.StoredBytes += b.tw.stored
	b.stats.DurationMs = time.Since(start).Milliseconds()
	mode := "incremental"
	if opt.Differential {
		mode = "differential"
	}
	if opt.Full || parentID == "" {
		mode = "full"
	}
	snap := &Snapshot{
		Time: time.Now().UTC(), Host: opt.Host, OS: runtime.GOOS, JobID: opt.JobID, JobName: opt.JobName,
		Kind: opt.Kind, Mode: mode, Paths: opt.Paths, Tags: opt.Tags, Tree: ids, Parent: parentID, Stats: b.stats,
	}
	if err := r.SaveSnapshot(ctx, snap); err != nil {
		return nil, err
	}
	b.prog.Current = ""
	b.progress(true)
	return snap, nil
}

func (b *backupRun) readPath(p string) string {
	if b.opt.ReadPath != nil {
		return b.opt.ReadPath(p)
	}
	return p
}

func (b *backupRun) prescan(root string) {
	_ = filepath.WalkDir(b.readPath(root), func(p string, d fs.DirEntry, err error) error {
		if err != nil || b.ctx.Err() != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil {
				b.prog.TotalFiles++
				b.prog.TotalBytes += fi.Size()
			}
		}
		return nil
	})
}

func (b *backupRun) addMeta(stored string, fi fs.FileInfo) {
	uid, gid, _, _ := statExtra(fi)
	n := &Node{Path: stored, Type: "d", Mode: uint32(fi.Mode().Perm()), MTime: fi.ModTime().UnixNano(), UID: uid, GID: gid}
	if err := b.tw.Add(n); err != nil {
		b.fail(err)
	}
}

func (b *backupRun) walk(root string) error {
	readRoot := b.readPath(root)
	rootFI, err := os.Lstat(readRoot)
	if err != nil {
		b.stats.Errors++
		b.log("error", "Quelle nicht lesbar: %s: %v", root, err)
		return nil
	}
	_, _, _, rootDev := statExtra(rootFI)
	return filepath.WalkDir(readRoot, func(rp string, d fs.DirEntry, err error) error {
		if cerr := b.ctx.Err(); cerr != nil {
			return cerr
		}
		if e := b.getErr(); e != nil {
			return e
		}
		rel, _ := filepath.Rel(readRoot, rp)
		orig := root
		if rel != "." {
			orig = filepath.Join(root, rel)
		}
		stored := StoreName(orig)
		if err != nil {
			b.stats.Errors++
			b.prog.Errors++
			b.log("warn", "Übersprungen: %s: %v", orig, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if rp != readRoot && b.ex.match(stored) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			b.stats.Errors++
			b.log("warn", "Übersprungen: %s: %v", orig, err)
			return nil
		}
		uid, gid, ino, dev := statExtra(fi)
		switch {
		case d.IsDir():
			if b.opt.OneFileSystem && dev != rootDev && rp != readRoot {
				b.log("info", "Anderes Dateisystem übersprungen: %s", orig)
				return fs.SkipDir
			}
			if b.opt.ExcludeCaches && rp != readRoot {
				if _, err := os.Stat(filepath.Join(rp, "CACHEDIR.TAG")); err == nil {
					return fs.SkipDir
				}
			}
			b.stats.Dirs++
			return b.tw.Add(&Node{Path: stored, Type: "d", Mode: uint32(fi.Mode().Perm()), MTime: fi.ModTime().UnixNano(), UID: uid, GID: gid})
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(rp)
			if err != nil {
				b.stats.Errors++
				return nil
			}
			b.stats.Links++
			return b.tw.Add(&Node{Path: stored, Type: "l", Mode: uint32(fi.Mode().Perm()), MTime: fi.ModTime().UnixNano(), Link: target, UID: uid, GID: gid})
		case d.Type().IsRegular():
			if b.opt.ExcludeLargerThan > 0 && fi.Size() > b.opt.ExcludeLargerThan {
				b.log("info", "Zu gross, übersprungen: %s (%d MB)", orig, fi.Size()>>20)
				return nil
			}
			n := &Node{Path: stored, Type: "f", Mode: uint32(fi.Mode().Perm()), MTime: fi.ModTime().UnixNano(), Size: fi.Size(), UID: uid, GID: gid, Inode: ino}
			if err := b.file(rp, n); err != nil {
				if b.ctx.Err() != nil {
					return b.ctx.Err()
				}
				b.stats.Errors++
				b.prog.Errors++
				b.log("warn", "Datei nicht gesichert: %s: %v", orig, err)
				return nil
			}
			return b.tw.Add(n)
		default:
			return nil // sockets, devices, pipes
		}
	})
}

// file stores (or reuses) the content of a regular file.
func (b *backupRun) file(readPath string, n *Node) error {
	b.stats.Files++
	b.stats.Bytes += n.Size
	b.prog.Files++
	b.prog.Current = OSPath(n.Path)
	if old, ok := b.parent[n.Path]; ok && old.Size == n.Size && old.MTime == n.MTime && old.Inode == n.Inode {
		all := true
		for _, c := range old.Chunks {
			if !b.r.HasBlob(c) {
				all = false
				break
			}
		}
		if all {
			n.Chunks, n.Hash = old.Chunks, old.Hash
			b.stats.Unchanged++
			b.prog.Bytes += n.Size
			b.progress(false)
			return nil
		}
	}
	if _, ok := b.parent[n.Path]; ok {
		b.stats.ChangedFile++
	} else {
		b.stats.NewFiles++
	}
	f, err := os.Open(readPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return b.content(f, n)
}

func (b *backupRun) content(rd io.Reader, n *Node) error {
	h := sha256.New()
	ch := newChunker(io.TeeReader(rd, h), b.r.gear)
	var size int64
	for {
		data, err := ch.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		size += int64(len(data))
		b.prog.Bytes += int64(len(data))
		id := b.r.key.chunkID(data)
		n.Chunks = append(n.Chunks, id)
		if !b.r.HasBlob(id) {
			atomic.AddInt64(&b.stats.NewBytes, int64(len(data)))
			b.sem <- struct{}{}
			b.wg.Add(1)
			go func(d []byte) {
				defer func() { <-b.sem; b.wg.Done() }()
				_, stored, err := b.r.SaveBlob(b.ctx, d)
				if err != nil {
					b.fail(fmt.Errorf("upload failed: %w", err))
					return
				}
				if stored > 0 {
					atomic.AddInt64(&b.stats.NewChunks, 1)
					atomic.AddInt64(&b.stats.StoredBytes, stored)
				}
			}(data)
		}
		b.progress(false)
		if e := b.getErr(); e != nil {
			return e
		}
	}
	n.Size = size
	n.Hash = hex.EncodeToString(h.Sum(nil))
	return nil
}

func (b *backupRun) stream(s Stream) error {
	name := strings.Trim(filepath.ToSlash(s.Name), "/")
	if !safeRel(name) {
		return fmt.Errorf("invalid stream name %q", s.Name)
	}
	b.log("info", "Stream: %s", name)
	rc, wait, err := s.Open(b.ctx)
	if err != nil {
		return err
	}
	n := &Node{Path: name, Type: "f", Mode: 0o600, MTime: time.Now().UnixNano()}
	b.stats.Files++
	b.stats.NewFiles++
	b.prog.Current = name
	err = b.content(rc, n)
	rc.Close()
	if werr := wait(); werr != nil && err == nil {
		err = werr
	}
	if err != nil {
		return fmt.Errorf("stream %s: %w", name, err)
	}
	b.stats.Bytes += n.Size
	b.log("info", "Stream %s: %d MB", name, n.Size>>20)
	return b.tw.Add(n)
}

// ErrLocked is returned if another process holds a conflicting lock.
var ErrLocked = errors.New("repository is locked by another operation")
