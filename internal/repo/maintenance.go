package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Locking

type lockInfo struct {
	Exclusive bool      `json:"exclusive"`
	Host      string    `json:"host"`
	PID       int       `json:"pid"`
	Time      time.Time `json:"time"`
	Purpose   string    `json:"purpose"`
}

// Lock is a held repository lock.
type Lock struct {
	r    *Repo
	name string
	info lockInfo
	stop chan struct{}
}

const staleLock = 30 * time.Minute

// Lock acquires a shared (backup/restore) or exclusive (prune) lock.
func (r *Repo) Lock(ctx context.Context, exclusive bool, purpose string) (*Lock, error) {
	files, err := r.be.List(ctx, "locks")
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		raw, err := r.be.Load(ctx, f.Name)
		if err != nil {
			continue
		}
		var li lockInfo
		if json.Unmarshal(raw, &li) != nil || time.Since(li.Time) > staleLock {
			_ = r.be.Delete(ctx, f.Name)
			continue
		}
		if exclusive || li.Exclusive {
			return nil, fmt.Errorf("%w (%s auf %s seit %s)", ErrLocked, li.Purpose, li.Host, li.Time.Local().Format("15:04"))
		}
	}
	host, _ := os.Hostname()
	l := &Lock{r: r, name: "locks/" + randomID(16), info: lockInfo{Exclusive: exclusive, Host: host, PID: os.Getpid(), Time: time.Now().UTC(), Purpose: purpose}, stop: make(chan struct{})}
	if err := l.write(ctx); err != nil {
		return nil, err
	}
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-l.stop:
				return
			case <-t.C:
				l.info.Time = time.Now().UTC()
				_ = l.write(context.Background())
			}
		}
	}()
	return l, nil
}

func (l *Lock) write(ctx context.Context) error {
	b, _ := json.Marshal(l.info)
	return l.r.be.Save(ctx, l.name, b)
}

// Unlock releases the lock.
func (l *Lock) Unlock() {
	close(l.stop)
	_ = l.r.be.Delete(context.Background(), l.name)
}

// Unlock removes all locks (use after a crash).
func (r *Repo) RemoveAllLocks(ctx context.Context) (int, error) {
	files, err := r.be.List(ctx, "locks")
	if err != nil {
		return 0, err
	}
	for _, f := range files {
		if err := r.be.Delete(ctx, f.Name); err != nil {
			return 0, err
		}
	}
	return len(files), nil
}

// ---------------------------------------------------------------------------
// Retention (GFS – grandfather/father/son)

// Retention defines which snapshots to keep.
type Retention struct {
	KeepLast       int `json:"keepLast"`
	KeepHourly     int `json:"keepHourly"`
	KeepDaily      int `json:"keepDaily"`
	KeepWeekly     int `json:"keepWeekly"`
	KeepMonthly    int `json:"keepMonthly"`
	KeepYearly     int `json:"keepYearly"`
	KeepWithinDays int `json:"keepWithinDays"`
}

// Empty reports whether no rule is set (then nothing is removed).
func (p Retention) Empty() bool {
	return p.KeepLast+p.KeepHourly+p.KeepDaily+p.KeepWeekly+p.KeepMonthly+p.KeepYearly+p.KeepWithinDays == 0
}

// Decision says why a snapshot is kept.
type Decision struct {
	Snapshot *Snapshot `json:"snapshot"`
	Keep     bool      `json:"keep"`
	Reasons  []string  `json:"reasons"`
}

