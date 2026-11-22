package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/swissnand/snbackup/internal/archive"
	"github.com/swissnand/snbackup/internal/backend"
	"github.com/swissnand/snbackup/internal/notify"
	"github.com/swissnand/snbackup/internal/repo"
	"github.com/swissnand/snbackup/internal/secure"
	"github.com/swissnand/snbackup/internal/store"
)

type runCtx struct {
	m     *Manager
	job   *store.Job
	live  *Live
	logf  *os.File
	mu    sync.Mutex
	warns int
	full  bool
}

func (rc *runCtx) log(level, msg string) {
	line := fmt.Sprintf("%s [%s] %s", time.Now().Format("2006-01-02 15:04:05"), strings.ToUpper(level), msg)
	rc.mu.Lock()
	if level == "warn" || level == "error" {
		rc.warns++
	}
	if rc.logf != nil {
		rc.logf.WriteString(line + "\n")
	}
	rc.mu.Unlock()
	rc.m.mu.Lock()
	rc.live.Log = append(rc.live.Log, line)
	if len(rc.live.Log) > 300 {
		rc.live.Log = rc.live.Log[len(rc.live.Log)-300:]
	}
	rc.m.mu.Unlock()
}

func (rc *runCtx) logf2(level, f string, a ...any) { rc.log(level, fmt.Sprintf(f, a...)) }

func (rc *runCtx) phase(p string) {
	rc.m.mu.Lock()
	rc.live.Phase = p
	rc.m.mu.Unlock()
	rc.log("info", "▶ "+p)
}

func (rc *runCtx) progress(p repo.Progress) {
	rc.m.mu.Lock()
	rc.live.Progress = p
	rc.m.mu.Unlock()
}

