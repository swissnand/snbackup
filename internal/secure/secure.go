// Package secure contains password hashing, secret encryption at rest and
// TOTP two-factor authentication.
package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const passwordIters = 600_000

// RandomToken returns n random bytes, URL-safe base64 encoded.
func RandomToken(n int) string {
	b := make([]byte, n)
	_, _ = io.ReadFull(rand.Reader, b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashPassword returns a PBKDF2-SHA256 password hash.
func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	k, err := pbkdf2.Key(sha256.New, pw, salt, passwordIters, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", passwordIters, hex.EncodeToString(salt), hex.EncodeToString(k)), nil
}

// CheckPassword verifies a password against a hash in constant time.
func CheckPassword(hash, pw string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 10000 {
		return false
	}
	salt, err1 := hex.DecodeString(parts[2])
	want, err2 := hex.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// PasswordPolicy validates a new account password.
func PasswordPolicy(pw string) error {
	if len(pw) < 12 {
		return errors.New("Das Passwort muss mindestens 12 Zeichen lang sein")
	}
	var lower, upper, digit, other bool
	for _, c := range pw {
		switch {
		case c >= 'a' && c <= 'z':
			lower = true
		case c >= 'A' && c <= 'Z':
			upper = true
		case c >= '0' && c <= '9':
			digit = true
		default:
			other = true
		}
	}
	n := 0
	for _, b := range []bool{lower, upper, digit, other} {
		if b {
			n++
		}
	}
	if n < 3 {
		return errors.New("Das Passwort braucht mindestens 3 von: Kleinbuchstaben, Grossbuchstaben, Ziffern, Sonderzeichen")
	}
	return nil
}

// TokenHash hashes API tokens for storage.
func TokenHash(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

// ---------------------------------------------------------------------------
// Secret box: encrypts credentials (repository passwords, S3 keys, SMTP
// passwords) inside the configuration file with a machine-local key file that
// is readable by the service account only.

// Box encrypts and decrypts strings.
type Box struct{ key []byte }

const boxPrefix = "enc:v1:"

// LoadBox loads or creates the key file.
func LoadBox(path string) (*Box, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		k, err := hex.DecodeString(strings.TrimSpace(string(b)))
		if err != nil || len(k) != 32 {
			return nil, errors.New("invalid secret key file " + path)
		}
		return &Box{key: k}, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	k := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, k); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(k)), 0o600); err != nil {
		return nil, err
	}
	return &Box{key: k}, nil
}

// Encrypt encrypts s (empty strings stay empty).
func (b *Box) Encrypt(s string) string {
	if s == "" || strings.HasPrefix(s, boxPrefix) {
		return s
	}
	block, _ := aes.NewCipher(b.key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	_, _ = io.ReadFull(rand.Reader, nonce)
	return boxPrefix + base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(s), nil))
}

// Decrypt reverses Encrypt. Plain values are returned unchanged.
func (b *Box) Decrypt(s string) (string, error) {
	if !strings.HasPrefix(s, boxPrefix) {
		return s, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, boxPrefix))
	if err != nil {
		return "", err
	}
	block, _ := aes.NewCipher(b.key)
	gcm, _ := cipher.NewGCM(block)
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("invalid secret")
	}
	pt, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("secret cannot be decrypted (wrong key file?)")
	}
	return string(pt), nil
}

// ---------------------------------------------------------------------------
// TOTP (RFC 6238) for two-factor authentication.

// NewTOTPSecret returns a random base32 secret.
func NewTOTPSecret() string {
	b := make([]byte, 20)
	_, _ = io.ReadFull(rand.Reader, b)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
}

func totpAt(secret string, counter uint64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.ReplaceAll(secret, " ", "")))
	if err != nil {
		return "", err
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff) % 1_000_000
	return fmt.Sprintf("%06d", code), nil
}

// TOTPCode returns the current code (used in tests).
func TOTPCode(secret string, t time.Time) string {
	c, _ := totpAt(secret, uint64(t.Unix()/30))
	return c
}

// ValidateTOTP checks a code with ±1 time step tolerance.
func ValidateTOTP(secret, code string, t time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	step := uint64(t.Unix() / 30)
	for _, d := range []int64{-1, 0, 1} {
		c, err := totpAt(secret, uint64(int64(step)+d))
		if err == nil && subtle.ConstantTimeCompare([]byte(c), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// TOTPURI builds an otpauth:// URI for authenticator apps.
func TOTPURI(secret, user string) string {
	return fmt.Sprintf("otpauth://totp/SWISSNAND%%20Backup:%s?secret=%s&issuer=SWISSNAND%%20Backup&digits=6&period=30", user, secret)
}
