package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// CopyResult summarises a backup-copy run.
type CopyResult struct {
	Copied      int   `json:"copied"`
	Skipped     int   `json:"skipped"`
	StoredBytes int64 `json:"storedBytes"`
}

// CopySnapshots copies snapshots from src to dst (backup copy job / 3-2-1
// rule). Both repositories may have different passwords and backends; data is
// re-encrypted for the destination. mapFile (optional) persists the chunk ID
// mapping so already copied chunks are not downloaded again.
func CopySnapshots(ctx context.Context, src, dst *Repo, filter func(*Snapshot) bool, mapFile string, log func(string)) (*CopyResult, error) {
	logf := func(f string, a ...any) {
		if log != nil {
			log(fmt.Sprintf(f, a...))
		}
	}
	dst.ResetIndex()
	if err := dst.LoadIndex(ctx); err != nil {
		return nil, err
	}
	mapping := map[string]string{}
	if mapFile != "" {
		if b, err := os.ReadFile(mapFile); err == nil {
			_ = json.Unmarshal(b, &mapping)
		}
	}
	var mu sync.Mutex
	save := func() {
		if mapFile == "" {
			return
		}
		mu.Lock()
		b, _ := json.Marshal(mapping)
		mu.Unlock()
		_ = os.WriteFile(mapFile, b, 0o600)
	}
	defer save()

	srcSnaps, err := src.Snapshots(ctx)
	if err != nil {
		return nil, err
	}
	dstSnaps, err := dst.Snapshots(ctx)
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, s := range dstSnaps {
		if s.Origin != "" {
			have[s.Origin] = true
		}
		have[s.ID] = true
	}
	res := &CopyResult{}
	for i := len(srcSnaps) - 1; i >= 0; i-- { // oldest first
		s := srcSnaps[i]
		if filter != nil && !filter(s) {
			continue
		}
		if have[s.ID] {
			res.Skipped++
			continue
		}
		logf("Kopiere Snapshot %s vom %s", s.ID[:8], s.Time.Local().Format("02.01.2006 15:04"))
		tw := &treeWriter{r: dst, ctx: ctx}
		err := src.WalkTree(ctx, s, func(n *Node) error {
			for i, c := range n.Chunks {
				mu.Lock()
				m, ok := mapping[c]
				mu.Unlock()
				if ok && dst.HasBlob(m) {
					n.Chunks[i] = m
					continue
				}
				data, err := src.LoadBlob(ctx, c)
				if err != nil {
					return err
				}
				id, stored, err := dst.SaveBlob(ctx, data)
				if err != nil {
					return err
				}
				res.StoredBytes += stored
				mu.Lock()
				mapping[c] = id
				mu.Unlock()
				n.Chunks[i] = id
			}
			return tw.Add(n)
		})
		if err != nil {
			return res, err
		}
		ids, err := tw.Close()
		if err != nil {
			return res, err
		}
		res.StoredBytes += tw.stored
		cp := *s
		cp.ID = ""
		cp.Origin = s.ID
		cp.Tree = ids
		if err := dst.SaveSnapshot(ctx, &cp); err != nil {
			return res, err
		}
		res.Copied++
		save()
	}
	return res, nil
}
