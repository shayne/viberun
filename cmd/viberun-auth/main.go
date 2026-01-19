// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/shayne/viberun/internal/auth"
	"github.com/shayne/viberun/internal/proxy"
	"golang.org/x/crypto/bcrypt"
)

const (
	authPathPrefix = "/__viberun/auth"
)

type server struct {
	configPath string
	assetsDir  string
	loginTmpl  *template.Template
}

type loginPageData struct {
	Assets   string
	Redirect string
	Error    string
}

func main() {
	var (
		configPath = flag.String("config", "", "path to proxy config")
		listenAddr = flag.String("listen", "", "listen address override")
		assetsDir  = flag.String("assets", "/etc/viberun/auth", "auth assets directory")
	)
	flag.Parse()

	cfg, err := loadConfig(strings.TrimSpace(*configPath))
	if err != nil {
		log.Fatalf("failed to load proxy config: %v", err)
	}

	addr := strings.TrimSpace(*listenAddr)
	if addr == "" {
		addr = strings.TrimSpace(cfg.Auth.ListenAddr)
	}
	if addr == "" {
		addr = proxy.DefaultAuthListenAddr()
	}

	tmplPath := filepath.Join(strings.TrimSpace(*assetsDir), "login.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		log.Fatalf("failed to load login template: %v", err)
	}

	s := &server{
		configPath: strings.TrimSpace(*configPath),
		assetsDir:  strings.TrimSpace(*assetsDir),
		loginTmpl:  tmpl,
	}

	mux := http.NewServeMux()
	assetsPrefix := authPathPrefix + "/assets/"
	mux.Handle(assetsPrefix, http.StripPrefix(assetsPrefix, http.FileServer(http.Dir(filepath.Join(s.assetsDir, "assets")))))
	mux.HandleFunc(authPathPrefix+"/login", s.handleLogin)
	mux.HandleFunc(authPathPrefix+"/logout", s.handleLogout)
	mux.HandleFunc(authPathPrefix+"/verify", s.handleVerify)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("viberun-auth listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("viberun-auth failed: %v", err)
	}
}

