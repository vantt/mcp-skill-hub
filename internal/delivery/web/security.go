package web

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

var requestSequence atomic.Uint64

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := fmt.Sprintf("req-%d", requestSequence.Add(1))
		w.Header().Set("X-Request-ID", reqID)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) isHostAllowed(rawHost string) bool {
	if s.opts.Dev && (rawHost == "127.0.0.1:5421" || rawHost == "localhost:5421") {
		return true
	}

	host, portStr, err := net.SplitHostPort(rawHost)
	if err != nil {
		return false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return false
	}

	for _, entry := range s.opts.AllowHosts {
		if strings.Contains(entry, ":") {
			if strings.EqualFold(entry, rawHost) {
				return true
			}
		} else {
			if strings.EqualFold(entry, host) && port == s.opts.ListenPort {
				return true
			}
		}
	}

	if port != s.opts.ListenPort {
		return false
	}

	hostLower := strings.ToLower(host)
	if hostLower == "127.0.0.1" || hostLower == "localhost" || hostLower == "::1" {
		return true
	}

	s.hostMu.Lock()
	now := s.opts.Now()
	if now.Sub(s.hostCacheAt) > 5*time.Second || s.cachedIPs == nil {
		s.cachedIPs = make(map[string]bool)
		if s.opts.Interfaces != nil {
			ifaces, err := s.opts.Interfaces()
			if err == nil {
				for _, iface := range ifaces {
					for _, ip := range iface.IPs {
						if v4 := ip.To4(); v4 != nil {
							s.cachedIPs[v4.String()] = true
						}
					}
				}
			}
		}
		if s.opts.Hostname != nil {
			h, err := s.opts.Hostname()
			if err == nil {
				s.cachedHost = strings.ToLower(h)
			}
		}
		s.hostCacheAt = now
	}
	isIP := s.cachedIPs[hostLower]
	cachedHost := s.cachedHost
	s.hostMu.Unlock()

	if isIP {
		return true
	}
	if cachedHost != "" && (hostLower == cachedHost || hostLower == cachedHost+".local") {
		return true
	}

	return false
}

func (s *Server) hostAllowlistMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.isHostAllowed(r.Host) {
			writeJSON(w, http.StatusMisdirectedRequest, app.ErrorResult(app.NewInvalidRequestError(
				"The Host header "+r.Host+" is not allowed.",
				"Open the URL printed by `skillhub serve web`, or add --allow-host "+r.Host+".",
			)))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func panicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				writeError(w, fmt.Errorf("panic: %v", rec), false)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func bodyLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > 4<<20 {
			writeJSON(w, http.StatusRequestEntityTooLarge, app.ErrorResult(app.NewInvalidRequestError(
				"Request body exceeds 4 MiB limit.",
				"Send a smaller request body.",
			)))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authThrottleMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		if s.throttle != nil && s.throttle.isBlocked(r.RemoteAddr) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, app.ErrorResult(app.NewInvalidRequestError(
				"Too many failed authentication attempts.",
				"Wait 60 seconds before trying again.",
			)))
			return
		}

		authHeader := r.Header.Get("Authorization")
		const prefix = "Bearer "
		var valid bool
		if strings.HasPrefix(authHeader, prefix) {
			token := strings.TrimPrefix(authHeader, prefix)
			if subtle.ConstantTimeCompare([]byte(token), []byte(s.opts.Token)) == 1 {
				valid = true
			}
		}

		if !valid {
			if s.throttle != nil {
				s.throttle.recordFailure(r.RemoteAddr)
			}
			writeJSON(w, http.StatusUnauthorized, app.ErrorResult(app.NewInvalidRequestError(
				"The session token is missing or invalid.",
				"Reopen the web UI from the URL printed by `skillhub serve web`.",
			)))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func originAndContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			expectedOrigin := "http://" + r.Host
			if r.Header.Get("Origin") != expectedOrigin {
				writeJSON(w, http.StatusForbidden, app.ErrorResult(app.NewInvalidRequestError(
					"The Origin header does not match.",
					"Mutating requests must originate from the web UI.",
				)))
				return
			}
			ct := r.Header.Get("Content-Type")
			if !strings.HasPrefix(strings.ToLower(ct), "application/json") {
				writeJSON(w, http.StatusUnsupportedMediaType, app.ErrorResult(app.NewInvalidRequestError(
					"The Content-Type header must be application/json.",
					"Encode request bodies as JSON.",
				)))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// withMiddleware wraps next in the full security middleware chain.
func (s *Server) withMiddleware(next http.Handler) http.Handler {
	handler := next
	handler = originAndContentTypeMiddleware(handler)
	handler = s.authThrottleMiddleware(handler)
	handler = bodyLimitMiddleware(handler)
	handler = securityHeadersMiddleware(handler)
	handler = panicRecoveryMiddleware(handler)
	handler = s.hostAllowlistMiddleware(handler)
	handler = requestIDMiddleware(handler)
	return handler
}