// ApplyRetention evaluates a policy per (host, job) group.
func ApplyRetention(snaps []*Snapshot, p Retention, now time.Time) []Decision {
	groups := map[string][]*Snapshot{}
	var keys []string
	for _, s := range snaps {
		k := s.Host + "\x00" + s.JobID + "\x00" + s.Kind
		if _, ok := groups[k]; !ok {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], s)
	}
	var out []Decision
	for _, k := range keys {
		g := groups[k]
		sort.Slice(g, func(i, j int) bool { return g[i].Time.After(g[j].Time) })
		type bucket struct {
			n   int
			fmt func(time.Time) string
			nm  string
			got map[string]bool
		}
		buckets := []*bucket{
			{p.KeepHourly, func(t time.Time) string { return t.Format("2006-01-02 15") }, "stündlich", map[string]bool{}},
			{p.KeepDaily, func(t time.Time) string { return t.Format("2006-01-02") }, "täglich", map[string]bool{}},
			{p.KeepWeekly, func(t time.Time) string { y, w := t.ISOWeek(); return fmt.Sprintf("%d-%02d", y, w) }, "wöchentlich", map[string]bool{}},
			{p.KeepMonthly, func(t time.Time) string { return t.Format("2006-01") }, "monatlich", map[string]bool{}},
			{p.KeepYearly, func(t time.Time) string { return t.Format("2006") }, "jährlich", map[string]bool{}},
		}
		for i, s := range g {
			d := Decision{Snapshot: s}
			if p.Empty() {
				d.Keep, d.Reasons = true, []string{"keine Regel"}
			}
			if i < p.KeepLast {
				d.Keep = true
				d.Reasons = append(d.Reasons, "letzte "+fmt.Sprint(p.KeepLast))
			}
			if p.KeepWithinDays > 0 && now.Sub(s.Time) <= time.Duration(p.KeepWithinDays)*24*time.Hour {
				d.Keep = true
				d.Reasons = append(d.Reasons, fmt.Sprintf("jünger als %d Tage", p.KeepWithinDays))
			}
			lt := s.Time.Local()
			for _, b := range buckets {
				if b.n <= 0 {
					continue
				}
				key := b.fmt(lt)
				if !b.got[key] && len(b.got) < b.n {
					b.got[key] = true
					d.Keep = true
					d.Reasons = append(d.Reasons, b.nm)
				}
			}
			out = append(out, d)
		}
	}
	return out
}

// Forget deletes snapshots according to the policy. jobID restricts the
// policy to one job ("" = all).
func (r *Repo) Forget(ctx context.Context, p Retention, jobID string, dryRun bool) ([]Decision, error) {
	snaps, err := r.Snapshots(ctx)
	if err != nil {
		return nil, err
	}
	var sel []*Snapshot
	for _, s := range snaps {
		if jobID == "" || s.JobID == jobID {
			sel = append(sel, s)
		}
	}
	dec := ApplyRetention(sel, p, time.Now())
	if dryRun {
		return dec, nil
	}
	for _, d := range dec {
		if !d.Keep {
			if err := r.DeleteSnapshot(ctx, d.Snapshot.ID); err != nil {
				return dec, err
			}
		}
	}
	return dec, nil
}

// PruneResult summarises a prune.
type PruneResult struct {
	Snapshots    int   `json:"snapshots"`
	TotalChunks  int   `json:"totalChunks"`
	UsedChunks   int   `json:"usedChunks"`
	Deleted      int   `json:"deleted"`
	FreedBytes   int64 `json:"freedBytes"`
	LockedErrors int   `json:"lockedErrors"`
}

