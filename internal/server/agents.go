package server

import (
	"crypto/subtle"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/swissnand/snbackup/internal/agent"
	"github.com/swissnand/snbackup/internal/jobs"
	"github.com/swissnand/snbackup/internal/secure"
	"github.com/swissnand/snbackup/internal/store"
)

func (s *Server) agentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("POST /api/agent/enroll", s.handleAgentEnroll)
	mux.HandleFunc("POST /api/agent/poll", s.agentAuth(s.handleAgentPoll))
	mux.HandleFunc("POST /api/agent/report", s.agentAuth(s.handleAgentReport))

	mux.HandleFunc("GET /api/agents", s.auth("viewer", s.handleAgents))
	mux.HandleFunc("POST /api/agents/enroll-token", s.auth("admin", s.handleEnrollToken))
	mux.HandleFunc("PUT /api/agents/{id}", s.auth("admin", s.handleAgentUpdate))
	mux.HandleFunc("DELETE /api/agents/{id}", s.auth("admin", s.handleAgentDelete))
}

// fingerprintOf reads a PEM certificate and returns its pinning fingerprint.
func fingerprintOf(certFile string) string {
	b, err := os.ReadFile(certFile)
	if err != nil {
		return ""
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return ""
	}
	if _, err := x509.ParseCertificate(blk.Bytes); err != nil {
		return ""
	}
	return agent.CertFingerprint(blk.Bytes)
}

func (s *Server) agentAuth(h func(http.ResponseWriter, *http.Request, *store.Agent)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v := strings.TrimPrefix(r.Header.Get("Authorization"), "Agent ")
		id, secret, ok := strings.Cut(v, ":")
		if !ok || id == "" {
			jsonErr(w, 401, "Agent nicht authentifiziert")
			return
		}
		a, found := s.st.Agent(id)
		if !found || subtle.ConstantTimeCompare([]byte(a.SecretHash), []byte(secure.TokenHash(secret))) != 1 {
			s.limiter.failed("ip:" + clientIP(r))
			jsonErr(w, 401, "Agent unbekannt oder entfernt")
			return
		}
		if a.Disabled {
			jsonErr(w, 403, "Agent deaktiviert")
			return
		}
		h(w, r, a)
	}
}

func (s *Server) handleAgentEnroll(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.limiter.blocked("enroll:" + ip) {
		jsonErr(w, 429, "Zu viele Versuche")
		return
	}
	var req struct{ Token, Name, Hostname, OS, Arch, Version string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "Ungültige Anfrage")
		return
	}
	hash := secure.TokenHash(strings.TrimSpace(req.Token))
	secret := secure.RandomToken(32)
	var ag *store.Agent
	err := s.st.Update(func(st *store.State) error {
		var tok *store.EnrollToken
		for i, t := range st.Enroll {
			if subtle.ConstantTimeCompare([]byte(t.Hash), []byte(hash)) == 1 {
				tok = t
				if time.Now().After(t.Expires) {
					return errors.New("Registrierungs-Token abgelaufen")
				}
				t.Uses++
				if !t.Reuse {
					st.Enroll = append(st.Enroll[:i], st.Enroll[i+1:]...)
				}
				break
			}
		}
		if tok == nil {
			return errors.New("Registrierungs-Token ungültig")
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = req.Hostname
		}
		if len(name) > 64 {
			name = name[:64]
		}
		now := time.Now()
		ag = &store.Agent{ID: "ag_" + secure.RandomToken(9), Name: name, Hostname: req.Hostname, OS: req.OS, Arch: req.Arch, Version: req.Version, IP: ip, SecretHash: secure.TokenHash(secret), Enrolled: now, LastSeen: &now}
		st.Agents = append(st.Agents, ag)
		return nil
	})
	if err != nil {
		s.limiter.failed("enroll:" + ip)
		s.audit("agent", ip, "agent.enroll_failed", err.Error())
		jsonErr(w, 403, err.Error())
		return
	}
	s.audit("agent", ip, "agent.enrolled", ag.Name+" ("+ag.OS+")")
	jsonOK(w, map[string]string{"agentId": ag.ID, "secret": secret})
}

