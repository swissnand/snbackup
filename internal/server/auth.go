package server

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/swissnand/snbackup/internal/secure"
	"github.com/swissnand/snbackup/internal/store"
)

const cookieName = "snb_session"

type session struct {
	User       string
	Role       string
	CSRF       string
	Created    time.Time
	LastSeen   time.Time
	IP         string
	Pending2FA bool
}

type sessionStore struct {
	mu sync.Mutex
	m  map[string]*session
}

func newSessionStore() *sessionStore { return &sessionStore{m: map[string]*session{}} }

func (s *sessionStore) create(user, role, ip string, pending bool) (string, *session) {
	tok := secure.RandomToken(32)
	ss := &session{User: user, Role: role, CSRF: secure.RandomToken(24), Created: time.Now(), LastSeen: time.Now(), IP: ip, Pending2FA: pending}
	s.mu.Lock()
	s.m[secure.TokenHash(tok)] = ss
	s.mu.Unlock()
	return tok, ss
}

func (s *sessionStore) get(tok string, idle time.Duration) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := secure.TokenHash(tok)
	ss, ok := s.m[k]
	if !ok {
		return nil
	}
	if time.Since(ss.LastSeen) > idle || time.Since(ss.Created) > 12*time.Hour {
		delete(s.m, k)
		return nil
	}
	ss.LastSeen = time.Now()
	return ss
}

func (s *sessionStore) delete(tok string) {
	s.mu.Lock()
	delete(s.m, secure.TokenHash(tok))
	s.mu.Unlock()
}

func (s *sessionStore) deleteUser(user string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.m {
		if v.User == user {
			delete(s.m, k)
		}
	}
}

func (s *sessionStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}

// loginLimiter blocks brute force: 5 failures per IP or user within
// 15 minutes lock that IP/user for 15 minutes.
type loginLimiter struct {
	mu   sync.Mutex
	fail map[string][]time.Time
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{fail: map[string][]time.Time{}} }

func (l *loginLimiter) blocked(keys ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		var recent []time.Time
		for _, t := range l.fail[k] {
			if time.Since(t) < 15*time.Minute {
				recent = append(recent, t)
			}
		}
		l.fail[k] = recent
		if len(recent) >= 5 {
			return true
		}
	}
	return false
}

func (l *loginLimiter) failed(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		l.fail[k] = append(l.fail[k], time.Now())
	}
}

func (l *loginLimiter) reset(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		delete(l.fail, k)
	}
}

// ---------------------------------------------------------------------------

type principal struct {
	User   string
	Role   string
	Token  bool
	sess   *session
	cookie string
}

var roleLevel = map[string]int{"viewer": 1, "operator": 2, "admin": 3}

func (p *principal) can(role string) bool { return roleLevel[p.Role] >= roleLevel[role] }

type handler func(w http.ResponseWriter, r *http.Request, p *principal)

// auth wraps a handler with authentication, CSRF protection and RBAC.
func (s *Server) auth(role string, h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := s.principal(r)
		if p == nil {
			jsonErr(w, http.StatusUnauthorized, "Nicht angemeldet")
			return
		}
		if !p.Token && r.Method != http.MethodGet && r.Method != http.MethodHead {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(p.sess.CSRF)) != 1 {
				jsonErr(w, http.StatusForbidden, "CSRF-Token ungültig")
				return
			}
		}
		if !p.Token {
			var u *store.User
			s.st.View(func(st *store.State) {
				for _, x := range st.Users {
					if x.Username == p.User {
						c := *x
						u = &c
					}
				}
			})
			if u == nil || u.Disabled {
				s.sessions.delete(p.cookie)
				jsonErr(w, http.StatusUnauthorized, "Konto deaktiviert")
				return
			}
			p.Role = u.Role
			allowedPre := strings.HasPrefix(r.URL.Path, "/api/me") || r.URL.Path == "/api/logout"
			if u.MustChange && !allowedPre {
				jsonErr(w, http.StatusForbidden, "password_change_required")
				return
			}
			if s.st.Settings().Require2FA && !u.TOTPEnabled && !allowedPre {
				jsonErr(w, http.StatusForbidden, "totp_setup_required")
				return
			}
		}
		if !p.can(role) {
			jsonErr(w, http.StatusForbidden, "Keine Berechtigung (Rolle "+role+" nötig)")
			return
		}
		h(w, r, p)
	}
}

