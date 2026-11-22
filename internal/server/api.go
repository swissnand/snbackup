package server

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/swissnand/snbackup/internal/backend"
	"github.com/swissnand/snbackup/internal/jobs"
	"github.com/swissnand/snbackup/internal/license"
	"github.com/swissnand/snbackup/internal/notify"
	"github.com/swissnand/snbackup/internal/repo"
	"github.com/swissnand/snbackup/internal/schedule"
	"github.com/swissnand/snbackup/internal/secure"
	"github.com/swissnand/snbackup/internal/store"
	"github.com/swissnand/snbackup/internal/version"
)

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.auth("viewer", s.handleLogout))
	mux.HandleFunc("GET /api/me", s.auth("viewer", s.handleMe))
	mux.HandleFunc("POST /api/me/password", s.auth("viewer", s.handlePassword))
	mux.HandleFunc("POST /api/me/totp/setup", s.auth("viewer", s.handleTOTPSetup))
	mux.HandleFunc("POST /api/me/totp/enable", s.auth("viewer", s.handleTOTPEnable))
	mux.HandleFunc("POST /api/me/totp/disable", s.auth("viewer", s.handleTOTPDisable))

	mux.HandleFunc("GET /api/dashboard", s.auth("viewer", s.handleDashboard))
	mux.HandleFunc("GET /api/system", s.auth("viewer", s.handleSystem))
	mux.HandleFunc("GET /api/activity", s.auth("viewer", s.handleActivity))
	mux.HandleFunc("GET /api/schedule", s.auth("viewer", s.handleSchedule))
	mux.HandleFunc("POST /api/schedule/preview", s.auth("viewer", s.handleSchedulePreview))

	mux.HandleFunc("GET /api/jobs", s.auth("viewer", s.handleJobs))
	mux.HandleFunc("POST /api/jobs", s.auth("admin", s.handleJobSave))
	mux.HandleFunc("GET /api/jobs/{id}", s.auth("viewer", s.handleJobGet))
	mux.HandleFunc("PUT /api/jobs/{id}", s.auth("admin", s.handleJobSave))
	mux.HandleFunc("DELETE /api/jobs/{id}", s.auth("admin", s.handleJobDelete))
	mux.HandleFunc("POST /api/jobs/{id}/run", s.auth("operator", s.handleJobRun))
	mux.HandleFunc("POST /api/jobs/{id}/stop", s.auth("operator", s.handleJobStop))
	mux.HandleFunc("POST /api/jobs/{id}/toggle", s.auth("operator", s.handleJobToggle))
	mux.HandleFunc("POST /api/jobs/{id}/clone", s.auth("admin", s.handleJobClone))

	mux.HandleFunc("GET /api/repos", s.auth("viewer", s.handleRepos))
	mux.HandleFunc("POST /api/repos", s.auth("admin", s.handleRepoCreate))
	mux.HandleFunc("PUT /api/repos/{id}", s.auth("admin", s.handleRepoUpdate))
	mux.HandleFunc("DELETE /api/repos/{id}", s.auth("admin", s.handleRepoDelete))
	mux.HandleFunc("POST /api/repos/test", s.auth("admin", s.handleRepoTest))
	mux.HandleFunc("POST /api/repos/{id}/refresh", s.auth("viewer", s.handleRepoRefresh))
	mux.HandleFunc("POST /api/repos/{id}/verify", s.auth("operator", s.handleRepoVerify))
	mux.HandleFunc("POST /api/repos/{id}/prune", s.auth("admin", s.handleRepoPrune))
	mux.HandleFunc("POST /api/repos/{id}/unlock", s.auth("admin", s.handleRepoUnlock))
	mux.HandleFunc("POST /api/repos/{id}/retention-preview", s.auth("viewer", s.handleRetentionPreview))
	mux.HandleFunc("GET /api/repos/{id}/snapshots", s.auth("viewer", s.handleSnapshots))
	mux.HandleFunc("DELETE /api/repos/{id}/snapshots/{sid}", s.auth("admin", s.handleSnapshotDelete))
	mux.HandleFunc("GET /api/repos/{id}/snapshots/{sid}/ls", s.auth("viewer", s.handleSnapshotLs))
	mux.HandleFunc("GET /api/repos/{id}/snapshots/{sid}/download", s.auth("operator", s.handleSnapshotDownload))
	mux.HandleFunc("POST /api/restore", s.auth("operator", s.handleRestore))

	mux.HandleFunc("GET /api/runs", s.auth("viewer", s.handleRuns))
	mux.HandleFunc("GET /api/runs/{id}/log", s.auth("viewer", s.handleRunLog))

	mux.HandleFunc("GET /api/settings", s.auth("admin", s.handleSettings))
	mux.HandleFunc("PUT /api/settings", s.auth("admin", s.handleSettingsSave))
	mux.HandleFunc("POST /api/settings/test-notify", s.auth("admin", s.handleTestNotify))
	mux.HandleFunc("GET /api/users", s.auth("admin", s.handleUsers))
	mux.HandleFunc("POST /api/users", s.auth("admin", s.handleUserSave))
	mux.HandleFunc("DELETE /api/users/{name}", s.auth("admin", s.handleUserDelete))
	mux.HandleFunc("GET /api/tokens", s.auth("admin", s.handleTokens))
	mux.HandleFunc("POST /api/tokens", s.auth("admin", s.handleTokenCreate))
	mux.HandleFunc("DELETE /api/tokens/{id}", s.auth("admin", s.handleTokenDelete))
	mux.HandleFunc("GET /api/audit", s.auth("admin", s.handleAudit))
	mux.HandleFunc("GET /api/license", s.auth("viewer", s.handleLicense))
	mux.HandleFunc("PUT /api/license", s.auth("admin", s.handleLicenseSave))
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	return dec.Decode(v)
}