func (m *Manager) execute(j *store.Job, trigger string, full bool) {
	run := store.Run{ID: secure.RandomToken(9), JobID: j.ID, JobName: j.Name, JobType: j.Type, Trigger: trigger, Start: time.Now(), Status: "running", Attempt: 1}
	ctx, cancel := context.WithCancel(m.ctx)
	defer cancel()
	if j.Options.TimeoutMin > 0 {
		var c2 context.CancelFunc
		ctx, c2 = context.WithTimeout(ctx, time.Duration(j.Options.TimeoutMin)*time.Minute)
		defer c2()
	}
	live := &Live{Run: run, Phase: "Start", cancel: cancel}
	m.mu.Lock()
	m.live[j.ID] = live
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.live, j.ID)
		m.mu.Unlock()
	}()
	_ = os.MkdirAll(filepath.Dir(m.st.LogPath(run.ID)), 0o700)
	lf, _ := os.OpenFile(m.st.LogPath(run.ID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if lf != nil {
		defer lf.Close()
	}
	rc := &runCtx{m: m, job: j, live: live, logf: lf, full: full || j.Options.Mode == "full"}
	m.st.SaveRun(&run)
	rc.logf2("info", "Job «%s» gestartet (Auslöser: %s, Typ: %s)", j.Name, trigger, j.Type)

	retries := j.Options.Retries
	delay := time.Duration(j.Options.RetryDelayMin) * time.Minute
	if delay == 0 {
		delay = 5 * time.Minute
	}
	var result *outcome
	var err error
	for attempt := 1; ; attempt++ {
		run.Attempt = attempt
		result, err = rc.runOnce(ctx)
		if err == nil || ctx.Err() != nil || attempt > retries {
			break
		}
		rc.logf2("warn", "Versuch %d fehlgeschlagen: %v – neuer Versuch in %s", attempt, err, delay)
		select {
		case <-ctx.Done():
		case <-time.After(delay):
		}
		if ctx.Err() != nil {
			break
		}
	}

	end := time.Now()
	run.End = &end
	switch {
	case errors.Is(ctx.Err(), context.Canceled) && m.ctx.Err() == nil:
		run.Status, run.Message = "cancelled", "Vom Benutzer abgebrochen"
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		run.Status, run.Message = "failed", "Zeitlimit überschritten"
	case err != nil:
		run.Status, run.Message = "failed", err.Error()
	case rc.warns > 0:
		run.Status, run.Message = "warning", fmt.Sprintf("Abgeschlossen mit %d Warnungen", rc.warns)
	default:
		run.Status = "success"
	}
	if result != nil {
		if run.Message == "" {
			run.Message = result.message
		}
		run.Snapshot = result.snapshot
		run.Stats = result.stats
		if result.extra != nil {
			run.Extra, _ = json.Marshal(result.extra)
		}
	}
	if run.Status == "failed" || run.Status == "cancelled" {
		rc.log("error", run.Message)
		if j.Hooks.OnFailure != "" {
			_ = rc.hook(context.Background(), "Fehler-Hook", j.Hooks.OnFailure, run.Status)
		}
	}
	rc.logf2("info", "Job beendet: %s (%s)", run.Status, end.Sub(run.Start).Round(time.Second))
	m.st.SaveRun(&run)
	_ = m.st.Update(func(st *store.State) error {
		for _, x := range st.Jobs {
			if x.ID == j.ID {
				t := run.Start
				x.LastRun = &t
				x.LastStatus = run.Status
				if rc.full && run.Status != "failed" {
					x.LastFull = &t
				}
			}
		}
		return nil
	})
	m.notify(j, &run)
	m.chain(j, run.Status)
}

type outcome struct {
	snapshot string
	stats    *repo.Stats
	message  string
	extra    any
}

func (rc *runCtx) runOnce(ctx context.Context) (*outcome, error) {
	j := rc.job
	if j.Hooks.Pre != "" {
		if err := rc.hook(ctx, "Pre-Hook", j.Hooks.Pre, "running"); err != nil {
			if j.Hooks.AbortOnPre {
				return nil, fmt.Errorf("Pre-Hook fehlgeschlagen: %w", err)
			}
			rc.logf2("warn", "Pre-Hook fehlgeschlagen (wird ignoriert): %v", err)
		}
	}
	var out *outcome
	var err error
	switch j.Type {
	case "backup":
		out, err = rc.backup(ctx)
	case "mirror":
		out, err = rc.mirror(ctx)
	case "archive":
		out, err = rc.archive(ctx)
	case "copy":
		out, err = rc.copy(ctx)
	case "verify":
		out, err = rc.verify(ctx, j.RepoID, j.Options.VerifyPercent)
	case "prune":
		out, err = rc.prune(ctx, j.RepoID, "")
	default:
		err = fmt.Errorf("unbekannter Jobtyp %q", j.Type)
	}
	if j.Hooks.Post != "" {
		status := "success"
		if err != nil {
			status = "failed"
		}
		if herr := rc.hook(context.Background(), "Post-Hook", j.Hooks.Post, status); herr != nil {
			rc.logf2("warn", "Post-Hook fehlgeschlagen: %v", herr)
		}
	}
	return out, err
}

func (rc *runCtx) withRepo(ctx context.Context, id string, fn func(r *repo.Repo) error) error {
	l := rc.m.repoLock(id)
	l.Lock()
	defer l.Unlock()
	rc.phase("Ziel öffnen und entschlüsseln")
	r, err := rc.m.OpenRepo(ctx, id)
	if err != nil {
		return err
	}
	return fn(r)
}

func (rc *runCtx) backup(ctx context.Context) (*outcome, error) {
	j := rc.job
	var out *outcome
	err := rc.withRepo(ctx, j.RepoID, func(r *repo.Repo) error {
		if j.Options.BandwidthKBs > 0 {
			r = r.WithBackend(backend.NewLimited(r.Backend(), j.Options.BandwidthKBs*1024))
			rc.logf2("info", "Upload-Limit: %d KB/s", j.Options.BandwidthKBs)
		}
		r.Compression = j.Options.Compression
		if r.Compression == "" {
			r.Compression = repo.CompressionAuto
		}
		lk, err := r.Lock(ctx, false, "backup "+j.Name)
		if err != nil {
			return err
		}
		var once sync.Once
		release := func() { once.Do(lk.Unlock) }
		defer release()

		opt := repo.BackupOptions{
			Paths: j.Source.Paths, Excludes: j.Source.Excludes, ExcludeCaches: j.Source.ExcludeCaches,
			OneFileSystem: j.Source.OneFileSystem, Full: rc.full, Differential: !rc.full && j.Options.Mode == "differential", Workers: j.Options.Workers,
			JobID: j.ID, JobName: j.Name, Kind: j.Source.Kind, Tags: j.Options.Tags,
			Progress: rc.progress, Log: rc.log,
		}
		if j.Source.ExcludeLargerMB > 0 {
			opt.ExcludeLargerThan = int64(j.Source.ExcludeLargerMB) << 20
		}
		if j.Options.Priority == "low" || j.Options.Priority == "idle" {
			opt.Workers = 1
		}
		if hr := rc.m.HostRoot; hr != "" {
			opt.ReadPath = func(p string) string { return filepath.Join(hr, p) }
		}
		cleanup, err := rc.prepareSource(ctx, &opt)
		if cleanup != nil {
			defer cleanup()
		}
		if err != nil {
			return err
		}
		rc.phase("Daten sichern")
		snap, err := r.Backup(ctx, opt)
		if err != nil {
			return err
		}
		st := snap.Stats
		rc.logf2("info", "Snapshot %s: %d Dateien, %s gesamt, %s neu, %s übertragen", snap.ID[:8], st.Files, human(st.Bytes), human(st.NewBytes), human(st.StoredBytes))
		modeName := map[string]string{"full": "Vollbackup", "incremental": "inkrementell", "differential": "differenziell"}[snap.Mode]
		out = &outcome{snapshot: snap.ID, stats: &st, message: fmt.Sprintf("%s · %d Dateien · %s neu", modeName, st.Files, human(st.NewBytes))}
		release()
		if !j.Retention.Empty() {
			rc.phase("Aufbewahrung anwenden")
			dec, err := r.Forget(ctx, j.Retention, j.ID, false)
			if err != nil {
				rc.logf2("warn", "Aufbewahrung: %v", err)
			} else {
				removed := 0
				for _, d := range dec {
					if !d.Keep {
						removed++
					}
				}
				rc.logf2("info", "Aufbewahrung: %d behalten, %d entfernt", len(dec)-removed, removed)
				if removed > 0 && j.PruneAfter {
					rc.phase("Speicher freigeben (Prune)")
					pr, err := r.Prune(ctx, false, func(s string) { rc.log("warn", s) })
					if err != nil {
						rc.logf2("warn", "Prune: %v", err)
					} else {
						rc.logf2("info", "Prune: %d Chunks gelöscht, %s freigegeben", pr.Deleted, human(pr.FreedBytes))
					}
				}
			}
		}
		if j.Options.VerifyAfter == "quick" || j.Options.VerifyAfter == "full" {
			pct := 5
			if j.Options.VerifyAfter == "full" {
				pct = 100
			}
			if j.Options.VerifyPercent > 0 && j.Options.VerifyAfter == "quick" {
				pct = j.Options.VerifyPercent
			}
			rc.phase(fmt.Sprintf("Integrität prüfen (%d%% der Daten)", pct))
			vr, err := r.Verify(ctx, pct, nil)
			if err != nil {
				rc.logf2("warn", "Prüfung: %v", err)
			} else if !vr.OK() {
				for _, p := range vr.Problems {
					rc.log("error", p)
				}
				return fmt.Errorf("Integritätsprüfung fehlgeschlagen: %d fehlend, %d beschädigt", vr.Missing, vr.Corrupt)
			} else {
				rc.logf2("info", "Prüfung OK: %d Chunks gelesen und verifiziert", vr.ReadChunks)
			}
		}
		return nil
	})
	return out, err
}

func (rc *runCtx) verify(ctx context.Context, repoID string, pct int) (*outcome, error) {
	if pct <= 0 {
		pct = 10
	}
	var out *outcome
	err := rc.withRepo(ctx, repoID, func(r *repo.Repo) error {
		rc.phase(fmt.Sprintf("Integrität prüfen (%d%%)", pct))
		vr, err := r.Verify(ctx, pct, func(s string) { rc.log("info", s) })
		if err != nil {
			return err
		}
		out = &outcome{message: fmt.Sprintf("%d Snapshots, %d Chunks, %d gelesen", vr.Snapshots, vr.Chunks, vr.ReadChunks), extra: vr}
		for _, p := range vr.Problems {
			rc.log("error", p)
		}
		if !vr.OK() {
			return fmt.Errorf("%d fehlende und %d beschädigte Chunks", vr.Missing, vr.Corrupt)
		}
		return nil
	})
	return out, err
}

func (rc *runCtx) prune(ctx context.Context, repoID, jobFilter string) (*outcome, error) {
	var out *outcome
	err := rc.withRepo(ctx, repoID, func(r *repo.Repo) error {
		if !rc.job.Retention.Empty() {
			rc.phase("Aufbewahrung anwenden")
			if _, err := r.Forget(ctx, rc.job.Retention, jobFilter, false); err != nil {
				return err
			}
		}
		rc.phase("Nicht mehr benötigte Daten löschen")
		pr, err := r.Prune(ctx, false, func(s string) { rc.log("warn", s) })
		if err != nil {
			return err
		}
		out = &outcome{message: fmt.Sprintf("%d Chunks gelöscht · %s frei", pr.Deleted, human(pr.FreedBytes)), extra: pr}
		return nil
	})
	return out, err
}

func (rc *runCtx) copy(ctx context.Context) (*outcome, error) {
	j := rc.job
	if j.TargetRepo == "" || j.TargetRepo == j.RepoID {
		return nil, errors.New("Backup-Kopie braucht ein anderes Zielrepository")
	}
	src, err := rc.m.OpenRepo(ctx, j.RepoID)
	if err != nil {
		return nil, fmt.Errorf("Quelle: %w", err)
	}
	var out *outcome
	err = rc.withRepo(ctx, j.TargetRepo, func(dst *repo.Repo) error {
		dst.Compression = repo.CompressionAuto
		lk, err := dst.Lock(ctx, false, "copy "+j.Name)
		if err != nil {
			return err
		}
		var once sync.Once
		release := func() { once.Do(lk.Unlock) }
		defer release()
		rc.phase("Snapshots kopieren (3-2-1)")
		filter := func(s *repo.Snapshot) bool { return j.CopyFilter == "" || s.JobID == j.CopyFilter }
		mapFile := filepath.Join(rc.m.stateDir("copymaps"), j.ID+".json")
		res, err := repo.CopySnapshots(ctx, src, dst, filter, mapFile, func(s string) { rc.log("info", s) })
		if err != nil {
			return err
		}
		out = &outcome{message: fmt.Sprintf("%d kopiert · %d bereits vorhanden · %s übertragen", res.Copied, res.Skipped, human(res.StoredBytes)), extra: res}
		release()
		if !j.Retention.Empty() {
			rc.phase("Aufbewahrung auf Kopie anwenden")
			if _, err := dst.Forget(ctx, j.Retention, "", false); err != nil {
				rc.logf2("warn", "Aufbewahrung: %v", err)
			} else if j.PruneAfter {
				if _, err := dst.Prune(ctx, false, nil); err != nil {
					rc.logf2("warn", "Prune: %v", err)
				}
			}
		}
		return nil
	})
	return out, err
}

func (rc *runCtx) archive(ctx context.Context) (*outcome, error) {
	j := rc.job
	if j.TargetPath == "" {
		return nil, errors.New("Archiv braucht einen Zielordner")
	}
	ex, err := repo.NewMatcher(j.Source.Excludes)
	if err != nil {
		return nil, err
	}
	pw := ""
	if j.Options.ArchiveEncrypt {
		pw, _ = rc.m.st.Box().Decrypt(j.Options.ArchivePassword)
		if len(pw) < 8 {
			return nil, errors.New("Archiv-Passwort fehlt oder ist zu kurz")
		}
	}
	name := safeName(j.Name) + "-" + time.Now().Format("20060102-150405") + ".tar.gz"
	if pw != "" {
		name += ".snarc"
	}
	out := filepath.Join(j.TargetPath, name)
	rc.phase("Archiv erstellen: " + name)
	files, bytes, err := archive.Create(out, j.Source.Paths, ex, pw, func(s string) { rc.log("warn", s) })
	if err != nil {
		return nil, err
	}
	fi, _ := os.Stat(out)
	var size int64
	if fi != nil {
		size = fi.Size()
	}
	rc.logf2("info", "Archiv %s: %d Dateien, %s → %s", name, files, human(bytes), human(size))
	if j.Options.ArchiveKeep > 0 {
		prefix := safeName(j.Name) + "-"
		entries, _ := os.ReadDir(j.TargetPath)
		var arcs []string
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), prefix) && strings.Contains(e.Name(), ".tar.gz") && !strings.HasSuffix(e.Name(), ".part") {
				arcs = append(arcs, e.Name())
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(arcs)))
		for i, a := range arcs {
			if i >= j.Options.ArchiveKeep {
				if err := os.Remove(filepath.Join(j.TargetPath, a)); err == nil {
					rc.logf2("info", "Altes Archiv gelöscht: %s", a)
				}
			}
		}
	}
	st := &repo.Stats{Files: files, Bytes: bytes, StoredBytes: size}
	return &outcome{stats: st, message: fmt.Sprintf("%s · %d Dateien · %s", name, files, human(size))}, nil
}

