package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const editorSessionCookie = "oddity_editor_session"
const editorSessionTTL = 12 * time.Hour
const csrfCookieName = "oddity_csrf"

type loginData struct {
	CSRF  string
	Error string
}

func (a *App) editorUser() string {
	if a.cfg.EditorUser != "" {
		return a.cfg.EditorUser
	}
	return "editor"
}
func (a *App) editorPassword() string {
	return a.cfg.EditorPassword
}
func (a *App) sessionSecret() string {
	return a.cfg.SessionSecret
}
func (a *App) adminCookiePath() string {
	if a.cfg.AdminCookiePath != "" {
		return a.cfg.AdminCookiePath
	}
	return "/admin"
}

func (a *App) makeEditorSession(now time.Time) string {
	payload := a.editorUser() + "|" + strconv.FormatInt(now.Add(editorSessionTTL).Unix(), 10)
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	mac := hmac.New(sha256.New, []byte(a.sessionSecret()))
	_, _ = mac.Write([]byte(encoded))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encoded + "." + signature
}

func (a *App) validEditorSession(value string, now time.Time) bool {
	encoded, signature, ok := strings.Cut(value, ".")
	if !ok {
		return false
	}
	mac := hmac.New(sha256.New, []byte(a.sessionSecret()))
	_, _ = mac.Write([]byte(encoded))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !secureEqual(signature, expected) {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}
	user, expiryRaw, ok := strings.Cut(string(raw), "|")
	if !ok || user != a.editorUser() {
		return false
	}
	expiry, err := strconv.ParseInt(expiryRaw, 10, 64)
	return err == nil && now.Unix() < expiry
}

func (a *App) requireAdminSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(editorSessionCookie); err == nil && a.validEditorSession(cookie.Value, time.Now()) {
			setAdminSecurityHeaders(w)
			next.ServeHTTP(w, r)
			return
		}
		user, password, basic := r.BasicAuth()
		basicUserOK := secureEqual(user, "admin")
		basicPasswordOK := secureEqual(password, a.cfg.AdminAPIKey)
		if basic && basicUserOK && basicPasswordOK {
			setAdminSecurityHeaders(w)
			next.ServeHTTP(w, r)
			return
		}
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
	})
}

func setAdminSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https:; style-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "same-origin")
}

func (a *App) adminLoginPage(w http.ResponseWriter, r *http.Request) {
	a.renderLogin(w, loginData{CSRF: a.ensureCSRFToken(w, r)}, http.StatusOK)
}

func (a *App) adminLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil || !a.validCSRF(r, r.FormValue("csrf")) {
		a.renderLogin(w, loginData{CSRF: a.ensureCSRFToken(w, r), Error: "Форма устарела. Обновите страницу и попробуйте снова."}, http.StatusForbidden)
		return
	}
	userOK := secureEqual(r.FormValue("username"), a.editorUser())
	passwordOK := secureEqual(r.FormValue("password"), a.editorPassword())
	if !userOK || !passwordOK {
		a.renderLogin(w, loginData{CSRF: a.ensureCSRFToken(w, r), Error: "Неверный логин или пароль"}, http.StatusUnauthorized)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: editorSessionCookie, Value: a.makeEditorSession(time.Now()), Path: a.adminCookiePath(), MaxAge: int(editorSessionTTL.Seconds()), HttpOnly: true, Secure: a.cfg.SecureCookies, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/admin/articles", http.StatusSeeOther)
}

func (a *App) adminLogout(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil || !a.validCSRF(r, r.FormValue("csrf")) {
		http.Error(w, "Недействительный CSRF-токен", http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: editorSessionCookie, Value: "", Path: a.adminCookiePath(), MaxAge: -1, HttpOnly: true, Secure: a.cfg.SecureCookies, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func (a *App) renderLogin(w http.ResponseWriter, data loginData, status int) {
	var buf strings.Builder
	if err := webTemplates.ExecuteTemplate(&buf, "admin_login.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Не удалось отобразить страницу входа: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(buf.String()))
}
