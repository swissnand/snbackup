// Package agent implements the SWISSNAND Backup agent: it registers with a
// backup server, polls it over HTTPS (outbound only), runs the assigned jobs
// with the local engine and reports progress and results.
package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/swissnand/snbackup/internal/jobs"
	"github.com/swissnand/snbackup/internal/store"
	"github.com/swissnand/snbackup/internal/version"
)

// Config is stored in <data>/agent.json (mode 0600).
type Config struct {
	Server      string `json:"server"`
	AgentID     string `json:"agentId"`
	Secret      string `json:"secret"`
	Fingerprint string `json:"fingerprint,omitempty"` // sha256 of the server certificate (pinning)
	Name        string `json:"name"`
}

// CertFingerprint returns the SHA-256 fingerprint of a DER certificate.
func CertFingerprint(der []byte) string {
	h := sha256.Sum256(der)
	return "sha256:" + hex.EncodeToString(h[:])
}

// Client builds an HTTPS client. With a fingerprint the server certificate is
// pinned (works with the server's self-signed certificate); without it the
// system CA store is used.
func Client(fingerprint string) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if fp := NormalizeFingerprint(fingerprint); fp != "" {
		tr.TLSClientConfig.InsecureSkipVerify = true // replaced by strict pinning below
		tr.TLSClientConfig.VerifyPeerCertificate = func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) == 0 || CertFingerprint(raw[0]) != fp {
				return errors.New("Server-Zertifikat stimmt nicht mit dem Fingerabdruck überein (möglicher Angriff)")
			}
			return nil
		}
	}
	return &http.Client{Transport: tr, Timeout: 60 * time.Second}
}

// NormalizeFingerprint accepts "sha256:ab:cd…", "AB CD…" or plain hex.
func NormalizeFingerprint(fp string) string {
	fp = strings.ToLower(strings.TrimSpace(fp))
	fp = strings.TrimPrefix(fp, "sha256:")
	fp = strings.NewReplacer(":", "", " ", "").Replace(fp)
	if fp == "" {
		return ""
	}
	return "sha256:" + fp
}

func configPath(dataDir string) string { return filepath.Join(dataDir, "agent.json") }

// LoadConfig reads the agent configuration.
func LoadConfig(dataDir string) (*Config, error) {
	b, err := os.ReadFile(configPath(dataDir))
	if err != nil {
		return nil, err
	}
	var c Config
	return &c, json.Unmarshal(b, &c)
}