func safeName(s string) string {
	var b strings.Builder
	s = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "Ä", "ae", "Ö", "oe", "Ü", "ue", "ß", "ss", "é", "e", "è", "e", "à", "a").Replace(s)
	for _, c := range strings.ToLower(s) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			b.WriteRune(c)
		} else if c == ' ' {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "archiv"
	}
	return b.String()
}

func human(n int64) string {
	const u = 1024
	if n < u {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(u), 0
	for x := n / u; x >= u; x /= u {
		div *= u
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// Human formats a byte count.
func Human(n int64) string { return human(n) }

func (m *Manager) notify(j *store.Job, run *store.Run) {
	n := j.Notify
	want := (run.Status == "success" && n.OnSuccess) || (run.Status == "warning" && n.OnWarning) ||
		((run.Status == "failed" || run.Status == "cancelled") && n.OnFailure)
	if !want {
		return
	}
	set := m.st.Settings()
	emails := append(append([]string{}, n.Emails...), set.DefaultEmails...)
	hooks := append(append([]string{}, n.Webhooks...), set.DefaultWebhooks...)
	label := map[string]string{"success": "erfolgreich", "warning": "mit Warnungen", "failed": "FEHLGESCHLAGEN", "cancelled": "abgebrochen"}[run.Status]
	text := fmt.Sprintf("Job: %s\nStatus: %s\nDauer: %s\n%s", j.Name, label, run.End.Sub(run.Start).Round(time.Second), run.Message)
	msg := notify.Message{Title: fmt.Sprintf("Backup «%s» %s", j.Name, label), Text: text, Status: run.Status, Job: j.Name, Server: set.ServerName, Time: run.Start.Format("02.01.2006 15:04")}
	smtpCfg := set.SMTP
	smtpCfg.Password, _ = m.st.Box().Decrypt(smtpCfg.Password)
	go func() {
		if len(emails) > 0 {
			if err := notify.Email(smtpCfg, emails, msg); err != nil && m.Audit != nil {
				m.Audit("system", "notify.email.failed", err.Error())
			}
		}
		for _, h := range hooks {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := notify.Webhook(ctx, h, msg); err != nil && m.Audit != nil {
				m.Audit("system", "notify.webhook.failed", err.Error())
			}
			cancel()
		}
	}()
}
