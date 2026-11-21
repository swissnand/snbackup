package secure

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPasswordAndTOTPAndBox(t *testing.T) {
	h, _ := HashPassword("Richtig-Pass-123")
	if !CheckPassword(h, "Richtig-Pass-123") || CheckPassword(h, "falsch") {
		t.Fatal("password check")
	}
	s := NewTOTPSecret()
	now := time.Now()
	if !ValidateTOTP(s, TOTPCode(s, now), now) || ValidateTOTP(s, "000000", now) && TOTPCode(s, now) != "000000" {
		t.Fatal("totp")
	}
	if ValidateTOTP(s, TOTPCode(s, now.Add(-5*time.Minute)), now) {
		t.Fatal("old totp accepted")
	}
	b, err := LoadBox(filepath.Join(t.TempDir(), "k"))
	if err != nil {
		t.Fatal(err)
	}
	enc := b.Encrypt("geheim")
	if dec, _ := b.Decrypt(enc); dec != "geheim" || enc == "geheim" {
		t.Fatal("box")
	}
	if PasswordPolicy("kurz") == nil || PasswordPolicy("Lang-genug-2026") != nil {
		t.Fatal("policy")
	}
}
