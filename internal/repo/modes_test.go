package repo

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/swissnand/snbackup/internal/backend"
)

// Full → incremental → differential: the differential run must compare
// against the last full backup, the incremental run against the last run.
func TestBackupModes(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	a := filepath.Join(src, "a.txt")
	b := filepath.Join(src, "b.txt")
	mkfile(t, a, 1000)
	mkfile(t, b, 1000)
	be, _ := backend.NewLocal(filepath.Join(dir, "repo"))
	r, err := Init(ctx, be, "passwort-123456")
	if err != nil {
		t.Fatal(err)
	}
	opt := BackupOptions{Paths: []string{src}, JobID: "j"}
	touch := func(p string) {
		time.Sleep(20 * time.Millisecond)
		mkfile(t, p, 1000)
	}
	full, _ := r.Backup(ctx, BackupOptions{Paths: opt.Paths, JobID: "j", Full: true})
	if full.Mode != "full" {
		t.Fatalf("mode %s", full.Mode)
	}
	touch(a)
	inc, _ := r.Backup(ctx, opt)
	if inc.Mode != "incremental" || inc.Stats.ChangedFile != 1 || inc.Parent != full.ID {
		t.Fatalf("incremental: %s %+v", inc.Mode, inc.Stats)
	}
	touch(b)
	inc2, _ := r.Backup(ctx, opt)
	if inc2.Stats.ChangedFile != 1 || inc2.Parent != inc.ID {
		t.Fatalf("incremental 2: %+v", inc2.Stats)
	}
	diff, _ := r.Backup(ctx, BackupOptions{Paths: opt.Paths, JobID: "j", Differential: true})
	if diff.Mode != "differential" || diff.Parent != full.ID || diff.Stats.ChangedFile != 2 {
		t.Fatalf("differential: %s parent=%s %+v", diff.Mode, diff.Parent, diff.Stats)
	}
	// Every snapshot restores completely on its own.
	out := filepath.Join(dir, "out")
	if _, err := r.Restore(ctx, diff, RestoreOptions{Target: out, Verify: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, StoreName(b))); err != nil {
		t.Fatal(err)
	}
}
