// Package server provides the HTTPS web console and REST API.
package server

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/swissnand/snbackup/internal/jobs"
	"github.com/swissnand/snbackup/internal/repo"
	"github.com/swissnand/snbackup/internal/secure"
	"github.com/swissnand/snbackup/internal/store"
)

//go:embed web
var webFS embed.FS

// Config configures the server.
type Config struct {
	Listen   string
	DataDir  string
	CertFile string
	KeyFile  string
	NoTLS    bool // only allowed on loopback (behind a reverse proxy)
}

// Server is the web console.
type Server struct {
	cfg      Config
	st       *store.Store
	m        *jobs.Manager
	sessions *sessionStore
	limiter  *loginLimiter
	auditMu  sync.Mutex
	trees    *treeCache
	usageMu  sync.Mutex
	usage    map[string]repoUsage
}

type repoUsage struct {
	Bytes     int64      `json:"bytes"`
	Chunks    int        `json:"chunks"`
	Snapshots int        `json:"snapshots"`
	Logical   int64      `json:"logical"`
	Updated   time.Time  `json:"updated"`
	Error     string     `json:"error,omitempty"`
	Latest    *time.Time `json:"latest,omitempty"`
}

// New creates the server.
func New(cfg Config, st *store.Store, m *jobs.Manager) *Server {
	s := &Server{cfg: cfg, st: st, m: m, sessions: newSessionStore(), limiter: newLoginLimiter(), trees: newTreeCache(6), usage: map[string]repoUsage{}}
	m.Audit = func(user, action, detail string) { s.audit(user, "", action, detail) }
	return s
}

// Bootstrap creates the first admin account if none exists and returns
// the generated password ("" if users exist).
func Bootstrap(st *store.Store) (string, error) {
	var pw string
	err := st.Update(func(s *store.State) error {
		if len(s.Users) > 0 {
			return nil
		}
		pw = secure.RandomToken(12)
		h, err := secure.HashPassword(pw)
		if err != nil {
			return err
		}
		s.Users = append(s.Users, &store.User{Username: "admin", Role: "admin", PassHash: h, MustChange: true, Created: time.Now()})
		return nil
	})
	if pw != "" {
		f := filepath.Join(st.Dir(), "initial-admin-password")
		_ = os.WriteFile(f, []byte(pw+"\n"), 0o600)
	}
	return pw, err
}

func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

// Run starts the HTTPS server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	sub, _ := fs.Sub(webFS, "web")
	static := http.FileServer(http.FS(sub))
	mux := http.NewServeMux()
	s.routes(mux)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			jsonErr(w, http.StatusNotFound, "not found")
			return
		}
		if r.URL.Path != "/" && !strings.Contains(r.URL.Path, ".") {
			r.URL.Path = "/"
		}
		if strings.HasSuffix(r.URL.Path, ".js") || strings.HasSuffix(r.URL.Path, ".css") || r.URL.Path == "/" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		static.ServeHTTP(w, r)
	}))
	go s.usageLoop(ctx)

	srv := &http.Server{
		Addr:              s.cfg.Listen,
		Handler:           s.secure(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          log.New(os.Stderr, "http: ", 0),
	}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	if s.cfg.NoTLS {
		if !isLoopback(s.cfg.Listen) {
			return errors.New("--no-tls ist nur auf localhost/127.0.0.1 erlaubt (z. B. hinter einem Reverse-Proxy)")
		}
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
	cert, key := s.cfg.CertFile, s.cfg.KeyFile
	if cert == "" || key == "" {
		var err error
		cert, key, err = ensureSelfSigned(s.cfg.DataDir)
		if err != nil {
			return err
		}
	}
	srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256}}
	err := srv.ListenAndServeTLS(cert, key)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// secure adds security headers, IP allowlisting and request size limits.
func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if allow := s.st.Settings().IPAllowlist; len(allow) > 0 && !ipAllowed(clientIP(r), allow) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func ipAllowed(ip string, list []string) bool {
	p := net.ParseIP(ip)
	if p == nil {
		return false
	}
	if p.IsLoopback() {
		return true // never lock yourself out locally
	}
	for _, e := range list {
		e = strings.TrimSpace(e)
		if !strings.Contains(e, "/") {
			if net.ParseIP(e).Equal(p) {
				return true
			}
			continue
		}
		if _, n, err := net.ParseCIDR(e); err == nil && n.Contains(p) {
			return true
		}
	}
	return false
}

