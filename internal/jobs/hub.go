package jobs

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/swissnand/snbackup/internal/repo"
	"github.com/swissnand/snbackup/internal/secure"
	"github.com/swissnand/snbackup/internal/store"
)

// Agent jobs
//
// A job with an AgentID is not executed on the backup server. Instead it is
// queued for the agent, which polls the server over HTTPS (outbound only –
// no open port needed on the agent), executes the job locally with the same
// engine and streams phase, progress and log lines back. The schedule,
// history, retention policy and notifications stay on the backup server.

const (
	agentPickupTimeout = 30 * time.Minute
	agentSilentTimeout = 10 * time.Minute
	// AgentOnlineWindow: an agent counts as online when it polled recently.
	AgentOnlineWindow = 45 * time.Second
)

// AgentTask is sent to an agent.
type AgentTask struct {
	RunID   string              `json:"runId"`
	Kind    string              `json:"kind"` // job | restore
	Job     *store.Job          `json:"job,omitempty"`
	Repos   []*store.Repository `json:"repos"`
	Full    bool                `json:"full"`
	Restore *RestoreRequest     `json:"restore,omitempty"`
}

// AgentReport is sent by an agent while/after running a task.
type AgentReport struct {
	RunID    string        `json:"runId"`
	Phase    string        `json:"phase"`
	Progress repo.Progress `json:"progress"`
	Log      []string      `json:"log"`
	Done     bool          `json:"done"`
	Status   string        `json:"status"`
	Message  string        `json:"message"`
	Snapshot string        `json:"snapshot"`
	Stats    *repo.Stats   `json:"stats"`
}

type dispatch struct {
	run      store.Run
	job      *store.Job
	agentID  string
	liveKey  string
	full     bool
	sent     bool
	lastSeen time.Time
	cancel   bool
	done     bool
}

type hub struct {
	mu      sync.Mutex
	pending map[string][]*AgentTask // agentID → tasks
	runs    map[string]*dispatch    // runID → dispatch
	seen    map[string]time.Time    // agentID → last poll
}

func newHub() *hub {
	return &hub{pending: map[string][]*AgentTask{}, runs: map[string]*dispatch{}, seen: map[string]time.Time{}}
}

// AgentOnline reports whether an agent polled recently.
func (m *Manager) AgentOnline(id string) bool {
	m.hub.mu.Lock()
	defer m.hub.mu.Unlock()
	t, ok := m.hub.seen[id]
	return ok && time.Since(t) < AgentOnlineWindow
}

// AgentRunning returns the number of tasks currently assigned to an agent.
func (m *Manager) AgentRunning(id string) int {
	m.hub.mu.Lock()
	defer m.hub.mu.Unlock()
	n := 0
	for _, d := range m.hub.runs {
		if d.agentID == id && !d.done {
			n++
		}
	}
	return n
}

func (m *Manager) decryptedRepo(id string) (*store.Repository, error) {
	r, ok := m.st.Repo(id)
	if !ok {
		return nil, fmt.Errorf("Ziel %s nicht gefunden", id)
	}
	return r, nil
}

// dispatchJob queues a job for its agent.
func (m *Manager) dispatchJob(j *store.Job, trigger string, full bool) error {
	ag, ok := m.st.Agent(j.AgentID)
	if !ok || ag.Disabled {
		return errors.New("Agent nicht gefunden oder deaktiviert")
	}
	box := m.st.Box()
	jc := *j
	jc.Source.DBPassword, _ = box.Decrypt(j.Source.DBPassword)
	jc.Options.ArchivePassword, _ = box.Decrypt(j.Options.ArchivePassword)
	jc.Notify = store.Notify{} // notifications are sent by the backup server
	var repos []*store.Repository
	for _, id := range []string{j.RepoID, j.TargetRepo} {
		if id == "" {
			continue
		}
		r, err := m.decryptedRepo(id)
		if err != nil {
			return err
		}
		repos = append(repos, r)
	}
	run := store.Run{ID: secure.RandomToken(9), JobID: j.ID, JobName: j.Name, JobType: j.Type, Trigger: trigger, Start: time.Now(), Status: "running", Attempt: 1, AgentID: ag.ID, Agent: ag.Name}
	m.st.SaveRun(&run)
	live := &Live{Run: run, Phase: "Wartet auf Agent «" + ag.Name + "»"}
	m.mu.Lock()
	m.live[j.ID] = live
	m.mu.Unlock()
	d := &dispatch{run: run, job: j, agentID: ag.ID, liveKey: j.ID, full: full || j.Options.Mode == "full", lastSeen: time.Now()}
	m.hub.mu.Lock()
	m.hub.runs[run.ID] = d
	m.hub.pending[ag.ID] = append(m.hub.pending[ag.ID], &AgentTask{RunID: run.ID, Kind: "job", Job: &jc, Repos: repos, Full: d.full})
	m.hub.mu.Unlock()
	go m.watchDispatch(run.ID)
	return nil
}

// dispatchRestore queues a restore on an agent.
func (m *Manager) dispatchRestore(req RestoreRequest) (string, error) {
	ag, ok := m.st.Agent(req.AgentID)
	if !ok || ag.Disabled {
		return "", errors.New("Agent nicht gefunden oder deaktiviert")
	}
	r, err := m.decryptedRepo(req.RepoID)
	if err != nil {
		return "", err
	}
	key := "restore-" + secure.RandomToken(6)
	run := store.Run{ID: secure.RandomToken(9), JobID: key, JobName: "Wiederherstellung auf " + ag.Name, JobType: "restore", Trigger: "manual:" + req.User, Start: time.Now(), Status: "running", Attempt: 1, AgentID: ag.ID, Agent: ag.Name}
	m.st.SaveRun(&run)
	m.mu.Lock()
	m.live[key] = &Live{Run: run, Phase: "Wartet auf Agent «" + ag.Name + "»"}
	m.mu.Unlock()
	rq := req
	rq.AgentID = ""
	m.hub.mu.Lock()
	m.hub.runs[run.ID] = &dispatch{run: run, agentID: ag.ID, liveKey: key, lastSeen: time.Now()}
	m.hub.pending[ag.ID] = append(m.hub.pending[ag.ID], &AgentTask{RunID: run.ID, Kind: "restore", Repos: []*store.Repository{r}, Restore: &rq})
	m.hub.mu.Unlock()
	go m.watchDispatch(run.ID)
	return key, nil
}