var loginLimiter = newRateLimiter(20, time.Minute)

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.renderLogin(w, r, "", "")
		return
	case http.MethodPost:
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()
	if err := r.ParseForm(); err != nil {
		s.renderLogin(w, r, "invalid form submission", "")
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	redirect := r.FormValue("redirect")
	client := clientIP(r)
	if !loginLimiter.Allow(client) {
		s.renderLogin(w, r, "too many attempts, try again soon", redirect)
		return
	}

	cfg, err := s.loadConfig()
	if err != nil {
		http.Error(w, "auth unavailable", http.StatusServiceUnavailable)
		return
	}
	if cfg.Auth.SigningKey == "" {
		http.Error(w, "auth not configured", http.StatusServiceUnavailable)
		return
	}
	user, ok := findUser(cfg, username)
	if !ok || !checkPassword(user.Password, password) {
		sleepToUniformDelay(start, 250*time.Millisecond)
		log.Printf("auth login failed user=%s ip=%s", username, client)
		s.renderLogin(w, r, "invalid username or password", redirect)
		return
	}

	ttl := parseTTL(cfg.Auth.CookieTTL)
	session, err := auth.NewSession(user.Username, ttl)
	if err != nil {
		http.Error(w, "unable to create session", http.StatusInternalServerError)
		return
	}
	token, err := auth.SignSession(session, []byte(cfg.Auth.SigningKey))
	if err != nil {
		http.Error(w, "unable to sign session", http.StatusInternalServerError)
		return
	}

	redirect = auth.SafeRedirect(r, redirect)
	cookie := buildSessionCookie(r, cfg, token)
	http.SetCookie(w, &cookie)
	log.Printf("auth login success user=%s ip=%s", user.Username, client)
	http.Redirect(w, r, redirect, http.StatusFound)
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, err := s.loadConfig()
	if err != nil {
		http.Error(w, "auth unavailable", http.StatusServiceUnavailable)
		return
	}
	cookieName := strings.TrimSpace(cfg.Auth.CookieName)
	if cookieName == "" {
		cookieName = proxy.DefaultAuthCookieName()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		Domain:   cookieDomain(cfg),
		HttpOnly: true,
		MaxAge:   -1,
	})
	log.Printf("auth logout ip=%s", clientIP(r))
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *server) handleVerify(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadConfig()
	if err != nil {
		http.Error(w, "auth unavailable", http.StatusServiceUnavailable)
		return
	}
	cookieName := strings.TrimSpace(cfg.Auth.CookieName)
	if cookieName == "" {
		cookieName = proxy.DefaultAuthCookieName()
	}
	cookie, err := r.Cookie(cookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		s.redirectToLogin(w, r)
		return
	}
	if cfg.Auth.SigningKey == "" {
		http.Error(w, "auth not configured", http.StatusServiceUnavailable)
		return
	}
	session, err := auth.VerifySession(cookie.Value, []byte(cfg.Auth.SigningKey))
	if err != nil {
		s.redirectToLogin(w, r)
		return
	}
	if _, ok := findUser(cfg, session.Username); !ok {
		s.redirectToLogin(w, r)
		return
	}
	app, ok := appForHost(cfg, forwardedHost(r))
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	access := proxy.EffectiveAppAccess(cfg, app)
	if access.Disabled {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if access.Access == proxy.AccessPrivate {
		allowed := proxy.EffectiveAllowedUsers(cfg, app)
		if !userAllowed(allowed, session.Username) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}
	w.Header().Set("X-Viberun-User", session.Username)
	if strings.TrimSpace(cfg.PrimaryUser) != "" && session.Username == cfg.PrimaryUser {
		w.Header().Set("X-Viberun-Roles", "primary")
	} else {
		w.Header().Set("X-Viberun-Roles", "user")
	}
	w.WriteHeader(http.StatusOK)
}

func (s *server) renderLogin(w http.ResponseWriter, r *http.Request, errMsg string, redirect string) {
	if redirect == "" {
		redirect = r.URL.Query().Get("redirect")
	}
	redirect = auth.SafeRedirect(r, redirect)
	data := loginPageData{
		Assets:   authPathPrefix + "/assets",
		Redirect: redirect,
		Error:    errMsg,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.loginTmpl.Execute(w, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (s *server) redirectToLogin(w http.ResponseWriter, r *http.Request) {
	redirect := url.QueryEscape(auth.BuildRedirectURL(r))
	target := authPathPrefix + "/login?redirect=" + redirect
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *server) loadConfig() (proxy.Config, error) {
	return loadConfig(s.configPath)
}

func loadConfig(path string) (proxy.Config, error) {
	if strings.TrimSpace(path) != "" {
		return proxy.LoadConfigFromPath(path)
	}
	cfg, _, err := proxy.LoadConfig()
	return cfg, err
}

func cookieDomain(cfg proxy.Config) string {
	if value := strings.TrimSpace(cfg.Auth.CookieDomain); value != "" {
		return value
	}
	return proxy.DefaultCookieDomain(cfg)
}

func buildSessionCookie(r *http.Request, cfg proxy.Config, token string) http.Cookie {
	cookieName := strings.TrimSpace(cfg.Auth.CookieName)
	if cookieName == "" {
		cookieName = proxy.DefaultAuthCookieName()
	}
	secure := isSecureRequest(r)
	if cfg.Auth.CookieSecure != nil {
		secure = *cfg.Auth.CookieSecure
	}
	return http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Domain:   cookieDomain(cfg),
		HttpOnly: true,
		SameSite: parseSameSite(cfg.Auth.CookieSame),
		Secure:   secure,
		Expires:  time.Now().Add(parseTTL(cfg.Auth.CookieTTL)),
	}
}

func parseTTL(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 12 * time.Hour
	}
	ttl, err := time.ParseDuration(raw)
	if err != nil || ttl <= 0 {
		return 12 * time.Hour
	}
	return ttl
}

func parseSameSite(raw string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func isSecureRequest(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return r.TLS != nil
}

func forwardedHost(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); value != "" {
		return value
	}
	return r.Host
}

func findUser(cfg proxy.Config, username string) (proxy.AuthUser, bool) {
	username = strings.TrimSpace(username)
	if username == "" {
		return proxy.AuthUser{}, false
	}
	for _, user := range cfg.Users {
		if user.Username == username || (user.Email != "" && user.Email == username) {
			return user, true
		}
	}
	return proxy.AuthUser{}, false
}

func checkPassword(hash string, password string) bool {
	hash = strings.TrimSpace(hash)
	hash = strings.TrimPrefix(hash, "bcrypt:")
	if hash == "" || password == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func appForHost(cfg proxy.Config, host string) (string, bool) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return "", false
	}
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}
	for app, access := range cfg.Apps {
		custom := strings.ToLower(strings.TrimSpace(access.CustomDomain))
		if custom == "" {
			continue
		}
		if custom == host {
			return app, true
		}
	}
	base := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(cfg.BaseDomain), "."))
	if base == "" {
		return "", false
	}
	if !strings.HasSuffix(host, "."+base) {
		return "", false
	}
	app := strings.TrimSuffix(host, "."+base)
	app = strings.Trim(app, ".")
	if app == "" {
		return "", false
	}
	return app, true
}

func userAllowed(users []string, username string) bool {
	for _, user := range users {
		if user == username {
			return true
		}
	}
	return false
}

type rateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rateEntry
	limit   int
	window  time.Duration
}

type rateEntry struct {
	count int
	start time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	if limit < 1 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	return &rateLimiter{
		entries: map[string]*rateEntry{},
		limit:   limit,
		window:  window,
	}
}

func (r *rateLimiter) Allow(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "unknown"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	entry := r.entries[key]
	if entry == nil {
		r.entries[key] = &rateEntry{count: 1, start: now}
		return true
	}
	if now.Sub(entry.start) > r.window {
		entry.count = 1
		entry.start = now
		return true
	}
	if entry.count >= r.limit {
		return false
	}
	entry.count++
	return true
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host := r.RemoteAddr
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}
	return host
}

func sleepToUniformDelay(start time.Time, min time.Duration) {
	if min <= 0 {
		return
	}
	elapsed := time.Since(start)
	if elapsed >= min {
		return
	}
	time.Sleep(min - elapsed)
}