func validIPEntry(e string) bool {
	e = strings.TrimSpace(e)
	if strings.Contains(e, "/") {
		_, _, err := net.ParseCIDR(e)
		return err == nil
	}
	return net.ParseIP(e) != nil
}

func ensureSelfSigned(dir string) (string, string, error) {
	certPath, keyPath := filepath.Join(dir, "tls-cert.pem"), filepath.Join(dir, "tls-key.pem")
	if _, err := os.Stat(certPath); err == nil {
		if _, err := os.Stat(keyPath); err == nil {
			return certPath, keyPath, nil
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	host, _ := os.Hostname()
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host, Organization: []string{"SWISSNAND Backup (self-signed)"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(2, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{host, "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() {
				tpl.IPAddresses = append(tpl.IPAddresses, ipn.IP)
			}
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	kb, _ := x509.MarshalECPrivateKey(key)
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return "", "", err
	}
	return certPath, keyPath, nil
}

// ---------------------------------------------------------------------------
// Audit log

type auditEntry struct {
	Time   time.Time `json:"time"`
	User   string    `json:"user"`
	IP     string    `json:"ip,omitempty"`
	Action string    `json:"action"`
	Detail string    `json:"detail,omitempty"`
}

func (s *Server) audit(user, ip, action, detail string) {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	f, err := os.OpenFile(filepath.Join(s.st.Dir(), "audit.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(auditEntry{Time: time.Now(), User: user, IP: ip, Action: action, Detail: detail})
	f.Write(append(b, '\n'))
}

func (s *Server) readAudit(limit int) []auditEntry {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	f, err := os.Open(filepath.Join(s.st.Dir(), "audit.jsonl"))
	if err != nil {
		return nil
	}
	defer f.Close()
	var all []auditEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e auditEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			all = append(all, e)
		}
	}
	out := make([]auditEntry, 0, limit)
	for i := len(all) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, all[i])
	}
	return out
}

// ---------------------------------------------------------------------------
// Snapshot tree cache for the file browser.

type treeCache struct {
	mu    sync.Mutex
	max   int
	order []string
	data  map[string][]*repo.Node
}

func newTreeCache(max int) *treeCache { return &treeCache{max: max, data: map[string][]*repo.Node{}} }

func (c *treeCache) get(k string) ([]*repo.Node, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.data[k]
	return v, ok
}

func (c *treeCache) put(k string, v []*repo.Node) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data[k]; !ok {
		c.order = append(c.order, k)
	}
	c.data[k] = v
	for len(c.order) > c.max {
		delete(c.data, c.order[0])
		c.order = c.order[1:]
	}
}

// ---------------------------------------------------------------------------
// Repository usage statistics (computed in the background).

func (s *Server) usageLoop(ctx context.Context) {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	s.refreshUsage(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.refreshUsage(ctx)
		}
	}
}

func (s *Server) refreshUsage(ctx context.Context) {
	for _, r := range s.st.Repos() {
		s.refreshRepoUsage(ctx, r.ID)
	}
}

func (s *Server) refreshRepoUsage(ctx context.Context, id string) {
	u := repoUsage{Updated: time.Now()}
	c, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	rp, err := s.m.OpenRepo(c, id)
	if err != nil {
		u.Error = err.Error()
	} else {
		u.Bytes, u.Chunks, err = rp.Usage(c)
		if err != nil {
			u.Error = err.Error()
		}
		if snaps, err := rp.Snapshots(c); err == nil {
			u.Snapshots = len(snaps)
			seen := map[string]bool{}
			for _, sn := range snaps {
				if !seen[sn.JobID] {
					seen[sn.JobID] = true
					u.Logical += sn.Stats.Bytes
				}
			}
			if len(snaps) > 0 {
				t := snaps[0].Time
				u.Latest = &t
			}
		}
	}
	s.usageMu.Lock()
	s.usage[id] = u
	s.usageMu.Unlock()
}

func (s *Server) getUsage(id string) (repoUsage, bool) {
	s.usageMu.Lock()
	defer s.usageMu.Unlock()
	u, ok := s.usage[id]
	return u, ok
}

func fmtErr(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprint(err)
}