// ---------------------------------------------------------------------------
// Me / password / 2FA

func (s *Server) user(name string) *store.User {
	var u *store.User
	s.st.View(func(st *store.State) {
		for _, x := range st.Users {
			if x.Username == name {
				c := *x
				u = &c
			}
		}
	})
	return u
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, p *principal) {
	out := map[string]any{"username": p.User, "role": p.Role, "token": p.Token, "version": version.Version, "server": s.st.Settings().ServerName, "license": license.Evaluate(s.st.Settings().License)}
	if p.sess != nil {
		out["csrf"] = p.sess.CSRF
	}
	if u := s.user(p.User); u != nil {
		out["mustChange"] = u.MustChange
		out["totpEnabled"] = u.TOTPEnabled
		out["require2FA"] = s.st.Settings().Require2FA
	}
	jsonOK(w, out)
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct{ Current, New string }
	if decode(r, &req) != nil {
		jsonErr(w, 400, "Ungültige Anfrage")
		return
	}
	u := s.user(p.User)
	if u == nil || !secure.CheckPassword(u.PassHash, req.Current) {
		jsonErr(w, 400, "Aktuelles Passwort ist falsch")
		return
	}
	if err := secure.PasswordPolicy(req.New); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if req.New == req.Current {
		jsonErr(w, 400, "Das neue Passwort muss sich unterscheiden")
		return
	}
	h, _ := secure.HashPassword(req.New)
	_ = s.st.Update(func(st *store.State) error {
		for _, x := range st.Users {
			if x.Username == p.User {
				x.PassHash, x.MustChange = h, false
			}
		}
		return nil
	})
	_ = os.Remove(s.st.Dir() + string(os.PathSeparator) + "initial-admin-password")
	s.audit(p.User, clientIP(r), "user.password_changed", "")
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleTOTPSetup(w http.ResponseWriter, r *http.Request, p *principal) {
	if p.Token {
		jsonErr(w, 403, "nicht mit API-Token")
		return
	}
	secret := secure.NewTOTPSecret()
	_ = s.st.Update(func(st *store.State) error {
		for _, x := range st.Users {
			if x.Username == p.User && !x.TOTPEnabled {
				x.TOTPSecret = s.st.Box().Encrypt(secret)
			}
		}
		return nil
	})
	jsonOK(w, map[string]string{"secret": secret, "uri": secure.TOTPURI(secret, p.User)})
}

func (s *Server) handleTOTPEnable(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct{ Code string }
	_ = decode(r, &req)
	u := s.user(p.User)
	if u == nil || u.TOTPSecret == "" {
		jsonErr(w, 400, "Zuerst einrichten")
		return
	}
	secret, _ := s.st.Box().Decrypt(u.TOTPSecret)
	if !secure.ValidateTOTP(secret, req.Code, time.Now()) {
		jsonErr(w, 400, "Code ungültig – Uhrzeit des Geräts prüfen")
		return
	}
	_ = s.st.Update(func(st *store.State) error {
		for _, x := range st.Users {
			if x.Username == p.User {
				x.TOTPEnabled = true
			}
		}
		return nil
	})
	s.audit(p.User, clientIP(r), "user.2fa_enabled", "")
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleTOTPDisable(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct{ Password string }
	_ = decode(r, &req)
	u := s.user(p.User)
	if u == nil || !secure.CheckPassword(u.PassHash, req.Password) {
		jsonErr(w, 400, "Passwort falsch")
		return
	}
	_ = s.st.Update(func(st *store.State) error {
		for _, x := range st.Users {
			if x.Username == p.User {
				x.TOTPEnabled, x.TOTPSecret = false, ""
			}
		}
		return nil
	})
	s.audit(p.User, clientIP(r), "user.2fa_disabled", "")
	jsonOK(w, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// Dashboard

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request, p *principal) {
	jobsList := s.st.Jobs()
	runs := s.st.Runs("", 0)
	now := time.Now()
	type day struct {
		Date    string `json:"date"`
		Success int    `json:"success"`
		Warning int    `json:"warning"`
		Failed  int    `json:"failed"`
		Bytes   int64  `json:"bytes"`
	}
	days := make([]day, 14)
	idx := map[string]int{}
	for i := 0; i < 14; i++ {
		d := now.AddDate(0, 0, i-13).Format("2006-01-02")
		days[i].Date = d
		idx[d] = i
	}
	var ok7, total7 int
	var transferred7 int64
	for _, rn := range runs {
		if rn.Status == "running" {
			continue
		}
		if i, found := idx[rn.Start.Format("2006-01-02")]; found {
			switch rn.Status {
			case "success":
				days[i].Success++
			case "warning":
				days[i].Warning++
			default:
				days[i].Failed++
			}
			if rn.Stats != nil {
				days[i].Bytes += rn.Stats.NewBytes
			}
		}
		if now.Sub(rn.Start) <= 7*24*time.Hour && rn.JobType != "restore" {
			total7++
			if rn.Status == "success" || rn.Status == "warning" {
				ok7++
			}
			if rn.Stats != nil {
				transferred7 += rn.Stats.StoredBytes
			}
		}
	}
	var enabled, failing int
	type upcoming struct {
		JobID string    `json:"jobId"`
		Name  string    `json:"name"`
		Type  string    `json:"type"`
		Time  time.Time `json:"time"`
	}
	var next []upcoming
	for _, j := range jobsList {
		if j.Enabled {
			enabled++
		}
		if j.LastStatus == "failed" {
			failing++
		}
		if t := s.m.NextRun(j.ID); !t.IsZero() {
			next = append(next, upcoming{j.ID, j.Name, j.Type, t})
		}
	}
	sort.Slice(next, func(i, k int) bool { return next[i].Time.Before(next[k].Time) })
	if len(next) > 6 {
		next = next[:6]
	}
	var stored, logical int64
	var repos []map[string]any
	for _, rp := range s.st.Repos() {
		u, _ := s.getUsage(rp.ID)
		stored += u.Bytes
		logical += u.Logical
		repos = append(repos, map[string]any{"id": rp.ID, "name": rp.Name, "kind": rp.Backend.Kind, "usage": u})
	}
	rate := 100.0
	if total7 > 0 {
		rate = float64(ok7) * 100 / float64(total7)
	}
	recent := runs
	if len(recent) > 8 {
		recent = recent[:8]
	}
	jsonOK(w, map[string]any{
		"jobs": len(jobsList), "enabled": enabled, "failing": failing,
		"successRate": rate, "runs7": total7, "transferred7": transferred7,
		"stored": stored, "logical": logical, "days": days, "next": next,
		"repos": repos, "running": s.m.Running(), "recent": recent,
		"license": license.Evaluate(s.st.Settings().License),
	})
}

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request, p *principal) {
	host, _ := os.Hostname()
	jsonOK(w, map[string]any{"version": version.Version, "commit": version.Commit, "os": runtime.GOOS, "arch": runtime.GOARCH, "go": runtime.Version(),
		"hostname": host, "dataDir": s.st.Dir(), "cpus": runtime.NumCPU(), "sessions": s.sessions.count(), "time": time.Now()})
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request, p *principal) {
	jsonOK(w, s.m.Running())
}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request, p *principal) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 62 {
		days = 7
	}
	start := time.Now().Truncate(24 * time.Hour)
	if q := r.URL.Query().Get("from"); q != "" {
		if t, err := time.ParseInLocation("2006-01-02", q, time.Local); err == nil {
			start = t
		}
	}
	type ev struct {
		JobID  string    `json:"jobId"`
		Name   string    `json:"name"`
		Type   string    `json:"type"`
		Time   time.Time `json:"time"`
		Source string    `json:"source"`
	}
	var out []ev
	for _, j := range s.st.Jobs() {
		for _, t := range schedule.Upcoming(j, time.Now(), time.Until(start.AddDate(0, 0, days)), 200) {
			if t.Before(start) {
				continue
			}
			out = append(out, ev{j.ID, j.Name, j.Type, t, j.Source.Kind})
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Time.Before(out[k].Time) })
	jsonOK(w, out)
}