// Enroll registers this machine with a backup server.
func Enroll(dataDir, server, token, fingerprint, name string) (*Config, error) {
	server = strings.TrimRight(server, "/")
	if !strings.HasPrefix(server, "https://") {
		return nil, errors.New("Server-URL muss mit https:// beginnen")
	}
	host, _ := os.Hostname()
	if name == "" {
		name = host
	}
	body, _ := json.Marshal(map[string]string{"token": token, "name": name, "hostname": host, "os": runtime.GOOS, "arch": runtime.GOARCH, "version": version.Version})
	resp, err := Client(fingerprint).Post(server+"/api/agent/enroll", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		AgentID string `json:"agentId"`
		Secret  string `json:"secret"`
		Error   string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != 200 {
		if out.Error == "" {
			out.Error = resp.Status
		}
		return nil, errors.New(out.Error)
	}
	c := &Config{Server: server, AgentID: out.AgentID, Secret: out.Secret, Fingerprint: fingerprint, Name: name}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return c, os.WriteFile(configPath(dataDir), b, 0o600)
}

// Agent runs tasks for a backup server.
type Agent struct {
	cfg    *Config
	client *http.Client
	st     *store.Store
	m      *jobs.Manager
	mu     sync.Mutex
	active map[string]string // runID → local job/live key
}

// New creates an agent using dataDir for local state.
func New(dataDir string, cfg *Config, hostRoot string) (*Agent, error) {
	st, err := store.Open(dataDir)
	if err != nil {
		return nil, err
	}
	m := jobs.New(st)
	m.HostRoot = hostRoot
	return &Agent{cfg: cfg, client: Client(cfg.Fingerprint), st: st, m: m, active: map[string]string{}}, nil
}

func (a *Agent) call(ctx context.Context, path string, in, out any) error {
	b, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.Server+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Agent "+a.cfg.AgentID+":"+a.cfg.Secret)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// Run polls the server until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	host, _ := os.Hostname()
	log.Printf("Agent «%s» verbunden mit %s", a.cfg.Name, a.cfg.Server)
	backoff := 5 * time.Second
	for {
		var resp struct {
			Tasks  []*jobs.AgentTask `json:"tasks"`
			Cancel []string          `json:"cancel"`
		}
		err := a.call(ctx, "/api/agent/poll", map[string]string{"hostname": host, "os": runtime.GOOS, "arch": runtime.GOARCH, "version": version.Version}, &resp)
		if err != nil {
			if ctx.Err() != nil {
				a.m.Stop()
				return nil
			}
			log.Printf("Server nicht erreichbar: %v", err)
			if strings.Contains(err.Error(), "HTTP 401") || strings.Contains(err.Error(), "HTTP 403") {
				log.Printf("Agent wurde auf dem Server entfernt oder gesperrt")
			}
			select {
			case <-ctx.Done():
				a.m.Stop()
				return nil
			case <-time.After(backoff):
			}
			if backoff < time.Minute {
				backoff *= 2
			}
			continue
		}
		backoff = 5 * time.Second
		for _, id := range resp.Cancel {
			a.mu.Lock()
			key, ok := a.active[id]
			a.mu.Unlock()
			if ok {
				log.Printf("Abbruch angefordert: %s", key)
				a.m.Cancel(key)
			}
		}
		for _, t := range resp.Tasks {
			go a.runTask(ctx, t)
		}
		select {
		case <-ctx.Done():
			a.m.Stop()
			return nil
		case <-time.After(5 * time.Second):
		}
	}
}

func (a *Agent) report(ctx context.Context, r jobs.AgentReport) {
	for i := 0; i < 5; i++ {
		if err := a.call(ctx, "/api/agent/report", r, nil); err == nil {
			return
		} else if !r.Done {
			return
		}
		time.Sleep(time.Duration(i+1) * 3 * time.Second)
	}
}

func (a *Agent) upsert(t *jobs.AgentTask) error {
	box := a.st.Box()
	return a.st.Update(func(st *store.State) error {
		for _, r := range t.Repos {
			c := *r
			c.Password = box.Encrypt(c.Password)
			c.Backend.S3.SecretKey = box.Encrypt(c.Backend.S3.SecretKey)
			c.Backend.WebDAVPassword = box.Encrypt(c.Backend.WebDAVPassword)
			replaced := false
			for i, x := range st.Repos {
				if x.ID == c.ID {
					st.Repos[i] = &c
					replaced = true
				}
			}
			if !replaced {
				st.Repos = append(st.Repos, &c)
			}
		}
		if t.Job != nil {
			j := *t.Job
			j.AgentID = ""
			j.Enabled = false // the server owns the schedule
			j.Source.DBPassword = box.Encrypt(j.Source.DBPassword)
			j.Options.ArchivePassword = box.Encrypt(j.Options.ArchivePassword)
			replaced := false
			for i, x := range st.Jobs {
				if x.ID == j.ID {
					j.LastRun, j.LastStatus, j.LastFull = x.LastRun, x.LastStatus, x.LastFull
					st.Jobs[i] = &j
					replaced = true
				}
			}
			if !replaced {
				st.Jobs = append(st.Jobs, &j)
			}
		}
		return nil
	})
}

func (a *Agent) runTask(ctx context.Context, t *jobs.AgentTask) {
	fail := func(msg string) {
		a.report(ctx, jobs.AgentReport{RunID: t.RunID, Done: true, Status: "failed", Message: msg})
	}
	if err := a.upsert(t); err != nil {
		fail(err.Error())
		return
	}
	for _, r := range t.Repos {
		a.m.ForgetRepo(r.ID)
	}
	var key string
	switch t.Kind {
	case "job":
		key = t.Job.ID
		log.Printf("Starte Job «%s»", t.Job.Name)
		if err := a.m.Trigger(key, "server", t.Full); err != nil {
			fail(err.Error())
			return
		}
	case "restore":
		k, err := a.m.Restore(*t.Restore)
		if err != nil {
			fail(err.Error())
			return
		}
		key = k
		log.Printf("Starte Wiederherstellung %s", key)
	default:
		fail("unbekannter Auftrag")
		return
	}
	a.mu.Lock()
	a.active[t.RunID] = key
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.active, t.RunID)
		a.mu.Unlock()
	}()

	var lastLine string
	started := false
	for {
		time.Sleep(1500 * time.Millisecond)
		var live *jobs.Live
		for _, l := range a.m.Running() {
			if l.Run.JobID == key {
				lc := l
				live = &lc
			}
		}
		if live != nil {
			started = true
			lines := live.Log
			if lastLine != "" {
				for i := len(lines) - 1; i >= 0; i-- {
					if lines[i] == lastLine {
						lines = lines[i+1:]
						break
					}
				}
			}
			if len(lines) > 0 {
				lastLine = lines[len(lines)-1]
			}
			a.report(ctx, jobs.AgentReport{RunID: t.RunID, Phase: live.Phase, Progress: live.Progress, Log: lines})
			continue
		}
		if !started && a.m.IsBusy(key) {
			continue
		}
		runs := a.st.Runs(key, 1)
		if len(runs) == 0 {
			fail("kein Ergebnis gefunden")
			return
		}
		r := runs[0]
		// send remaining log lines from the local log file
		var rest []string
		if b, err := os.ReadFile(a.st.LogPath(r.ID)); err == nil {
			all := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
			idx := -1
			if lastLine != "" {
				for i := len(all) - 1; i >= 0; i-- {
					if all[i] == lastLine {
						idx = i
						break
					}
				}
			}
			rest = all[idx+1:]
		}
		a.report(ctx, jobs.AgentReport{RunID: t.RunID, Log: rest, Done: true, Status: r.Status, Message: r.Message, Snapshot: r.Snapshot, Stats: r.Stats})
		log.Printf("Fertig: %s (%s)", key, r.Status)
		return
	}
}
