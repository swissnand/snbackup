// Package store persists server configuration: repositories, jobs, users,
// API tokens, settings and the run history. Secrets are encrypted at rest.
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/swissnand/snbackup/internal/backend"
	"github.com/swissnand/snbackup/internal/repo"
	"github.com/swissnand/snbackup/internal/secure"
)

// Repository is a configured backup target.
type Repository struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Backend     backend.Config `json:"backend"`
	Password    string         `json:"password"`
	Created     time.Time      `json:"created"`
	RepoID      string         `json:"repoId,omitempty"`
}

// Source describes what a job backs up.
type Source struct {
	Kind            string   `json:"kind"` // files | postgres | mysql | command | docker
	Paths           []string `json:"paths,omitempty"`
	Excludes        []string `json:"excludes,omitempty"`
	ExcludeLargerMB int      `json:"excludeLargerMB,omitempty"`
	ExcludeCaches   bool     `json:"excludeCaches,omitempty"`
	OneFileSystem   bool     `json:"oneFileSystem,omitempty"`
	UseVSS          bool     `json:"useVSS,omitempty"` // Windows Volume Shadow Copy
	DBHost          string   `json:"dbHost,omitempty"`
	DBPort          int      `json:"dbPort,omitempty"`
	DBUser          string   `json:"dbUser,omitempty"`
	DBPassword      string   `json:"dbPassword,omitempty"`
	Databases       []string `json:"databases,omitempty"`
	AllDatabases    bool     `json:"allDatabases,omitempty"`
	DumpArgs        string   `json:"dumpArgs,omitempty"`
	DockerContainer string   `json:"dockerContainer,omitempty"` // run dump inside container
	Command         string   `json:"command,omitempty"`
	CommandName     string   `json:"commandName,omitempty"`
	DockerVolumes   []string `json:"dockerVolumes,omitempty"`
	StopContainers  []string `json:"stopContainers,omitempty"`
}

// Schedule describes when a job runs.
type Schedule struct {
	Kind        string   `json:"kind"` // manual | interval | daily | weekly | monthly | cron | after
	EveryMin    int      `json:"everyMin,omitempty"`
	Times       []string `json:"times,omitempty"`     // "HH:MM"
	Weekdays    []int    `json:"weekdays,omitempty"`  // 0=Sunday
	MonthDays   []int    `json:"monthDays,omitempty"` // 1..31, -1 = last day
	Cron        string   `json:"cron,omitempty"`
	AfterJob    string   `json:"afterJob,omitempty"`
	OnlySuccess bool     `json:"onlySuccess,omitempty"`
	WindowStart string   `json:"windowStart,omitempty"` // backup window "HH:MM"
	WindowEnd   string   `json:"windowEnd,omitempty"`
	JitterMin   int      `json:"jitterMin,omitempty"`
	CatchUp     bool     `json:"catchUp,omitempty"`   // run missed schedules at startup
	FullEvery   string   `json:"fullEvery,omitempty"` // "" | weekly:<0-6> | monthly:<1-31> – periodic active full
	PauseFrom   string   `json:"pauseFrom,omitempty"` // YYYY-MM-DD holiday/maintenance pause
	PauseUntil  string   `json:"pauseUntil,omitempty"`
}