func (s *Server) handleSchedulePreview(w http.ResponseWriter, r *http.Request, p *principal) {
	var sc store.Schedule
	if decode(r, &sc) != nil {
		jsonErr(w, 400, "Ungültige Anfrage")
		return
	}
	if err := schedule.Validate(sc); err != nil {
		jsonOK(w, map[string]any{"error": err.Error(), "text": schedule.Describe(sc)})
		return
	}
	j := &store.Job{ID: "preview", Enabled: true, Schedule: sc}
	jsonOK(w, map[string]any{"times": schedule.Upcoming(j, time.Now(), 62*24*time.Hour, 6), "text": schedule.Describe(sc)})
}

// ---------------------------------------------------------------------------
// Jobs

type jobView struct {
	*store.Job
	NextRun      *time.Time `json:"nextRun,omitempty"`
	ScheduleText string     `json:"scheduleText"`
	Busy         bool       `json:"busy"`
	RepoName     string     `json:"repoName,omitempty"`
	AgentName    string     `json:"agentName,omitempty"`
	AgentOnline  bool       `json:"agentOnline,omitempty"`
}

func maskJob(j *store.Job) *store.Job {
	c := *j
	if c.Source.DBPassword != "" {
		c.Source.DBPassword = "********"
	}
	if c.Options.ArchivePassword != "" {
		c.Options.ArchivePassword = "********"
	}
	return &c
}

func (s *Server) view(j *store.Job) jobView {
	v := jobView{Job: maskJob(j), ScheduleText: schedule.Describe(j.Schedule), Busy: s.m.IsBusy(j.ID)}
	if t := s.m.NextRun(j.ID); !t.IsZero() {
		v.NextRun = &t
	}
	if r, ok := s.st.Repo(j.RepoID); ok {
		v.RepoName = r.Name
	}
	if j.AgentID != "" {
		if a, ok := s.st.Agent(j.AgentID); ok {
			v.AgentName = a.Name
			v.AgentOnline = s.m.AgentOnline(a.ID)
		}
	}
	return v
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request, p *principal) {
	var out []jobView
	for _, j := range s.st.Jobs() {
		out = append(out, s.view(j))
	}
	sort.Slice(out, func(i, k int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[k].Name) })
	jsonOK(w, out)
}

func (s *Server) handleJobGet(w http.ResponseWriter, r *http.Request, p *principal) {
	j, ok := s.st.Job(r.PathValue("id"))
	if !ok {
		jsonErr(w, 404, "Job nicht gefunden")
		return
	}
	jsonOK(w, map[string]any{"job": s.view(j), "runs": s.st.Runs(j.ID, 30)})
}

var validJobTypes = map[string]bool{"backup": true, "mirror": true, "archive": true, "copy": true, "verify": true, "prune": true}
var validSources = map[string]bool{"files": true, "postgres": true, "mysql": true, "command": true, "docker": true, "proxmox": true, "hyperv": true, "image": true, "sysimage": true}

