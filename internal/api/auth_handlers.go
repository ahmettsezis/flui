package api

import (
	"errors"
	"net/http"

	"github.com/ahmettsezis/flui/internal/auth"
)

type sessionResponse struct {
	User      *auth.User `json:"user"`
	CSRFToken string     `json:"csrf_token"`
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}
	if !s.loginLimiter.Allow(ip) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "too many sign-in attempts, try again in a minute")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Username == "" || req.Password == "" || len(req.Username) > 64 || len(req.Password) > 256 {
		writeError(w, http.StatusUnauthorized, auth.ErrInvalidCredentials.Error())
		return
	}
	raw, sess, u, err := s.Auth.Login(r.Context(), req.Username, req.Password, ip, r.UserAgent())
	if errors.Is(err, auth.ErrInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	setSessionCookie(w, raw, int(s.Config.Auth.SessionMaxLifetime.D().Seconds()))
	writeJSON(w, http.StatusOK, sessionResponse{User: u, CSRFToken: sess.CSRFToken})
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, sessionResponse{User: userFrom(r), CSRFToken: sessionFrom(r).CSRFToken})
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.Auth.Logout(r.Context(), sessionFrom(r), userFrom(r), s.clientIP(r)); err != nil {
		s.internalError(w, r, err)
		return
	}
	clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) changePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	u := userFrom(r)
	err := s.Auth.ChangePassword(r.Context(), u, sessionFrom(r), req.Current, req.New, s.clientIP(r))
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, http.StatusBadRequest, "current password is incorrect")
	case errors.Is(err, auth.ErrWeakPassword):
		writeError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		s.internalError(w, r, err)
	default:
		u.MustChangePassword = false
		if s.OnPasswordChanged != nil {
			s.OnPasswordChanged(u)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