// Options are advanced job settings.
type Options struct {
	Mode            string   `json:"mode,omitempty"` // incremental | full
	Compression     string   `json:"compression,omitempty"`
	Workers         int      `json:"workers,omitempty"`
	BandwidthKBs    int      `json:"bandwidthKBs,omitempty"`
	Retries         int      `json:"retries,omitempty"`
	RetryDelayMin   int      `json:"retryDelayMin,omitempty"`
	TimeoutMin      int      `json:"timeoutMin,omitempty"`
	Priority        string   `json:"priority,omitempty"`    // normal | low | idle
	VerifyAfter     string   `json:"verifyAfter,omitempty"` // none | quick | full
	VerifyPercent   int      `json:"verifyPercent,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	MirrorDelete    bool     `json:"mirrorDelete,omitempty"`
	MirrorTrash     bool     `json:"mirrorTrash,omitempty"`
	ArchiveEncrypt  bool     `json:"archiveEncrypt,omitempty"`
	ArchiveKeep     int      `json:"archiveKeep,omitempty"`
	ArchivePassword string   `json:"archivePassword,omitempty"`
}

// Hooks are commands executed around a job.
type Hooks struct {
	Pre        string `json:"pre,omitempty"`
	Post       string `json:"post,omitempty"`
	OnFailure  string `json:"onFailure,omitempty"`
	AbortOnPre bool   `json:"abortOnPre,omitempty"`
	TimeoutMin int    `json:"timeoutMin,omitempty"`
}

// Notify configures job notifications.
type Notify struct {
	OnSuccess bool     `json:"onSuccess"`
	OnWarning bool     `json:"onWarning"`
	OnFailure bool     `json:"onFailure"`
	Emails    []string `json:"emails,omitempty"`
	Webhooks  []string `json:"webhooks,omitempty"`
}

// Job is a backup job definition.
type Job struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Enabled     bool           `json:"enabled"`
	Type        string         `json:"type"` // backup | mirror | archive | copy | verify | prune
	Source      Source         `json:"source"`
	RepoID      string         `json:"repoId,omitempty"`
	TargetRepo  string         `json:"targetRepo,omitempty"` // copy job destination
	TargetPath  string         `json:"targetPath,omitempty"` // mirror / archive target
	CopyFilter  string         `json:"copyFilter,omitempty"` // job ID filter for copy
	Schedule    Schedule       `json:"schedule"`
	Retention   repo.Retention `json:"retention"`
	PruneAfter  bool           `json:"pruneAfter"`
	Options     Options        `json:"options"`
	Hooks       Hooks          `json:"hooks"`
	Notify      Notify         `json:"notify"`
	Created     time.Time      `json:"created"`
	Updated     time.Time      `json:"updated"`
	LastRun     *time.Time     `json:"lastRun,omitempty"`
	LastStatus  string         `json:"lastStatus,omitempty"`
	LastFull    *time.Time     `json:"lastFull,omitempty"`
}

// User is a GUI/API account.
type User struct {
	Username    string     `json:"username"`
	Role        string     `json:"role"` // admin | operator | viewer
	PassHash    string     `json:"passHash"`
	TOTPSecret  string     `json:"totpSecret,omitempty"`
	TOTPEnabled bool       `json:"totpEnabled"`
	MustChange  bool       `json:"mustChange"`
	Created     time.Time  `json:"created"`
	LastLogin   *time.Time `json:"lastLogin,omitempty"`
	Disabled    bool       `json:"disabled,omitempty"`
}

// APIToken grants API access for automation.
type APIToken struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Hash     string     `json:"hash"`
	Role     string     `json:"role"`
	Owner    string     `json:"owner"`
	Created  time.Time  `json:"created"`
	Expires  *time.Time `json:"expires,omitempty"`
	LastUsed *time.Time `json:"lastUsed,omitempty"`
}

// SMTP settings.
type SMTP struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	From     string `json:"from"`
	TLS      string `json:"tls"` // starttls | tls | none
}

// Settings are global server settings.
type Settings struct {
	ServerName        string   `json:"serverName"`
	MaxConcurrentJobs int      `json:"maxConcurrentJobs"`
	SessionMinutes    int      `json:"sessionMinutes"`
	Require2FA        bool     `json:"require2FA"`
	IPAllowlist       []string `json:"ipAllowlist,omitempty"`
	HistoryDays       int      `json:"historyDays"`
	SMTP              SMTP     `json:"smtp"`
	DefaultEmails     []string `json:"defaultEmails,omitempty"`
	DefaultWebhooks   []string `json:"defaultWebhooks,omitempty"`
	DailyReport       bool     `json:"dailyReport"`
	DailyReportTime   string   `json:"dailyReportTime,omitempty"`
	Language          string   `json:"language"`
	License           string   `json:"license,omitempty"`
}

// State is everything persisted in state.json.
type State struct {
	Repos    []*Repository `json:"repos"`
	Jobs     []*Job        `json:"jobs"`
	Users    []*User       `json:"users"`
	Tokens   []*APIToken   `json:"tokens"`
	Settings Settings      `json:"settings"`
}

// Run is one execution of a job.
type Run struct {
	ID       string          `json:"id"`
	JobID    string          `json:"jobId"`
	JobName  string          `json:"jobName"`
	JobType  string          `json:"jobType"`
	Trigger  string          `json:"trigger"` // schedule | manual | chain | api | catch-up
	Start    time.Time       `json:"start"`
	End      *time.Time      `json:"end,omitempty"`
	Status   string          `json:"status"` // running | success | warning | failed | cancelled
	Message  string          `json:"message,omitempty"`
	Snapshot string          `json:"snapshot,omitempty"`
	Stats    *repo.Stats     `json:"stats,omitempty"`
	Attempt  int             `json:"attempt"`
	Extra    json.RawMessage `json:"extra,omitempty"`
}

// Store is the thread-safe persistence layer.
type Store struct {
	mu   sync.RWMutex
	dir  string
	box  *secure.Box
	st   State
	runs []*Run
}

// Open loads (or creates) the store in dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	box, err := secure.LoadBox(filepath.Join(dir, "secret.key"))
	if err != nil {
		return nil, err
	}
	s := &Store{dir: dir, box: box}
	b, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err == nil {
		if err := json.Unmarshal(b, &s.st); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	d := &s.st.Settings
	if d.MaxConcurrentJobs == 0 {
		d.MaxConcurrentJobs = 2
	}
	if d.SessionMinutes == 0 {
		d.SessionMinutes = 30
	}
	if d.HistoryDays == 0 {
		d.HistoryDays = 90
	}
	if d.Language == "" {
		d.Language = "de"
	}
	if d.ServerName == "" {
		d.ServerName, _ = os.Hostname()
	}
	if d.SMTP.Port == 0 {
		d.SMTP.Port = 587
		d.SMTP.TLS = "starttls"
	}
	s.loadRuns()
	return s, nil
}

// Dir returns the data directory.
func (s *Store) Dir() string { return s.dir }

// Box returns the secret box.
func (s *Store) Box() *secure.Box { return s.box }

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, "state.json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, "state.json"))
}

// Update runs fn with exclusive access and persists the result.
func (s *Store) Update(fn func(st *State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(&s.st); err != nil {
		return err
	}
	return s.saveLocked()
}

// View runs fn with shared access. fn must not keep references.
func (s *Store) View(fn func(st *State)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fn(&s.st)
}

// Job returns a copy of a job.
func (s *Store) Job(id string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, j := range s.st.Jobs {
		if j.ID == id {
			c := *j
			return &c, true
		}
	}
	return nil, false
}

// Jobs returns copies of all jobs.
func (s *Store) Jobs() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Job, 0, len(s.st.Jobs))
	for _, j := range s.st.Jobs {
		c := *j
		out = append(out, &c)
	}
	return out
}

// Repo returns a copy of a repository with decrypted secrets.
func (s *Store) Repo(id string) (*Repository, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.st.Repos {
		if r.ID == id {
			c := *r
			c.Password, _ = s.box.Decrypt(c.Password)
			c.Backend.S3.SecretKey, _ = s.box.Decrypt(c.Backend.S3.SecretKey)
			c.Backend.WebDAVPassword, _ = s.box.Decrypt(c.Backend.WebDAVPassword)
			return &c, true
		}
	}
	return nil, false
}

// Repos returns copies of all repositories (secrets removed).
func (s *Store) Repos() []*Repository {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Repository, 0, len(s.st.Repos))
	for _, r := range s.st.Repos {
		c := *r
		c.Password = ""
		c.Backend.S3.SecretKey = ""
		c.Backend.WebDAVPassword = ""
		out = append(out, &c)
	}
	return out
}

// Settings returns a copy of the settings.
func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.st.Settings
}

// ---------------------------------------------------------------------------
// Run history (runs.jsonl, append-only, compacted at startup)

func (s *Store) loadRuns() {
	f, err := os.Open(filepath.Join(s.dir, "runs.jsonl"))
	if err != nil {
		return
	}
	defer f.Close()
	byID := map[string]*Run{}
	var order []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var r Run
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		if _, ok := byID[r.ID]; !ok {
			order = append(order, r.ID)
		}
		rc := r
		byID[r.ID] = &rc
	}
	cutoff := time.Now().AddDate(0, 0, -s.st.Settings.HistoryDays)
	for _, id := range order {
		r := byID[id]
		if r.Start.Before(cutoff) {
			continue
		}
		if r.Status == "running" { // crashed while running
			r.Status = "failed"
			r.Message = "Abgebrochen (Dienst wurde beendet)"
			t := r.Start
			r.End = &t
		}
		s.runs = append(s.runs, r)
	}
	s.compactRuns()
}

func (s *Store) compactRuns() {
	tmp := filepath.Join(s.dir, "runs.jsonl.tmp")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	w := bufio.NewWriter(f)
	for _, r := range s.runs {
		b, _ := json.Marshal(r)
		w.Write(b)
		w.WriteByte('\n')
	}
	w.Flush()
	f.Close()
	_ = os.Rename(tmp, filepath.Join(s.dir, "runs.jsonl"))
}

// SaveRun appends or updates a run.
func (s *Store) SaveRun(r *Run) {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	for i, x := range s.runs {
		if x.ID == r.ID {
			c := *r
			s.runs[i] = &c
			found = true
			break
		}
	}
	if !found {
		c := *r
		s.runs = append(s.runs, &c)
	}
	f, err := os.OpenFile(filepath.Join(s.dir, "runs.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		b, _ := json.Marshal(r)
		f.Write(append(b, '\n'))
		f.Close()
	}
}

// Runs returns runs (newest first), optionally filtered by job.
func (s *Store) Runs(jobID string, limit int) []*Run {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Run
	for i := len(s.runs) - 1; i >= 0; i-- {
		r := s.runs[i]
		if jobID != "" && r.JobID != jobID {
			continue
		}
		c := *r
		out = append(out, &c)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.After(out[j].Start) })
	return out
}

// LogPath returns the log file of a run.
func (s *Store) LogPath(runID string) string {
	return filepath.Join(s.dir, "logs", runID+".log")
}
