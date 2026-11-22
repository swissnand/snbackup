// Package jobs runs backup jobs: queueing, scheduling, retries, hooks,
// retention, verification and notifications.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/swissnand/snbackup/internal/backend"
	"github.com/swissnand/snbackup/internal/repo"
	"github.com/swissnand/snbackup/internal/schedule"
	"github.com/swissnand/snbackup/internal/secure"
	"github.com/swissnand/snbackup/internal/store"
)

// Live is the live state of a running job.
type Live struct {
	Run      store.Run     `json:"run"`
	Phase    string        `json:"phase"`
	Progress repo.Progress `json:"progress"`
	Log      []string      `json:"log"`
	cancel   context.CancelFunc
}

// Manager owns job execution.
type Manager struct {
	st     *store.Store
	mu     sync.Mutex
	live   map[string]*Live // by job ID
	queued map[string]string
	sem    chan struct{}
	repoMu map[string]*sync.Mutex
	repos  map[string]*cachedRepo
	next   map[string]time.Time
	ctx    context.Context
	stop   context.CancelFunc
	wg     sync.WaitGroup
	Audit  func(user, action, detail string)
	hub    *hub
	// HostRoot is set when running as a containerised agent with the host
	// filesystem mounted (e.g. /host): sources are read below it while the
	// original paths are stored.
	HostRoot string
}

type cachedRepo struct {
	r   *repo.Repo
	sig string
}

// New creates a manager.
func New(st *store.Store) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	n := st.Settings().MaxConcurrentJobs
	if n <= 0 {
		n = 2
	}
	return &Manager{st: st, live: map[string]*Live{}, queued: map[string]string{}, sem: make(chan struct{}, n),
		repoMu: map[string]*sync.Mutex{}, repos: map[string]*cachedRepo{}, next: map[string]time.Time{}, ctx: ctx, stop: cancel, hub: newHub()}
}

// OpenRepo opens (and caches) a configured repository.
func (m *Manager) OpenRepo(ctx context.Context, id string) (*repo.Repo, error) {
	r, ok := m.st.Repo(id)
	if !ok {
		return nil, errors.New("Ziel nicht gefunden")
	}
	sig := fmt.Sprintf("%v|%s", r.Backend, secure.TokenHash(r.Password))
	m.mu.Lock()
	if c, ok := m.repos[id]; ok && c.sig == sig {
		m.mu.Unlock()
		return c.r, nil
	}
	m.mu.Unlock()
	be, err := backend.Open(r.Backend)
	if err != nil {
		return nil, err
	}
	rp, err := repo.Open(ctx, be, r.Password)
	if err != nil {
		return nil, fmt.Errorf("Ziel «%s» (%s): %w", r.Name, be.Location(), err)
	}
	m.mu.Lock()
	m.repos[id] = &cachedRepo{r: rp, sig: sig}
	m.mu.Unlock()
	return rp, nil
}

// ForgetRepo drops a cached repository handle.
func (m *Manager) ForgetRepo(id string) {
	m.mu.Lock()
	delete(m.repos, id)
	m.mu.Unlock()
}

func (m *Manager) repoLock(id string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.repoMu[id]
	if !ok {
		l = &sync.Mutex{}
		m.repoMu[id] = l
	}
	return l
}

// Running returns live state of all running/queued jobs.
func (m *Manager) Running() []Live {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Live, 0, len(m.live))
	for _, l := range m.live {
		c := *l
		c.Log = append([]string(nil), l.Log...)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Run.Start.Before(out[j].Run.Start) })
	return out
}

// IsBusy reports whether a job is running or queued.
func (m *Manager) IsBusy(jobID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, a := m.live[jobID]
	_, b := m.queued[jobID]
	return a || b
}

// Trigger queues a job run.
func (m *Manager) Trigger(jobID, trigger string, full bool) error {
	j, ok := m.st.Job(jobID)
	if !ok {
		return errors.New("Job nicht gefunden")
	}
	m.mu.Lock()
	if _, busy := m.live[jobID]; busy {
		m.mu.Unlock()
		return errors.New("Job läuft bereits")
	}
	if j.AgentID != "" {
		m.mu.Unlock()
		return m.dispatchJob(j, trigger, full)
	}
	if _, q := m.queued[jobID]; q {
		m.mu.Unlock()
		return errors.New("Job ist bereits in der Warteschlange")
	}
	m.queued[jobID] = trigger
	m.mu.Unlock()
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		select {
		case m.sem <- struct{}{}:
		case <-m.ctx.Done():
			m.mu.Lock()
			delete(m.queued, jobID)
			m.mu.Unlock()
			return
		}
		defer func() { <-m.sem }()
		m.mu.Lock()
		delete(m.queued, jobID)
		m.mu.Unlock()
		m.execute(j, trigger, full)
	}()
	return nil
}

