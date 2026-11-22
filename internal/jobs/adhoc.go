package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/swissnand/snbackup/internal/repo"
	"github.com/swissnand/snbackup/internal/secure"
	"github.com/swissnand/snbackup/internal/store"
)

// RunAdhoc executes a transient job (not stored), e.g. a manual
// verification started from the GUI.
func (m *Manager) RunAdhoc(j *store.Job, trigger string) string {
	if j.ID == "" {
		j.ID = "adhoc-" + secure.RandomToken(6)
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.sem <- struct{}{}
		defer func() { <-m.sem }()
		m.execute(j, trigger, false)
	}()
	return j.ID
}

// RestoreRequest describes a restore started from GUI/API/CLI.
type RestoreRequest struct {
	RepoID     string   `json:"repoId"`
	SnapshotID string   `json:"snapshotId"`
	Include    []string `json:"include"`
	Target     string   `json:"target"` // empty = original location
	Overwrite  string   `json:"overwrite"`
	Verify     bool     `json:"verify"`
	Owner      bool     `json:"restoreOwner"`
	AgentID    string   `json:"agentId,omitempty"` // restore on this agent
	User       string   `json:"-"`
}

// Restore runs a restore in the background and records it in the history.
func (m *Manager) Restore(req RestoreRequest) (string, error) {
	if req.SnapshotID == "" {
		return "", errors.New("Snapshot fehlt")
	}
	if req.AgentID != "" {
		return m.dispatchRestore(req)
	}
	key := "restore-" + secure.RandomToken(6)
	run := store.Run{ID: secure.RandomToken(9), JobID: key, JobName: "Wiederherstellung " + req.SnapshotID[:min(8, len(req.SnapshotID))], JobType: "restore", Trigger: "manual:" + req.User, Start: time.Now(), Status: "running", Attempt: 1}
	ctx, cancel := context.WithCancel(m.ctx)
	live := &Live{Run: run, Phase: "Wiederherstellung vorbereiten", cancel: cancel}
	m.mu.Lock()
	m.live[key] = live
	m.mu.Unlock()
	m.st.SaveRun(&run)
	j := &store.Job{ID: key, Name: run.JobName, Type: "restore"}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer cancel()
		defer func() { m.mu.Lock(); delete(m.live, key); m.mu.Unlock() }()
		rc := &runCtx{m: m, job: j, live: live}
		_ = os.MkdirAll(filepath.Dir(m.st.LogPath(run.ID)), 0o700)
		if lf, err := os.OpenFile(m.st.LogPath(run.ID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			rc.logf = lf
			defer lf.Close()
		}
		err := rc.withRepo(ctx, req.RepoID, func(r *repo.Repo) error {
			lk, err := r.Lock(ctx, false, "restore")
			if err != nil {
				return err
			}
			defer lk.Unlock()
			s, err := r.LoadSnapshot(ctx, req.SnapshotID)
			if err != nil {
				return err
			}
			target := req.Target
			if target == "" {
				rc.log("warn", "Wiederherstellung an den ursprünglichen Ort")
			}
			if m.HostRoot != "" {
				target = filepath.Join(m.HostRoot, target)
			}
			rc.phase("Dateien wiederherstellen")
			res, err := r.Restore(ctx, s, repo.RestoreOptions{Target: target, Include: req.Include, Overwrite: req.Overwrite, Verify: req.Verify, RestoreOwner: req.Owner, Progress: rc.progress, Log: rc.log})
			if err != nil {
				return err
			}
			run.Message = fmt.Sprintf("%s · %s · %d übersprungen · %d Fehler", files(res.Files), human(res.Bytes), res.Skipped, res.Errors)
			run.Stats = &repo.Stats{Files: res.Files, Bytes: res.Bytes, Errors: res.Errors}
			if res.Errors > 0 {
				return fmt.Errorf("%d Fehler bei der Wiederherstellung", res.Errors)
			}
			return nil
		})
		end := time.Now()
		run.End = &end
		switch {
		case err != nil && ctx.Err() != nil:
			run.Status, run.Message = "cancelled", "Abgebrochen"
		case err != nil:
			run.Status = "failed"
			if run.Message == "" {
				run.Message = err.Error()
			}
		case rc.warns > 1:
			run.Status = "warning"
		default:
			run.Status = "success"
		}
		m.st.SaveRun(&run)
		if m.Audit != nil {
			m.Audit(req.User, "restore."+run.Status, run.Message)
		}
	}()
	return key, nil
}