func (s *Server) validateJob(j *store.Job) error {
	j.Name = strings.TrimSpace(j.Name)
	if j.Name == "" || len(j.Name) > 80 {
		return errors.New("Name fehlt oder ist zu lang")
	}
	if !validJobTypes[j.Type] {
		return errors.New("Ungültiger Jobtyp")
	}
	if j.AgentID != "" {
		if _, ok := s.st.Agent(j.AgentID); !ok {
			return errors.New("Agent nicht gefunden")
		}
	}
	if j.Type == "backup" && !validSources[j.Source.Kind] {
		return errors.New("Ungültige Quelle")
	}
	needsRepo := j.Type == "backup" || j.Type == "copy" || j.Type == "verify" || j.Type == "prune"
	if needsRepo {
		if _, ok := s.st.Repo(j.RepoID); !ok {
			return errors.New("Bitte ein gültiges Backup-Ziel wählen")
		}
	}
	if j.Type == "copy" {
		if _, ok := s.st.Repo(j.TargetRepo); !ok || j.TargetRepo == j.RepoID {
			return errors.New("Bitte ein zweites, anderes Ziel für die Kopie wählen")
		}
	}
	if (j.Type == "mirror" || j.Type == "archive") && strings.TrimSpace(j.TargetPath) == "" {
		return errors.New("Zielordner fehlt")
	}
	if (j.Type == "mirror" || j.Type == "archive" || (j.Type == "backup" && (j.Source.Kind == "files" || j.Source.Kind == ""))) && len(j.Source.Paths) == 0 {
		return errors.New("Mindestens einen Quellpfad angeben")
	}
	if _, err := repo.NewMatcher(j.Source.Excludes); err != nil {
		return err
	}
	if err := schedule.Validate(j.Schedule); err != nil {
		return err
	}
	if j.Options.Compression != "" && j.Options.Compression != "off" && j.Options.Compression != "fast" && j.Options.Compression != "auto" && j.Options.Compression != "max" {
		return errors.New("Ungültige Kompression")
	}
	if j.Options.Workers > 32 {
		j.Options.Workers = 32
	}
	if j.Options.Retries > 10 {
		j.Options.Retries = 10
	}
	for _, h := range j.Notify.Webhooks {
		if !strings.HasPrefix(h, "https://") {
			return errors.New("Webhooks müssen mit https:// beginnen")
		}
	}
	return nil
}

func (s *Server) handleJobSave(w http.ResponseWriter, r *http.Request, p *principal) {
	var j store.Job
	if err := decode(r, &j); err != nil {
		jsonErr(w, 400, "Ungültige Anfrage: "+err.Error())
		return
	}
	id := r.PathValue("id")
	if err := s.validateJob(&j); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	box := s.st.Box()
	var saved *store.Job
	err := s.st.Update(func(st *store.State) error {
		if id == "" {
			j.ID = secure.RandomToken(8)
			j.Created = time.Now()
			j.Updated = j.Created
			j.Source.DBPassword = box.Encrypt(j.Source.DBPassword)
			j.Options.ArchivePassword = box.Encrypt(j.Options.ArchivePassword)
			st.Jobs = append(st.Jobs, &j)
			saved = &j
			return nil
		}
		for i, x := range st.Jobs {
			if x.ID == id {
				j.ID, j.Created, j.Updated = x.ID, x.Created, time.Now()
				j.LastRun, j.LastStatus, j.LastFull = x.LastRun, x.LastStatus, x.LastFull
				if j.Source.DBPassword == "********" || j.Source.DBPassword == "" {
					j.Source.DBPassword = x.Source.DBPassword
				} else {
					j.Source.DBPassword = box.Encrypt(j.Source.DBPassword)
				}
				if j.Options.ArchivePassword == "********" || j.Options.ArchivePassword == "" {
					j.Options.ArchivePassword = x.Options.ArchivePassword
				} else {
					j.Options.ArchivePassword = box.Encrypt(j.Options.ArchivePassword)
				}
				st.Jobs[i] = &j
				saved = &j
				return nil
			}
		}
		return errors.New("Job nicht gefunden")
	})
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	s.m.Reschedule()
	s.audit(p.User, clientIP(r), map[bool]string{true: "job.create", false: "job.update"}[id == ""], saved.Name)
	jsonOK(w, s.view(saved))
}

