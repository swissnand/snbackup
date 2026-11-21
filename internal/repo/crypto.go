package repo

import (
	"bytes"
	"compress/flate"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
)

// Cryptographic design
//
//   - Every repository has a random 256-bit master key set (encryption key,
//     MAC key and chunker seed). It never leaves memory unencrypted.
//   - The master key is stored in keys/<id>, encrypted with a key-encryption
//     key derived from the repository password via PBKDF2-HMAC-SHA256
//     (default 600'000 iterations, OWASP 2023 recommendation). Multiple
//     passwords can unlock the same repository.
//   - All objects (chunks, trees, snapshots) are encrypted with AES-256-GCM
//     (authenticated encryption, random 96-bit nonce). The object name is
//     bound as additional authenticated data, so objects cannot be swapped.
//   - Chunk IDs are HMAC-SHA256(macKey, plaintext). The storage provider
//     can therefore not confirm whether a known file is stored (no
//     fingerprinting), and every loaded chunk is re-verified.
//   - The chunker uses a secret, per-repository gear table so chunk
//     boundaries leak no information about the content either.

const (
	formatVersion   = 1
	defaultKDFIters = 600_000
	objVersion      = 0x01

	compRaw   = 0x00
	compFlate = 0x01
)

// ErrWrongPassword is returned when no key file matches the password.
var ErrWrongPassword = errors.New("wrong password or no matching key")

// MasterKey holds the secret repository keys.
type MasterKey struct {
	Enc  []byte `json:"enc"`
	Mac  []byte `json:"mac"`
	Gear []byte `json:"gear"`
}

func newMasterKey() (*MasterKey, error) {
	k := &MasterKey{Enc: make([]byte, 32), Mac: make([]byte, 32), Gear: make([]byte, 32)}
	for _, b := range [][]byte{k.Enc, k.Mac, k.Gear} {
		if _, err := io.ReadFull(rand.Reader, b); err != nil {
			return nil, err
		}
	}
	return k, nil
}

func deriveKEK(password string, salt []byte, iter int) ([]byte, error) {
	return pbkdf2.Key(sha256.New, password, salt, iter, 32)
}

// Seal encrypts plaintext with AES-256-GCM. Output: version | nonce | ct+tag.
func Seal(key, plaintext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 1+gcm.NonceSize(), 1+gcm.NonceSize()+len(plaintext)+gcm.Overhead())
	out[0] = objVersion
	if _, err := io.ReadFull(rand.Reader, out[1:]); err != nil {
		return nil, err
	}
	return gcm.Seal(out, out[1:1+gcm.NonceSize()], plaintext, aad), nil
}

// OpenSealed decrypts data produced by Seal.
func OpenSealed(key, data, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < 1+gcm.NonceSize()+gcm.Overhead() || data[0] != objVersion {
		return nil, errors.New("invalid or corrupted object")
	}
	ns := gcm.NonceSize()
	pt, err := gcm.Open(nil, data[1:1+ns], data[1+ns:], aad)
	if err != nil {
		return nil, errors.New("authentication failed: object corrupted or tampered with")
	}
	return pt, nil
}

func (k *MasterKey) chunkID(data []byte) string {
	m := hmac.New(sha256.New, k.Mac)
	m.Write(data)
	return hex.EncodeToString(m.Sum(nil))
}

// Compression levels.
const (
	CompressionOff  = "off"
	CompressionFast = "fast"
	CompressionAuto = "auto"
	CompressionMax  = "max"
)

func compress(data []byte, mode string) []byte {
	level := flate.DefaultCompression
	switch mode {
	case CompressionOff:
		return append([]byte{compRaw}, data...)
	case CompressionFast:
		level = flate.BestSpeed
	case CompressionMax:
		level = flate.BestCompression
	}
	var buf bytes.Buffer
	buf.Grow(len(data)/2 + 16)
	buf.WriteByte(compFlate)
	w, _ := flate.NewWriter(&buf, level)
	_, _ = w.Write(data)
	_ = w.Close()
	// Incompressible data (media, archives, encrypted files) is stored raw.
	if buf.Len() >= len(data)+1 {
		return append([]byte{compRaw}, data...)
	}
	return buf.Bytes()
}

func decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty payload")
	}
	switch data[0] {
	case compRaw:
		return data[1:], nil
	case compFlate:
		return io.ReadAll(flate.NewReader(bytes.NewReader(data[1:])))
	}
	return nil, errors.New("unknown compression")
}