func (s *Server) principal(r *http.Request) *principal {
	if ah := r.Header.Get("Authorization"); strings.HasPrefix(ah, "Bearer ") {
		tok := strings.TrimPrefix(ah, "Bearer ")
		hash := secure.TokenHash(tok)
		var p *principal
		_ = s.st.Update(func(st *store.State) error {
			for _, t := range st.Tokens {
				if subtle.ConstantTimeCompare([]byte(t.Hash), []byte(hash)) == 1 {
					if t.Expires != nil && time.Now().After(*t.Expires) {
						return nil
					}
					now := time.Now()
					t.LastUsed = &now
					p = &principal{User: t.Owner + " (Token " + t.Name + ")", Role: t.Role, Token: true}
				}
			}
			return nil
		})
		return p
	}
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	ss := s.sessions.get(c.Value, time.Duration(s.st.Settings().SessionMinutes)*time.Minute)
	if ss == nil || ss.Pending2FA {
		return nil
	}
	return &principal{User: ss.User, Role: ss.Role, sess: ss, cookie: c.Value}
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, tok string) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: tok, Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode})
}

// POST /api/login {username, password, totp}
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TOTP     string `json:"totp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "Ungültige Anfrage")
		return
	}
	ip := clientIP(r)
	req.Username = strings.TrimSpace(strings.ToLower(req.Username))
	if s.limiter.blocked("ip:"+ip, "user:"+req.Username) {
		s.audit(req.Username, ip, "login.blocked", "zu viele Fehlversuche")
		jsonErr(w, http.StatusTooManyRequests, "Zu viele Fehlversuche – bitte 15 Minuten warten")
		return
	}
	var u *store.User
	s.st.View(func(st *store.State) {
		for _, x := range st.Users {
			if x.Username == req.Username {
				c := *x
				u = &c
			}
		}
	})
	// Always hash to keep timing independent of user existence.
	ok := false
	if u != nil && !u.Disabled {
		ok = secure.CheckPassword(u.PassHash, req.Password)
	} else {
		secure.CheckPassword("pbkdf2-sha256$600000$00000000000000000000000000000000$0000000000000000000000000000000000000000000000000000000000000000", req.Password)
	}
	if !ok {
		s.limiter.failed("ip:"+ip, "user:"+req.Username)
		s.audit(req.Username, ip, "login.failed", "")
		time.Sleep(400 * time.Millisecond)
		jsonErr(w, http.StatusUnauthorized, "Benutzername oder Passwort falsch")
		return
	}
	if u.TOTPEnabled {
		if req.TOTP == "" {
			jsonOK(w, map[string]any{"totpRequired": true})
			return
		}
		secret, _ := s.st.Box().Decrypt(u.TOTPSecret)
		if !secure.ValidateTOTP(secret, req.TOTP, time.Now()) {
			s.limiter.failed("ip:"+ip, "user:"+req.Username)
			s.audit(req.Username, ip, "login.totp_failed", "")
			jsonErr(w, http.StatusUnauthorized, "Bestätigungscode ungültig")
			return
		}
	}
	s.limiter.reset("ip:"+ip, "user:"+req.Username)
	tok, ss := s.sessions.create(u.Username, u.Role, ip, false)
	setSessionCookie(w, r, tok)
	_ = s.st.Update(func(st *store.State) error {
		for _, x := range st.Users {
			if x.Username == u.Username {
				now := time.Now()
				x.LastLogin = &now
			}
		}
		return nil
	})
	s.audit(u.Username, ip, "login.success", "")
	jsonOK(w, map[string]any{"csrf": ss.CSRF, "mustChange": u.MustChange})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, p *principal) {
	if p.cookie != "" {
		s.sessions.delete(p.cookie)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode})
	s.audit(p.User, clientIP(r), "logout", "")
	jsonOK(w, map[string]bool{"ok": true})
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