// Cancel stops a running job.
func (m *Manager) Cancel(jobID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if l, ok := m.live[jobID]; ok && l.cancel != nil {
		l.cancel()
		return true
	}
	if _, ok := m.live[jobID]; ok {
		m.mu.Unlock()
		defer m.mu.Lock()
		return m.cancelDispatch(jobID)
	}
	return false
}

func appendLines(path string, lines []string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	for _, l := range lines {
		f.WriteString(l + "\n")
	}
}

// NextRun returns the cached next scheduled run of a job.
func (m *Manager) NextRun(jobID string) time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.next[jobID]
}

// Reschedule recomputes next run times (after job changes).
func (m *Manager) Reschedule() {
	now := time.Now()
	jobs := m.st.Jobs()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next = map[string]time.Time{}
	for _, j := range jobs {
		if t := schedule.Next(j, now); !t.IsZero() {
			m.next[j.ID] = t
		}
	}
}

// Start runs the scheduler loop.
func (m *Manager) Start() {
	m.Reschedule()
	// Catch-up of missed runs (server was off at scheduled time).
	now := time.Now()
	for _, j := range m.st.Jobs() {
		if j.Enabled && j.Schedule.CatchUp && j.LastRun != nil {
			jc := *j
			if n := schedule.Next(&jc, *j.LastRun); !n.IsZero() && n.Before(now.Add(-5*time.Minute)) {
				_ = m.Trigger(j.ID, "catch-up", false)
			}
		}
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-m.ctx.Done():
				return
			case now := <-t.C:
				m.tick(now)
			}
		}
	}()
}

func (m *Manager) tick(now time.Time) {
	m.mu.Lock()
	var due []string
	for id, t := range m.next {
		if !t.IsZero() && !now.Before(t) {
			due = append(due, id)
		}
	}
	m.mu.Unlock()
	for _, id := range due {
		j, ok := m.st.Job(id)
		if !ok {
			continue
		}
		full := false
		if fe := j.Schedule.FullEvery; fe != "" {
			full = isFullDay(fe, now, j.LastFull)
		}
		_ = m.Trigger(id, "schedule", full)
		jc := *j
		t := now
		jc.LastRun = &t
		m.mu.Lock()
		if n := schedule.Next(&jc, now); !n.IsZero() {
			m.next[id] = n
		} else {
			delete(m.next, id)
		}
		m.mu.Unlock()
	}
}

func isFullDay(spec string, now time.Time, lastFull *time.Time) bool {
	if lastFull != nil && lastFull.Format("2006-01-02") == now.Format("2006-01-02") {
		return false
	}
	var d int
	switch {
	case strings.HasPrefix(spec, "weekly:"):
		fmt.Sscanf(spec[7:], "%d", &d)
		return int(now.Weekday()) == d
	case strings.HasPrefix(spec, "monthly:"):
		fmt.Sscanf(spec[8:], "%d", &d)
		return now.Day() == d
	}
	return false
}

// Stop cancels all jobs and waits.
func (m *Manager) Stop() {
	m.stop()
	m.mu.Lock()
	for _, l := range m.live {
		if l.cancel != nil {
			l.cancel()
		}
	}
	m.mu.Unlock()
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
	}
}

func (m *Manager) chain(finished *store.Job, status string) {
	for _, j := range m.st.Jobs() {
		if !j.Enabled || j.Schedule.Kind != "after" || j.Schedule.AfterJob != finished.ID {
			continue
		}
		if j.Schedule.OnlySuccess && status != "success" {
			continue
		}
		_ = m.Trigger(j.ID, "chain", false)
	}
}

func (m *Manager) stateDir(sub string) string {
	d := filepath.Join(m.st.Dir(), sub)
	_ = os.MkdirAll(d, 0o700)
	return d
}
