package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/ahmettsezis/flui/internal/auth"
)

// The __Host- prefix makes browsers reject the cookie unless it is Secure, host-only and Path=/.
const sessionCookie = "__Host-flui_session"

type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxSession
)

func userFrom(r *http.Request) *auth.User       { return r.Context().Value(ctxUser).(*auth.User) }
func sessionFrom(r *http.Request) *auth.Session { return r.Context().Value(ctxSession).(*auth.Session) }

// require authenticates the session and checks the role. Unsafe methods also need a valid CSRF token and a
// same-origin request. Users who must change their password can only reach handlers with allowPending.
func (s *server) require(role auth.Role, allowPending bool) func(http.HandlerFunc) http.Handler {
	return func(next http.HandlerFunc) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(sessionCookie)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "sign in required")
				return
			}
			sess, u, err := s.Auth.Authenticate(r.Context(), c.Value)
			if errors.Is(err, auth.ErrNoSession) {
				clearSessionCookie(w)
				writeError(w, http.StatusUnauthorized, "sign in required")
				return
			}
			if err != nil {
				s.internalError(w, r, err)
				return
			}
			if !safeMethod(r.Method) {
				if !sameOrigin(r) {
					writeError(w, http.StatusForbidden, "cross-origin request rejected")
					return
				}
				if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(sess.CSRFToken)) != 1 {
					writeError(w, http.StatusForbidden, "invalid CSRF token")
					return
				}
			}
			if u.MustChangePassword && !allowPending {
				writeError(w, http.StatusForbidden, "password change required")
				return
			}
			if !u.Role.Allows(role) {
				writeError(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			ctx := context.WithValue(context.WithValue(r.Context(), ctxUser, u), ctxSession, sess)
			next(w, r.WithContext(ctx))
		})
	}
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// sameOrigin rejects requests whose Origin (or Referer) does not match the Host, and non-JSON bodies, which
// cannot be sent cross-origin without a CORS preflight.
func sameOrigin(r *http.Request) bool {
	if r.ContentLength != 0 || r.Header.Get("Content-Type") != "" {
		mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mt != "application/json" {
			return false
		}
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

func setSessionCookie(w http.ResponseWriter, raw string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: raw, Path: "/", MaxAge: maxAge,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) { setSessionCookie(w, "", -1) }

// clientIP returns the client address, honouring X-Forwarded-For only from trusted proxies.
func (s *server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || !s.isTrusted(addr) {
		return host
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		if !s.isTrusted(hop) {
			return hop.String()
		}
	}
	return host
}

func (s *server) isTrusted(a netip.Addr) bool {
	a = a.Unmap()
	for _, p := range s.trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