func (m *Manager) watchDispatch(runID string) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-t.C:
		}
		m.hub.mu.Lock()
		d, ok := m.hub.runs[runID]
		if !ok || d.done {
			m.hub.mu.Unlock()
			return
		}
		var reason string
		if !d.sent && time.Since(d.run.Start) > agentPickupTimeout {
			reason = "Agent hat den Auftrag nicht abgeholt (offline?)"
			m.removePendingLocked(d.agentID, runID)
		} else if d.sent && time.Since(d.lastSeen) > agentSilentTimeout {
			reason = "Verbindung zum Agent verloren"
		}
		m.hub.mu.Unlock()
		if reason != "" {
			m.finishDispatch(runID, AgentReport{RunID: runID, Done: true, Status: "failed", Message: reason})
			return
		}
	}
}

func (m *Manager) removePendingLocked(agentID, runID string) {
	q := m.hub.pending[agentID]
	for i, t := range q {
		if t.RunID == runID {
			m.hub.pending[agentID] = append(q[:i], q[i+1:]...)
			return
		}
	}
}

// AgentPoll is called by an agent; it returns new tasks and runs to cancel.
func (m *Manager) AgentPoll(agentID string) (tasks []*AgentTask, cancel []string) {
	m.hub.mu.Lock()
	defer m.hub.mu.Unlock()
	m.hub.seen[agentID] = time.Now()
	tasks = m.hub.pending[agentID]
	delete(m.hub.pending, agentID)
	for _, t := range tasks {
		if d, ok := m.hub.runs[t.RunID]; ok {
			d.sent = true
			d.lastSeen = time.Now()
		}
	}
	for id, d := range m.hub.runs {
		if d.agentID == agentID && d.cancel && !d.done {
			cancel = append(cancel, id)
		}
	}
	if tasks == nil {
		tasks = []*AgentTask{}
	}
	return tasks, cancel
}

// AgentReport processes a progress/final report from an agent.
func (m *Manager) AgentReport(agentID string, r AgentReport) error {
	m.hub.mu.Lock()
	d, ok := m.hub.runs[r.RunID]
	if !ok || d.agentID != agentID {
		m.hub.mu.Unlock()
		return errors.New("unknown run")
	}
	d.lastSeen = time.Now()
	m.hub.seen[agentID] = time.Now()
	key := d.liveKey
	m.hub.mu.Unlock()

	m.mu.Lock()
	if l, ok := m.live[key]; ok {
		if r.Phase != "" {
			l.Phase = r.Phase
		}
		l.Progress = r.Progress
		l.Log = append(l.Log, r.Log...)
		if len(l.Log) > 300 {
			l.Log = l.Log[len(l.Log)-300:]
		}
	}
	m.mu.Unlock()
	m.appendRunLog(r.RunID, r.Log)
	if r.Done {
		m.finishDispatch(r.RunID, r)
	}
	return nil
}

func (m *Manager) appendRunLog(runID string, lines []string) {
	if len(lines) == 0 {
		return
	}
	appendLines(m.st.LogPath(runID), lines)
}

func (m *Manager) finishDispatch(runID string, r AgentReport) {
	m.hub.mu.Lock()
	d, ok := m.hub.runs[runID]
	if !ok || d.done {
		m.hub.mu.Unlock()
		return
	}
	d.done = true
	delete(m.hub.runs, runID)
	m.hub.mu.Unlock()

	run := d.run
	end := time.Now()
	run.End = &end
	run.Status = r.Status
	if run.Status == "" {
		run.Status = "failed"
	}
	run.Message = r.Message
	run.Snapshot = r.Snapshot
	run.Stats = r.Stats
	m.st.SaveRun(&run)
	m.appendRunLog(runID, []string{fmt.Sprintf("%s [INFO] Ergebnis vom Agent: %s %s", end.Format("2006-01-02 15:04:05"), run.Status, run.Message)})
	m.mu.Lock()
	delete(m.live, d.liveKey)
	m.mu.Unlock()
	if d.job == nil { // restore
		if m.Audit != nil {
			m.Audit("agent", "restore."+run.Status, run.Message)
		}
		return
	}
	_ = m.st.Update(func(st *store.State) error {
		for _, x := range st.Jobs {
			if x.ID == d.job.ID {
				t := run.Start
				x.LastRun = &t
				x.LastStatus = run.Status
				if d.full && run.Status != "failed" {
					x.LastFull = &t
				}
			}
		}
		return nil
	})
	m.notify(d.job, &run)
	m.chain(d.job, run.Status)
}

// cancelDispatch marks an agent job for cancellation.
func (m *Manager) cancelDispatch(liveKey string) bool {
	m.hub.mu.Lock()
	defer m.hub.mu.Unlock()
	for id, d := range m.hub.runs {
		if d.liveKey == liveKey && !d.done {
			if !d.sent {
				m.removePendingLocked(d.agentID, id)
				go m.finishDispatch(id, AgentReport{RunID: id, Done: true, Status: "cancelled", Message: "Abgebrochen, bevor der Agent startete"})
				return true
			}
			d.cancel = true
			return true
		}
	}
	return false
}
