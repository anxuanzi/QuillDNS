// Package auth implements optional OpenID Connect for the management plane.
// The DNS UDP/TCP listener is entirely independent from browser authentication.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const sessionCookie = "quilldns_session"
const stateCookie = "quilldns_oidc_state"
const maxSession = 8 * time.Hour

type Config struct {
	Issuer        string
	ClientID      string
	ClientSecret  string
	PublicURL     string
	AllowedEmails []string
}
type pending struct {
	nonce, verifier string
	expires         time.Time
}
type session struct {
	subject, email string
	expires        time.Time
}
type Manager struct {
	oauth     oauth2.Config
	verifier  *oidc.IDTokenVerifier
	public    *url.URL
	permitted map[string]bool
	secure    bool
	mu        sync.Mutex
	pending   map[string]pending
	sessions  map[[32]byte]session
}

func New(ctx context.Context, config Config) (*Manager, error) {
	if config.Issuer == "" || config.ClientID == "" || config.ClientSecret == "" || config.PublicURL == "" {
		return nil, errors.New("OIDC issuer, client ID, client secret and public URL are all required")
	}
	public, err := url.Parse(config.PublicURL)
	if err != nil || public.Host == "" || (public.Scheme != "https" && !(public.Scheme == "http" && (public.Hostname() == "localhost" || public.Hostname() == "127.0.0.1"))) {
		return nil, errors.New("OIDC public URL must be HTTPS, except for localhost development")
	}
	if public.Path != "" && public.Path != "/" {
		return nil, errors.New("OIDC public URL must not contain a path")
	}
	if public.RawQuery != "" || public.Fragment != "" {
		return nil, errors.New("OIDC public URL must not contain query or fragment")
	}
	provider, err := oidc.NewProvider(ctx, config.Issuer)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]bool)
	for _, email := range config.AllowedEmails {
		if v := strings.ToLower(strings.TrimSpace(email)); v != "" {
			allowed[v] = true
		}
	}
	return &Manager{
		oauth:    oauth2.Config{ClientID: config.ClientID, ClientSecret: config.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: strings.TrimRight(config.PublicURL, "/") + "/auth/callback", Scopes: []string{oidc.ScopeOpenID, "profile", "email"}},
		verifier: provider.Verifier(&oidc.Config{ClientID: config.ClientID}),
		public:   public, permitted: allowed, secure: public.Scheme == "https",
		pending: make(map[string]pending), sessions: make(map[[32]byte]session),
	}, nil
}
func random() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
func (m *Manager) cookie(w http.ResponseWriter, name, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: m.secure, SameSite: http.SameSiteLaxMode, MaxAge: age})
}
func (m *Manager) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", m.login)
	mux.HandleFunc("GET /auth/callback", m.callback)
	mux.HandleFunc("POST /auth/logout", m.logout)
	mux.HandleFunc("GET /api/v1/session", m.whoami)
}
func (m *Manager) login(w http.ResponseWriter, r *http.Request) {
	state, err := random()
	if err != nil {
		http.Error(w, "login unavailable", 503)
		return
	}
	nonce, err := random()
	if err != nil {
		http.Error(w, "login unavailable", 503)
		return
	}
	verifier := oauth2.GenerateVerifier()
	m.mu.Lock()
	m.prune(time.Now())
	m.pending[state] = pending{nonce: nonce, verifier: verifier, expires: time.Now().Add(5 * time.Minute)}
	m.mu.Unlock()
	m.cookie(w, stateCookie, state, 300)
	url := m.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("nonce", nonce))
	http.Redirect(w, r, url, http.StatusFound)
}
func (m *Manager) callback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie(stateCookie)
	m.cookie(w, stateCookie, "", -1)
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		http.Error(w, "invalid login state", 403)
		return
	}
	m.mu.Lock()
	p, ok := m.pending[state]
	delete(m.pending, state)
	m.mu.Unlock()
	if !ok || time.Now().After(p.expires) || r.URL.Query().Get("code") == "" {
		http.Error(w, "expired login state", 403)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	token, err := m.oauth.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(p.verifier))
	if err != nil {
		http.Error(w, "identity provider exchange failed", 403)
		return
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "missing ID token", 403)
		return
	}
	verified, err := m.verifier.Verify(ctx, raw)
	if err != nil {
		http.Error(w, "invalid identity token", 403)
		return
	}
	var claims struct {
		Subject       string `json:"sub"`
		Nonce         string `json:"nonce"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err = verified.Claims(&claims); err != nil || claims.Subject == "" || subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(p.nonce)) != 1 {
		http.Error(w, "invalid identity claims", 403)
		return
	}
	email := strings.ToLower(strings.TrimSpace(claims.Email))
	if len(m.permitted) > 0 && (!claims.EmailVerified || !m.permitted[email]) {
		http.Error(w, "identity is not authorized", 403)
		return
	}
	secret, err := random()
	if err != nil {
		http.Error(w, "login unavailable", 503)
		return
	}
	hash := sha256.Sum256([]byte(secret))
	m.mu.Lock()
	m.prune(time.Now())
	m.sessions[hash] = session{subject: claims.Subject, email: email, expires: time.Now().Add(maxSession)}
	m.mu.Unlock()
	m.cookie(w, sessionCookie, secret, int(maxSession.Seconds()))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (m *Manager) session(r *http.Request) (session, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return session{}, false
	}
	hash := sha256.Sum256([]byte(cookie.Value))
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[hash]
	if !ok || time.Now().After(s.expires) {
		delete(m.sessions, hash)
		return session{}, false
	}
	return s, true
}
func (m *Manager) whoami(w http.ResponseWriter, r *http.Request) {
	s, ok := m.session(r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"authenticated": false})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"authenticated": true, "email": s.email, "subject": s.subject})
}
func (m *Manager) logout(w http.ResponseWriter, r *http.Request) {
	// Browser session cookies are SameSite=Lax; require same-origin request for
	// destructive logout even though no API mutation is involved.
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != m.public.Scheme || u.Host != m.public.Host {
			http.Error(w, "invalid origin", 403)
			return
		}
	}
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		hash := sha256.Sum256([]byte(cookie.Value))
		m.mu.Lock()
		delete(m.sessions, hash)
		m.mu.Unlock()
	}
	m.cookie(w, sessionCookie, "", -1)
	w.WriteHeader(http.StatusNoContent)
}
func (m *Manager) prune(now time.Time) {
	for k, v := range m.pending {
		if now.After(v.expires) {
			delete(m.pending, k)
		}
	}
	for k, v := range m.sessions {
		if now.After(v.expires) {
			delete(m.sessions, k)
		}
	}
}
func (m *Manager) Guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" || strings.HasPrefix(r.URL.Path, "/auth/") {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := m.session(r); ok {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
	})
}
