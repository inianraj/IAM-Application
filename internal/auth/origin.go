package auth

import (
	"net/http"
	"net/url"
	"slices"
	"strings"

	"crud/internal/util"
)

// OriginGuard protects the cookie-based API from cross-site requests (CSRF)
// and sets CORS headers for the allowed origins.
//
//   - A POST/PUT/PATCH/DELETE carrying an Origin header is rejected with 403
//     unless that origin is this host or listed in allowed. Requests without
//     an Origin header (curl, non-browser clients) pass.
//   - Origins in allowed get CORS headers with credentials, and preflight
//     (OPTIONS) requests are answered here. With no allowed origins the API is
//     same-origin only and no CORS headers are sent.
func OriginGuard(allowed []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			listed := origin != "" && slices.Contains(allowed, origin)

			if listed {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Add("Vary", "Origin")

				if r.Method == http.MethodOptions {
					h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
					h.Set("Access-Control-Allow-Headers", "Content-Type")
					h.Set("Access-Control-Max-Age", "600")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}

			if isUnsafe(r.Method) && origin != "" && !listed && !sameHost(origin, r.Host) {
				util.Error(w, http.StatusForbidden, "cross-origin request rejected")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isUnsafe(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func sameHost(origin, host string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, host)
}
