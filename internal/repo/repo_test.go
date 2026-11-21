package repo

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swissnand/snbackup/internal/backend"
)

func mkfile(t *testing.T, p string, size int) []byte {
	t.Helper()
	data := make([]byte, size)
	_, _ = rand.Read(data)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o640); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRoundtripDedupPruneVerifyCopy(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	big := mkfile(t, filepath.Join(src, "big.bin"), 9<<20)
	mkfile(t, filepath.Join(src, "sub", "a.txt"), 1234)
	mkfile(t, filepath.Join(src, "node_modules", "x.js"), 100)
	_ = os.Symlink("sub/a.txt", filepath.Join(src, "link"))

	be, _ := backend.NewLocal(filepath.Join(dir, "repo"))
	r, err := Init(ctx, be, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, be, "wrong password!"); err != ErrWrongPassword {
		t.Fatalf("expected wrong password, got %v", err)
	}
	r, err = Open(ctx, be, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	opt := BackupOptions{Paths: []string{src}, Excludes: []string{"node_modules"}, JobID: "j1"}
	s1, err := r.Backup(ctx, opt)
	if err != nil {
		t.Fatal(err)
	}
	if s1.Stats.Files != 2 || s1.Stats.Links != 1 {
		t.Fatalf("unexpected stats %+v", s1.Stats)
	}

	// Modify the middle of the big file: only a few chunks may be new.
	copy(big[4<<20:], []byte("CHANGED CONTENT IN THE MIDDLE"))
	_ = os.WriteFile(filepath.Join(src, "big.bin"), big, 0o640)
	time.Sleep(10 * time.Millisecond)
	s2, err := r.Backup(ctx, opt)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Stats.NewBytes >= int64(len(big))/2 {
		t.Fatalf("dedup ineffective: %d new bytes", s2.Stats.NewBytes)
	}
	if s2.Stats.Unchanged != 1 {
		t.Fatalf("expected 1 unchanged file, got %d", s2.Stats.Unchanged)
	}

	out := filepath.Join(dir, "restore")
	res, err := r.Restore(ctx, s2, RestoreOptions{Target: out, Verify: true})
	if err != nil || res.Errors != 0 {
		t.Fatalf("restore: %v %+v", err, res)
	}
	got, _ := os.ReadFile(filepath.Join(out, StoreName(src), "big.bin"))
	if !bytes.Equal(got, big) {
		t.Fatal("restored content differs")
	}
	if _, err := os.Stat(filepath.Join(out, StoreName(src), "node_modules")); err == nil {
		t.Fatal("excluded dir restored")
	}

	// Retention: keep only last 1 → s1 forgotten, prune frees chunks.
	dec, err := r.Forget(ctx, Retention{KeepLast: 1}, "", false)
	if err != nil || len(dec) != 2 {
		t.Fatal(err, len(dec))
	}
	pr, err := r.Prune(ctx, false, nil)
	if err != nil || pr.Deleted == 0 {
		t.Fatalf("prune: %v %+v", err, pr)
	}
	vr, err := r.Verify(ctx, 100, nil)
	if err != nil || !vr.OK() {
		t.Fatalf("verify: %v %+v", err, vr)
	}

	// Backup copy to a second repository with another password.
	be2, _ := backend.NewLocal(filepath.Join(dir, "repo2"))
	r2, _ := Init(ctx, be2, "another password")
	cr, err := CopySnapshots(ctx, r, r2, nil, "", nil)
	if err != nil || cr.Copied != 1 {
		t.Fatalf("copy: %v %+v", err, cr)
	}
	snaps, _ := r2.Snapshots(ctx)
	out2 := filepath.Join(dir, "restore2")
	if _, err := r2.Restore(ctx, snaps[0], RestoreOptions{Target: out2, Verify: true}); err != nil {
		t.Fatal(err)
	}
	got2, _ := os.ReadFile(filepath.Join(out2, StoreName(src), "big.bin"))
	if !bytes.Equal(got2, big) {
		t.Fatal("copied restore differs")
	}

	// Tamper detection: flip a byte in a chunk.
	files, _ := be.List(ctx, "data")
	p := filepath.Join(dir, "repo", files[0].Name)
	raw, _ := os.ReadFile(p)
	raw[len(raw)-5] ^= 0xff
	_ = os.WriteFile(p, raw, 0o600)
	vr, _ = r.Verify(ctx, 100, nil)
	if vr.Corrupt == 0 {
		t.Fatal("tampering not detected")
	}
}

func TestMatcher(t *testing.T) {
	m, _ := newMatcher([]string{"*.tmp", "node_modules", "/var/log/**", "**/cache"})
	cases := map[string]bool{
		"home/a/x.tmp":          true,
		"home/a/node_modules/y": false, // component match is on the dir itself
		"home/a/node_modules":   true,
		"var/log/syslog":        true,
		"var/logger":            false,
		"home/u/.local/cache":   true,
		"home/u/document.txt":   false,
	}
	for p, want := range cases {
		if m.match(p) != want {
			t.Errorf("%s: want %v", p, want)
		}
	}
}

func TestSafeRel(t *testing.T) {
	for _, p := range []string{"../etc/passwd", "/etc/passwd", "a/../../b", ""} {
		if safeRel(p) {
			t.Errorf("%q should be unsafe", p)
		}
	}
	if !safeRel("etc/nginx/nginx.conf") || strings.Contains("x", "y") {
		t.Error("valid path rejected")
	}
}
