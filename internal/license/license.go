// Package license verifies SWISSNAND Backup license keys.
//
// Editions:
//   - Community: free for private, non-commercial use, education and
//     registered non-profit organisations. All features included.
//   - Business / Enterprise: required for any commercial use. Licence keys are
//     Ed25519-signed by SWISSNAND and verified offline (no phone-home).
package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// PublicKey is the Ed25519 public key (base64) used to verify licences.
// Set at build time:  -ldflags "-X github.com/swissnand/snbackup/internal/license.PublicKey=<base64>"
var PublicKey = ""

// License is the signed licence payload.
type License struct {
	ID       string    `json:"id"`
	Edition  string    `json:"edition"` // business | enterprise
	Customer string    `json:"customer"`
	Email    string    `json:"email,omitempty"`
	Servers  int       `json:"servers"` // 0 = unlimited
	Issued   time.Time `json:"issued"`
	Expires  time.Time `json:"expires"` // zero = perpetual
}

// Info is the effective licence state shown in the GUI.
type Info struct {
	Edition    string     `json:"edition"`
	Commercial bool       `json:"commercial"`
	Customer   string     `json:"customer,omitempty"`
	Servers    int        `json:"servers,omitempty"`
	Expires    *time.Time `json:"expires,omitempty"`
	Valid      bool       `json:"valid"`
	Message    string     `json:"message"`
	ID         string     `json:"id,omitempty"`
}

const prefix = "SNB1."

// Sign creates a licence key (used by the sn-licgen tool).
func Sign(l License, priv ed25519.PrivateKey) (string, error) {
	payload, err := json.Marshal(l)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(priv, payload)
	return prefix + base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Verify parses and verifies a licence key.
func Verify(key string) (*License, error) {
	key = strings.TrimSpace(key)
	if !strings.HasPrefix(key, prefix) {
		return nil, errors.New("Ungültiges Lizenzformat")
	}
	parts := strings.Split(strings.TrimPrefix(key, prefix), ".")
	if len(parts) != 2 {
		return nil, errors.New("Ungültiges Lizenzformat")
	}
	payload, err1 := base64.RawURLEncoding.DecodeString(parts[0])
	sig, err2 := base64.RawURLEncoding.DecodeString(parts[1])
	if err1 != nil || err2 != nil {
		return nil, errors.New("Ungültiges Lizenzformat")
	}
	pk, err := base64.StdEncoding.DecodeString(PublicKey)
	if err != nil || len(pk) != ed25519.PublicKeySize {
		return nil, errors.New("Dieser Build enthält keinen Lizenz-Schlüssel (Entwickler-Build)")
	}
	if !ed25519.Verify(pk, payload, sig) {
		return nil, errors.New("Signatur ungültig – Lizenz nicht von SWISSNAND ausgestellt")
	}
	var l License
	if err := json.Unmarshal(payload, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// Evaluate returns the licence info for a stored key ("" = Community).
func Evaluate(key string) Info {
	if strings.TrimSpace(key) == "" {
		return Info{Edition: "Community", Valid: true, Message: "Kostenlos für private, nicht-kommerzielle Nutzung. Für den Einsatz in Unternehmen ist eine Business-Lizenz nötig."}
	}
	l, err := Verify(key)
	if err != nil {
		return Info{Edition: "Community", Valid: false, Message: err.Error()}
	}
	ed := strings.ToUpper(l.Edition[:1]) + l.Edition[1:]
	info := Info{Edition: ed, Commercial: true, Customer: l.Customer, Servers: l.Servers, Valid: true, ID: l.ID, Message: "Lizenziert für kommerzielle Nutzung"}
	if !l.Expires.IsZero() {
		e := l.Expires
		info.Expires = &e
		if time.Now().After(e) {
			info.Valid = false
			info.Message = "Lizenz abgelaufen am " + e.Format("02.01.2006") + " – bitte verlängern"
		}
	}
	return info
}