func (r *Repo) referenced(ctx context.Context, snaps []*Snapshot) (map[string]struct{}, error) {
	used := map[string]struct{}{}
	for _, s := range snaps {
		for _, t := range s.Tree {
			used[t] = struct{}{}
		}
		err := r.WalkTree(ctx, s, func(n *Node) error {
			for _, c := range n.Chunks {
				used[c] = struct{}{}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("snapshot %s: %w", s.ID[:8], err)
		}
	}
	return used, nil
}

// Prune removes chunks that are no longer referenced by any snapshot.
func (r *Repo) Prune(ctx context.Context, dryRun bool, log func(string)) (*PruneResult, error) {
	lk, err := r.Lock(ctx, true, "prune")
	if err != nil {
		return nil, err
	}
	defer lk.Unlock()
	snaps, err := r.Snapshots(ctx)
	if err != nil {
		return nil, err
	}
	used, err := r.referenced(ctx, snaps)
	if err != nil {
		return nil, err
	}
	files, err := r.be.List(ctx, "data")
	if err != nil {
		return nil, err
	}
	res := &PruneResult{Snapshots: len(snaps), TotalChunks: len(files), UsedChunks: len(used)}
	for _, f := range files {
		id := f.Name[strings.LastIndex(f.Name, "/")+1:]
		if _, ok := used[id]; ok {
			continue
		}
		res.Deleted++
		res.FreedBytes += f.Size
		if dryRun {
			continue
		}
		if err := r.be.Delete(ctx, f.Name); err != nil {
			res.LockedErrors++
			if log != nil {
				log(fmt.Sprintf("Löschen nicht möglich (Object Lock?): %s: %v", id[:12], err))
			}
		}
		r.mu.Lock()
		delete(r.known, id)
		r.mu.Unlock()
	}
	return res, nil
}

// ---------------------------------------------------------------------------
// Verification

// VerifyResult summarises a check.
type VerifyResult struct {
	Snapshots    int      `json:"snapshots"`
	Chunks       int      `json:"chunks"`
	Missing      int      `json:"missing"`
	ReadChunks   int      `json:"readChunks"`
	Corrupt      int      `json:"corrupt"`
	BrokenSnaps  []string `json:"brokenSnapshots,omitempty"`
	OrphanChunks int      `json:"orphanChunks"`
	DurationMs   int64    `json:"durationMs"`
	Problems     []string `json:"problems,omitempty"`
}

// OK reports whether the repository is healthy.
func (v *VerifyResult) OK() bool { return v.Missing == 0 && v.Corrupt == 0 && len(v.BrokenSnaps) == 0 }

// Verify checks structure and (optionally) reads a percentage of all data
// to prove it decrypts and matches its checksum.
func (r *Repo) Verify(ctx context.Context, readPercent int, log func(string)) (*VerifyResult, error) {
	start := time.Now()
	logf := func(f string, a ...any) {
		if log != nil {
			log(fmt.Sprintf(f, a...))
		}
	}
	r.mu.Lock()
	r.loaded = false
	r.mu.Unlock()
	if err := r.LoadIndex(ctx); err != nil {
		return nil, err
	}
	snaps, err := r.Snapshots(ctx)
	if err != nil {
		return nil, err
	}
	res := &VerifyResult{Snapshots: len(snaps)}
	used := map[string]struct{}{}
	for _, s := range snaps {
		broken := false
		for _, t := range s.Tree {
			used[t] = struct{}{}
		}
		err := r.WalkTree(ctx, s, func(n *Node) error {
			for _, c := range n.Chunks {
				used[c] = struct{}{}
				if !r.HasBlob(c) {
					res.Missing++
					broken = true
					if len(res.Problems) < 50 {
						res.Problems = append(res.Problems, fmt.Sprintf("Snapshot %s: Chunk %s fehlt (%s)", s.ID[:8], c[:12], n.Path))
					}
				}
			}
			return nil
		})
		if err != nil {
			broken = true
			res.Problems = append(res.Problems, fmt.Sprintf("Snapshot %s: Baum nicht lesbar: %v", s.ID[:8], err))
		}
		if broken {
			res.BrokenSnaps = append(res.BrokenSnaps, s.ID)
		}
		logf("Snapshot %s geprüft", s.ID[:8])
	}
	res.Chunks = len(used)
	r.mu.Lock()
	for id := range r.known {
		if _, ok := used[id]; !ok {
			res.OrphanChunks++
		}
	}
	r.mu.Unlock()
	if readPercent > 0 {
		ids := make([]string, 0, len(used))
		for id := range used {
			if r.HasBlob(id) {
				ids = append(ids, id)
			}
		}
		rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
		n := len(ids) * readPercent / 100
		if n == 0 && len(ids) > 0 {
			n = 1
		}
		logf("Lese %d von %d Chunks (%d%%) und prüfe Integrität", n, len(ids), readPercent)
		for _, id := range ids[:n] {
			if err := ctx.Err(); err != nil {
				return res, err
			}
			res.ReadChunks++
			if _, err := r.LoadBlob(ctx, id); err != nil {
				res.Corrupt++
				if len(res.Problems) < 50 {
					res.Problems = append(res.Problems, err.Error())
				}
			}
		}
	}
	res.DurationMs = time.Since(start).Milliseconds()
	return res, nil
}