func (s *Server) handleAgentPoll(w http.ResponseWriter, r *http.Request, a *store.Agent) {
	var info struct{ Hostname, OS, Arch, Version string }
	_ = json.NewDecoder(r.Body).Decode(&info)
	if a.LastSeen == nil || time.Since(*a.LastSeen) > time.Minute || a.Version != info.Version || a.IP != clientIP(r) {
		_ = s.st.Update(func(st *store.State) error {
			for _, x := range st.Agents {
				if x.ID == a.ID {
					now := time.Now()
					x.LastSeen = &now
					x.IP = clientIP(r)
					if info.Version != "" {
						x.Version, x.OS, x.Arch, x.Hostname = info.Version, info.OS, info.Arch, info.Hostname
					}
				}
			}
			return nil
		})
	}
	tasks, cancel := s.m.AgentPoll(a.ID)
	jsonOK(w, map[string]any{"tasks": tasks, "cancel": cancel})
}

func (s *Server) handleAgentReport(w http.ResponseWriter, r *http.Request, a *store.Agent) {
	var rep jobs.AgentReport
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		jsonErr(w, 400, "Ungültige Anfrage")
		return
	}
	if err := s.m.AgentReport(a.ID, rep); err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request, p *principal) {
	jobCount := map[string]int{}
	for _, j := range s.st.Jobs() {
		if j.AgentID != "" {
			jobCount[j.AgentID]++
		}
	}
	var out []map[string]any
	for _, a := range s.st.Agents() {
		a.SecretHash = ""
		out = append(out, map[string]any{"agent": a, "online": s.m.AgentOnline(a.ID), "running": s.m.AgentRunning(a.ID), "jobs": jobCount[a.ID]})
	}
	jsonOK(w, map[string]any{"agents": out, "fingerprint": s.fingerprint})
}

func (s *Server) handleEnrollToken(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Name  string `json:"name"`
		Hours int    `json:"hours"`
		Reuse bool   `json:"reuse"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Hours <= 0 || req.Hours > 24*30 {
		req.Hours = 24
	}
	tok := "snbe_" + secure.RandomToken(24)
	_ = s.st.Update(func(st *store.State) error {
		// drop expired tokens
		var keep []*store.EnrollToken
		for _, t := range st.Enroll {
			if time.Now().Before(t.Expires) {
				keep = append(keep, t)
			}
		}
		st.Enroll = append(keep, &store.EnrollToken{Hash: secure.TokenHash(tok), Name: req.Name, Created: time.Now(), Expires: time.Now().Add(time.Duration(req.Hours) * time.Hour), Reuse: req.Reuse})
		return nil
	})
	s.audit(p.User, clientIP(r), "agent.token_created", fmt.Sprintf("%s, %dh, mehrfach=%v", req.Name, req.Hours, req.Reuse))
	jsonOK(w, map[string]any{"token": tok, "fingerprint": s.fingerprint, "expires": time.Now().Add(time.Duration(req.Hours) * time.Hour)})
}

func (s *Server) handleAgentUpdate(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Name     string   `json:"name"`
		Disabled bool     `json:"disabled"`
		Tags     []string `json:"tags"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	id := r.PathValue("id")
	err := s.st.Update(func(st *store.State) error {
		for _, a := range st.Agents {
			if a.ID == id {
				if strings.TrimSpace(req.Name) != "" {
					a.Name = strings.TrimSpace(req.Name)
				}
				a.Disabled, a.Tags = req.Disabled, req.Tags
				return nil
			}
		}
		return errors.New("Agent nicht gefunden")
	})
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	s.audit(p.User, clientIP(r), "agent.update", id)
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleAgentDelete(w http.ResponseWriter, r *http.Request, p *principal) {
	id := r.PathValue("id")
	for _, j := range s.st.Jobs() {
		if j.AgentID == id {
			jsonErr(w, 409, "Agent wird noch von Job «"+j.Name+"» verwendet")
			return
		}
	}
	var name string
	_ = s.st.Update(func(st *store.State) error {
		for i, a := range st.Agents {
			if a.ID == id {
				name = a.Name
				st.Agents = append(st.Agents[:i], st.Agents[i+1:]...)
				break
			}
		}
		return nil
	})
	s.audit(p.User, clientIP(r), "agent.delete", name)
	jsonOK(w, map[string]bool{"ok": true})
}
