package archive

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptedArchive(t *testing.T) {
	d := t.TempDir()
	src := filepath.Join(d, "src")
	os.MkdirAll(src, 0o755)
	data := make([]byte, 3<<20)
	rand.Read(data)
	os.WriteFile(filepath.Join(src, "a.bin"), data, 0o644)
	out := filepath.Join(d, "x.tar.gz.snarc")
	if _, _, err := Create(out, []string{src}, nil, "Archiv-Passwort", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Extract(out, filepath.Join(d, "bad"), "falsch"); err == nil {
		t.Fatal("wrong password accepted")
	}
	raw, _ := os.ReadFile(out)
	os.WriteFile(out+"2.snarc", raw[:len(raw)-100], 0o600)
	if _, err := Extract(out+"2.snarc", filepath.Join(d, "trunc"), "Archiv-Passwort"); err == nil {
		t.Fatal("truncation not detected")
	}
	if _, err := Extract(out, filepath.Join(d, "ok"), "Archiv-Passwort"); err != nil {
		t.Fatal(err)
	}
	var got []byte
	filepath.Walk(filepath.Join(d, "ok"), func(p string, fi os.FileInfo, err error) error {
		if err == nil && fi.Name() == "a.bin" {
			got, _ = os.ReadFile(p)
		}
		return nil
	})
	if !bytes.Equal(got, data) {
		t.Fatal("content differs")
	}
}