func (s *Server) handleJobDelete(w http.ResponseWriter, r *http.Request, p *principal) {
	id := r.PathValue("id")
	var name string
	err := s.st.Update(func(st *store.State) error {
		for i, x := range st.Jobs {
			if x.ID == id {
				name = x.Name
				st.Jobs = append(st.Jobs[:i], st.Jobs[i+1:]...)
				return nil
			}
		}
		return errors.New("Job nicht gefunden")
	})
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	s.m.Reschedule()
	s.audit(p.User, clientIP(r), "job.delete", name)
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleJobRun(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct{ Full bool }
	_ = decode(r, &req)
	id := r.PathValue("id")
	if err := s.m.Trigger(id, "manual", req.Full); err != nil {
		jsonErr(w, 409, err.Error())
		return
	}
	j, _ := s.st.Job(id)
	s.audit(p.User, clientIP(r), "job.run", j.Name)
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleJobStop(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.m.Cancel(r.PathValue("id")) {
		jsonErr(w, 409, "Job läuft nicht")
		return
	}
	s.audit(p.User, clientIP(r), "job.stop", r.PathValue("id"))
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleJobToggle(w http.ResponseWriter, r *http.Request, p *principal) {
	id := r.PathValue("id")
	var state bool
	err := s.st.Update(func(st *store.State) error {
		for _, x := range st.Jobs {
			if x.ID == id {
				x.Enabled = !x.Enabled
				state = x.Enabled
				return nil
			}
		}
		return errors.New("Job nicht gefunden")
	})
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	s.m.Reschedule()
	s.audit(p.User, clientIP(r), "job.toggle", fmt.Sprintf("%s → %v", id, state))
	jsonOK(w, map[string]bool{"enabled": state})
}

func (s *Server) handleJobClone(w http.ResponseWriter, r *http.Request, p *principal) {
	src, ok := s.st.Job(r.PathValue("id"))
	if !ok {
		jsonErr(w, 404, "Job nicht gefunden")
		return
	}
	c := *src
	c.ID = secure.RandomToken(8)
	c.Name = src.Name + " (Kopie)"
	c.Enabled = false
	c.Created, c.Updated = time.Now(), time.Now()
	c.LastRun, c.LastStatus, c.LastFull = nil, "", nil
	_ = s.st.Update(func(st *store.State) error { st.Jobs = append(st.Jobs, &c); return nil })
	s.audit(p.User, clientIP(r), "job.clone", c.Name)
	jsonOK(w, s.view(&c))
}

// ---------------------------------------------------------------------------
// Repositories

type repoReq struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Backend     backend.Config `json:"backend"`
	Password    string         `json:"password"`
	Mode        string         `json:"mode"` // init | connect
}

func (s *Server) handleRepos(w http.ResponseWriter, r *http.Request, p *principal) {
	var out []map[string]any
	for _, rp := range s.st.Repos() {
		u, _ := s.getUsage(rp.ID)
		out = append(out, map[string]any{"repo": rp, "usage": u, "location": location(rp.Backend)})
	}
	jsonOK(w, out)
}

func location(b backend.Config) string {
	switch b.Kind {
	case "s3":
		return "s3://" + b.S3.Bucket + "/" + b.S3.Prefix
	case "webdav":
		return b.WebDAVURL
	case "rclone":
		return b.RcloneRemote
	}
	return b.Path
}

func openOrInit(ctx context.Context, req repoReq) (*repo.Repo, error) {
	be, err := backend.Open(req.Backend)
	if err != nil {
		return nil, err
	}
	if req.Mode == "init" {
		return repo.Init(ctx, be, req.Password)
	}
	return repo.Open(ctx, be, req.Password)
}

func (s *Server) handleRepoTest(w http.ResponseWriter, r *http.Request, p *principal) {
	var req repoReq
	if decode(r, &req) != nil {
		jsonErr(w, 400, "Ungültige Anfrage")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	be, err := backend.Open(req.Backend)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	probe := "locks/.snb-write-test-" + secure.RandomToken(6)
	if err := be.Save(ctx, probe, []byte("ok")); err != nil {
		jsonErr(w, 400, "Schreibtest fehlgeschlagen: "+err.Error())
		return
	}
	_ = be.Delete(ctx, probe)
	exists, _ := be.Exists(ctx, "config")
	jsonOK(w, map[string]any{"ok": true, "repositoryExists": exists, "location": be.Location()})
}

func (s *Server) handleRepoCreate(w http.ResponseWriter, r *http.Request, p *principal) {
	var req repoReq
	if decode(r, &req) != nil {
		jsonErr(w, 400, "Ungültige Anfrage")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		jsonErr(w, 400, "Name fehlt")
		return
	}
	if req.Mode == "init" && len(req.Password) < 12 {
		jsonErr(w, 400, "Das Verschlüsselungspasswort muss mindestens 12 Zeichen haben")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	rp, err := openOrInit(ctx, req)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	box := s.st.Box()
	b := req.Backend
	b.S3.SecretKey = box.Encrypt(b.S3.SecretKey)
	b.WebDAVPassword = box.Encrypt(b.WebDAVPassword)
	nr := &store.Repository{ID: secure.RandomToken(8), Name: req.Name, Description: req.Description, Backend: b, Password: box.Encrypt(req.Password), Created: time.Now(), RepoID: rp.ID()}
	_ = s.st.Update(func(st *store.State) error { st.Repos = append(st.Repos, nr); return nil })
	go s.refreshRepoUsage(context.Background(), nr.ID)
	s.audit(p.User, clientIP(r), "repo."+req.Mode, req.Name+" "+location(req.Backend))
	jsonOK(w, map[string]string{"id": nr.ID})
}

func (s *Server) handleRepoUpdate(w http.ResponseWriter, r *http.Request, p *principal) {
	id := r.PathValue("id")
	var req repoReq
	if decode(r, &req) != nil {
		jsonErr(w, 400, "Ungültige Anfrage")
		return
	}
	cur, ok := s.st.Repo(id)
	if !ok {
		jsonErr(w, 404, "Ziel nicht gefunden")
		return
	}
	nb := req.Backend
	if nb.S3.SecretKey == "" {
		nb.S3.SecretKey = cur.Backend.S3.SecretKey
	}
	if nb.WebDAVPassword == "" {
		nb.WebDAVPassword = cur.Backend.WebDAVPassword
	}
	pw := req.Password
	if pw == "" {
		pw = cur.Password
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if _, err := openOrInit(ctx, repoReq{Backend: nb, Password: pw, Mode: "connect"}); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	box := s.st.Box()
	_ = s.st.Update(func(st *store.State) error {
		for _, x := range st.Repos {
			if x.ID == id {
				if req.Name != "" {
					x.Name = req.Name
				}
				x.Description = req.Description
				nb.S3.SecretKey = box.Encrypt(nb.S3.SecretKey)
				nb.WebDAVPassword = box.Encrypt(nb.WebDAVPassword)
				x.Backend = nb
				x.Password = box.Encrypt(pw)
			}
		}
		return nil
	})
	s.m.ForgetRepo(id)
	s.audit(p.User, clientIP(r), "repo.update", req.Name)
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleRepoDelete(w http.ResponseWriter, r *http.Request, p *principal) {
	id := r.PathValue("id")
	for _, j := range s.st.Jobs() {
		if j.RepoID == id || j.TargetRepo == id {
			jsonErr(w, 409, "Ziel wird noch von Job «"+j.Name+"» verwendet")
			return
		}
	}
	var name string
	_ = s.st.Update(func(st *store.State) error {
		for i, x := range st.Repos {
			if x.ID == id {
				name = x.Name
				st.Repos = append(st.Repos[:i], st.Repos[i+1:]...)
				break
			}
		}
		return nil
	})
	s.m.ForgetRepo(id)
	s.audit(p.User, clientIP(r), "repo.remove", name+" (Daten bleiben erhalten)")
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleRepoRefresh(w http.ResponseWriter, r *http.Request, p *principal) {
	s.refreshRepoUsage(r.Context(), r.PathValue("id"))
	u, _ := s.getUsage(r.PathValue("id"))
	jsonOK(w, u)
}

func (s *Server) handleRepoVerify(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct{ Percent int }
	_ = decode(r, &req)
	rp, ok := s.st.Repo(r.PathValue("id"))
	if !ok {
		jsonErr(w, 404, "Ziel nicht gefunden")
		return
	}
	id := s.m.RunAdhoc(&store.Job{Name: "Prüfung " + rp.Name, Type: "verify", RepoID: rp.ID, Options: store.Options{VerifyPercent: req.Percent}}, "manual")
	s.audit(p.User, clientIP(r), "repo.verify", rp.Name)
	jsonOK(w, map[string]string{"id": id})
}

func (s *Server) handleRepoPrune(w http.ResponseWriter, r *http.Request, p *principal) {
	rp, ok := s.st.Repo(r.PathValue("id"))
	if !ok {
		jsonErr(w, 404, "Ziel nicht gefunden")
		return
	}
	id := s.m.RunAdhoc(&store.Job{Name: "Bereinigung " + rp.Name, Type: "prune", RepoID: rp.ID}, "manual")
	s.audit(p.User, clientIP(r), "repo.prune", rp.Name)
	jsonOK(w, map[string]string{"id": id})
}

func (s *Server) handleRepoUnlock(w http.ResponseWriter, r *http.Request, p *principal) {
	rp, err := s.m.OpenRepo(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	n, err := rp.RemoveAllLocks(r.Context())
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	s.audit(p.User, clientIP(r), "repo.unlock", fmt.Sprint(n))
	jsonOK(w, map[string]int{"removed": n})
}

func (s *Server) handleRetentionPreview(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Policy repo.Retention `json:"policy"`
		JobID  string         `json:"jobId"`
	}
	_ = decode(r, &req)
	rp, err := s.m.OpenRepo(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	dec, err := rp.Forget(r.Context(), req.Policy, req.JobID, true)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, dec)
}

func (s *Server) handleSnapshots(w http.ResponseWriter, r *http.Request, p *principal) {
	rp, err := s.m.OpenRepo(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	snaps, err := rp.Snapshots(r.Context())
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if job := r.URL.Query().Get("job"); job != "" {
		var f []*repo.Snapshot
		for _, sn := range snaps {
			if sn.JobID == job {
				f = append(f, sn)
			}
		}
		snaps = f
	}
	for _, sn := range snaps {
		sn.Tree = nil
	}
	jsonOK(w, snaps)
}

func (s *Server) handleSnapshotDelete(w http.ResponseWriter, r *http.Request, p *principal) {
	rp, err := s.m.OpenRepo(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if err := rp.DeleteSnapshot(r.Context(), r.PathValue("sid")); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	s.audit(p.User, clientIP(r), "snapshot.delete", r.PathValue("sid"))
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Server) tree(ctx context.Context, repoID, sid string) (*repo.Repo, *repo.Snapshot, []*repo.Node, error) {
	rp, err := s.m.OpenRepo(ctx, repoID)
	if err != nil {
		return nil, nil, nil, err
	}
	snap, err := rp.LoadSnapshot(ctx, sid)
	if err != nil {
		return nil, nil, nil, err
	}
	key := repoID + "/" + snap.ID
	if nodes, ok := s.trees.get(key); ok {
		return rp, snap, nodes, nil
	}
	nodes, err := rp.LoadTree(ctx, snap)
	if err != nil {
		return nil, nil, nil, err
	}
	s.trees.put(key, nodes)
	return rp, snap, nodes, nil
}

func (s *Server) handleSnapshotLs(w http.ResponseWriter, r *http.Request, p *principal) {
	_, snap, nodes, err := s.tree(r.Context(), r.PathValue("id"), r.PathValue("sid"))
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	dir := strings.Trim(r.URL.Query().Get("path"), "/")
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	type entry struct {
		Path  string `json:"path"`
		Name  string `json:"name"`
		Type  string `json:"type"`
		Size  int64  `json:"size"`
		MTime int64  `json:"mtime"`
		Mode  uint32 `json:"mode"`
	}
	var list []*repo.Node
	if q != "" {
		for _, n := range nodes {
			if strings.Contains(strings.ToLower(path.Base(n.Path)), q) {
				list = append(list, n)
				if len(list) >= 500 {
					break
				}
			}
		}
	} else {
		list = repo.ListDir(nodes, dir)
	}
	out := make([]entry, 0, len(list))
	for _, n := range list {
		out = append(out, entry{n.Path, path.Base(n.Path), n.Type, n.Size, n.MTime / 1e6, n.Mode})
	}
	sort.SliceStable(out, func(i, k int) bool {
		if (out[i].Type == "d") != (out[k].Type == "d") {
			return out[i].Type == "d"
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[k].Name)
	})
	jsonOK(w, map[string]any{"snapshot": snap.ID, "path": dir, "entries": out, "os": snap.OS})
}

func (s *Server) handleSnapshotDownload(w http.ResponseWriter, r *http.Request, p *principal) {
	rp, _, nodes, err := s.tree(r.Context(), r.PathValue("id"), r.PathValue("sid"))
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	want := strings.Trim(r.URL.Query().Get("path"), "/")
	s.audit(p.User, clientIP(r), "snapshot.download", want)
	for _, n := range nodes {
		if n.Path == want && n.Type == "f" {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(n.Path)))
			w.Header().Set("Content-Length", fmt.Sprint(n.Size))
			_ = rp.WriteFile(r.Context(), n, w)
			return
		}
	}
	// Directory → stream as ZIP.
	name := path.Base(want)
	if want == "" {
		name = "snapshot"
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name+".zip"))
	zw := zip.NewWriter(w)
	for _, n := range nodes {
		if n.Type != "f" || (want != "" && !strings.HasPrefix(n.Path, want+"/")) {
			continue
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(n.Path, want), "/")
		fh := &zip.FileHeader{Name: rel, Method: zip.Deflate, Modified: time.Unix(0, n.MTime)}
		fw, err := zw.CreateHeader(fh)
		if err != nil {
			return
		}
		if err := rp.WriteFile(r.Context(), n, fw); err != nil {
			return
		}
	}
	_ = zw.Close()
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request, p *principal) {
	var req jobs.RestoreRequest
	if decode(r, &req) != nil {
		jsonErr(w, 400, "Ungültige Anfrage")
		return
	}
	if req.Target == "" && !p.can("admin") {
		jsonErr(w, 403, "Wiederherstellung an den Originalort nur für Administratoren")
		return
	}
	req.User = p.User
	id, err := s.m.Restore(req)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	target := req.Target
	if target == "" {
		target = "(Originalort)"
	}
	s.audit(p.User, clientIP(r), "restore.start", fmt.Sprintf("Snapshot %s → %s (%d Pfade)", req.SnapshotID, target, len(req.Include)))
	jsonOK(w, map[string]string{"id": id})
}

// ---------------------------------------------------------------------------
// Runs

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request, p *principal) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	runs := s.st.Runs(r.URL.Query().Get("job"), 0)
	status := r.URL.Query().Get("status")
	var out []*store.Run
	for _, rn := range runs {
		if status != "" && rn.Status != status {
			continue
		}
		out = append(out, rn)
		if len(out) >= limit {
			break
		}
	}
	jsonOK(w, out)
}

func (s *Server) handleRunLog(w http.ResponseWriter, r *http.Request, p *principal) {
	id := r.PathValue("id")
	if strings.ContainsAny(id, `/\.`) {
		jsonErr(w, 400, "ungültige ID")
		return
	}
	b, err := os.ReadFile(s.st.LogPath(id))
	if err != nil {
		jsonErr(w, 404, "Kein Protokoll vorhanden")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(b)
}

// ---------------------------------------------------------------------------
// Settings, users, tokens, audit, license

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request, p *principal) {
	set := s.st.Settings()
	has := set.SMTP.Password != ""
	set.SMTP.Password = ""
	set.License = ""
	jsonOK(w, map[string]any{"settings": set, "smtpHasPassword": has})
}

func (s *Server) handleSettingsSave(w http.ResponseWriter, r *http.Request, p *principal) {
	var in store.Settings
	if err := decode(r, &in); err != nil {
		jsonErr(w, 400, "Ungültige Anfrage")
		return
	}
	if in.MaxConcurrentJobs < 1 || in.MaxConcurrentJobs > 16 {
		in.MaxConcurrentJobs = 2
	}
	if in.SessionMinutes < 5 || in.SessionMinutes > 720 {
		in.SessionMinutes = 30
	}
	for _, e := range in.IPAllowlist {
		if !validIPEntry(e) {
			jsonErr(w, 400, "Ungültiger IP-Eintrag: "+e)
			return
		}
	}
	_ = s.st.Update(func(st *store.State) error {
		old := st.Settings
		if in.SMTP.Password == "" {
			in.SMTP.Password = old.SMTP.Password
		} else {
			in.SMTP.Password = s.st.Box().Encrypt(in.SMTP.Password)
		}
		in.License = old.License
		st.Settings = in
		return nil
	})
	s.audit(p.User, clientIP(r), "settings.update", "")
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleTestNotify(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Email   string `json:"email"`
		Webhook string `json:"webhook"`
	}
	_ = decode(r, &req)
	set := s.st.Settings()
	msg := notify.Message{Title: "Testnachricht von SWISSNAND Backup", Text: "Wenn du diese Nachricht siehst, funktionieren die Benachrichtigungen.", Status: "success", Server: set.ServerName, Time: time.Now().Format("02.01.2006 15:04")}
	if req.Email != "" {
		c := set.SMTP
		c.Password, _ = s.st.Box().Decrypt(c.Password)
		if err := notify.Email(c, []string{req.Email}, msg); err != nil {
			jsonErr(w, 400, "E-Mail: "+err.Error())
			return
		}
	}
	if req.Webhook != "" {
		if err := notify.Webhook(r.Context(), req.Webhook, msg); err != nil {
			jsonErr(w, 400, "Webhook: "+err.Error())
			return
		}
	}
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request, p *principal) {
	var out []map[string]any
	s.st.View(func(st *store.State) {
		for _, u := range st.Users {
			out = append(out, map[string]any{"username": u.Username, "role": u.Role, "totpEnabled": u.TOTPEnabled, "created": u.Created, "lastLogin": u.LastLogin, "disabled": u.Disabled, "mustChange": u.MustChange})
		}
	})
	jsonOK(w, out)
}

func (s *Server) handleUserSave(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Username string `json:"username"`
		Role     string `json:"role"`
		Password string `json:"password"`
		Disabled bool   `json:"disabled"`
		Reset2FA bool   `json:"reset2fa"`
	}
	if decode(r, &req) != nil {
		jsonErr(w, 400, "Ungültige Anfrage")
		return
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	if req.Username == "" || len(req.Username) > 40 || strings.ContainsAny(req.Username, " /\\<>\"'") {
		jsonErr(w, 400, "Ungültiger Benutzername")
		return
	}
	if roleLevel[req.Role] == 0 {
		jsonErr(w, 400, "Ungültige Rolle")
		return
	}
	var hash string
	if req.Password != "" {
		if err := secure.PasswordPolicy(req.Password); err != nil {
			jsonErr(w, 400, err.Error())
			return
		}
		hash, _ = secure.HashPassword(req.Password)
	}
	err := s.st.Update(func(st *store.State) error {
		admins := 0
		for _, u := range st.Users {
			if u.Role == "admin" && !u.Disabled && u.Username != req.Username {
				admins++
			}
		}
		if (req.Role != "admin" || req.Disabled) && admins == 0 {
			return errors.New("Es muss mindestens ein aktiver Administrator bestehen bleiben")
		}
		for _, u := range st.Users {
			if u.Username == req.Username {
				u.Role, u.Disabled = req.Role, req.Disabled
				if hash != "" {
					u.PassHash, u.MustChange = hash, true
				}
				if req.Reset2FA {
					u.TOTPEnabled, u.TOTPSecret = false, ""
				}
				return nil
			}
		}
		if hash == "" {
			return errors.New("Für neue Benutzer ist ein Passwort nötig")
		}
		st.Users = append(st.Users, &store.User{Username: req.Username, Role: req.Role, PassHash: hash, MustChange: true, Created: time.Now(), Disabled: req.Disabled})
		return nil
	})
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if req.Disabled || req.Password != "" {
		s.sessions.deleteUser(req.Username)
	}
	s.audit(p.User, clientIP(r), "user.save", req.Username+" ("+req.Role+")")
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleUserDelete(w http.ResponseWriter, r *http.Request, p *principal) {
	name := r.PathValue("name")
	if name == p.User {
		jsonErr(w, 400, "Du kannst dich nicht selbst löschen")
		return
	}
	err := s.st.Update(func(st *store.State) error {
		admins := 0
		for _, u := range st.Users {
			if u.Role == "admin" && !u.Disabled && u.Username != name {
				admins++
			}
		}
		if admins == 0 {
			return errors.New("Der letzte Administrator kann nicht gelöscht werden")
		}
		for i, u := range st.Users {
			if u.Username == name {
				st.Users = append(st.Users[:i], st.Users[i+1:]...)
				return nil
			}
		}
		return errors.New("Benutzer nicht gefunden")
	})
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	s.sessions.deleteUser(name)
	s.audit(p.User, clientIP(r), "user.delete", name)
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request, p *principal) {
	var out []store.APIToken
	s.st.View(func(st *store.State) {
		for _, t := range st.Tokens {
			c := *t
			c.Hash = ""
			out = append(out, c)
		}
	})
	jsonOK(w, out)
}

func (s *Server) handleTokenCreate(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Name string `json:"name"`
		Role string `json:"role"`
		Days int    `json:"days"`
	}
	_ = decode(r, &req)
	if strings.TrimSpace(req.Name) == "" || roleLevel[req.Role] == 0 {
		jsonErr(w, 400, "Name und Rolle angeben")
		return
	}
	tok := "snb_" + secure.RandomToken(32)
	t := &store.APIToken{ID: secure.RandomToken(6), Name: req.Name, Hash: secure.TokenHash(tok), Role: req.Role, Owner: p.User, Created: time.Now()}
	if req.Days > 0 {
		e := time.Now().AddDate(0, 0, req.Days)
		t.Expires = &e
	}
	_ = s.st.Update(func(st *store.State) error { st.Tokens = append(st.Tokens, t); return nil })
	s.audit(p.User, clientIP(r), "token.create", req.Name)
	jsonOK(w, map[string]string{"token": tok, "id": t.ID})
}

func (s *Server) handleTokenDelete(w http.ResponseWriter, r *http.Request, p *principal) {
	id := r.PathValue("id")
	_ = s.st.Update(func(st *store.State) error {
		for i, t := range st.Tokens {
			if t.ID == id {
				st.Tokens = append(st.Tokens[:i], st.Tokens[i+1:]...)
				break
			}
		}
		return nil
	})
	s.audit(p.User, clientIP(r), "token.delete", id)
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request, p *principal) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 2000 {
		limit = 300
	}
	jsonOK(w, s.readAudit(limit))
}

func (s *Server) handleLicense(w http.ResponseWriter, r *http.Request, p *principal) {
	jsonOK(w, license.Evaluate(s.st.Settings().License))
}

func (s *Server) handleLicenseSave(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct{ Key string }
	_ = decode(r, &req)
	if req.Key != "" {
		if _, err := license.Verify(req.Key); err != nil {
			jsonErr(w, 400, err.Error())
			return
		}
	}
	_ = s.st.Update(func(st *store.State) error { st.Settings.License = strings.TrimSpace(req.Key); return nil })
	info := license.Evaluate(req.Key)
	s.audit(p.User, clientIP(r), "license.update", info.Edition+" "+info.Customer)
	jsonOK(w, info)
}
