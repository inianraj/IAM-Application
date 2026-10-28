package auth

import (
	"net/http"
	"time"
)

// CookieName is the session cookie.
const CookieName = "session"

// CookieConfig holds the cookie settings that depend on the deployment.
type CookieConfig struct {
	// Secure sends the cookie over HTTPS only; disable for local http.
	Secure bool
	// TTL is the cookie lifetime; it matches the session lifetime in Redis.
	TTL time.Duration
}

// SetSessionCookie sends the token. The cookie is HttpOnly (not readable by
// scripts) and SameSite=Strict (not sent on cross-site requests).
func SetSessionCookie(w http.ResponseWriter, cfg CookieConfig, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(cfg.TTL.Seconds()),
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// ClearSessionCookie tells the browser to drop the cookie.
func ClearSessionCookie(w http.ResponseWriter, cfg CookieConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// TokenFromRequest returns the session token, or "" if there is none.
func TokenFromRequest(r *http.Request) string {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return ""
	}
	return c.Value
}
