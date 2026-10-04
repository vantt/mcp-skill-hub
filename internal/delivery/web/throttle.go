package web

import (
	"net"
	"sync"
	"time"
)

type throttleEntry struct {
	failures []time.Time
	lastSeen time.Time
}

type authThrottle struct {
	mu      sync.Mutex
	entries map[string]*throttleEntry
	now     func() time.Time
	maxKeys int
}

func newAuthThrottle(now func() time.Time) *authThrottle {
	if now == nil {
		now = time.Now
	}
	return &authThrottle{
		entries: make(map[string]*throttleEntry),
		now:     now,
		maxKeys: 1024,
	}
}

func extractIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func (t *authThrottle) isBlocked(remoteAddr string) bool {
	ip := extractIP(remoteAddr)
	t.mu.Lock()
	defer t.mu.Unlock()

	entry, ok := t.entries[ip]
	if !ok {
		return false
	}
	now := t.now()
	valid := entry.failures[:0]
	for _, ts := range entry.failures {
		if now.Sub(ts) < 60*time.Second {
			valid = append(valid, ts)
		}
	}
	entry.failures = valid
	return len(entry.failures) >= 20
}

func (t *authThrottle) recordFailure(remoteAddr string) {
	ip := extractIP(remoteAddr)
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	entry, ok := t.entries[ip]
	if !ok {
		if len(t.entries) >= t.maxKeys {
			var oldestIP string
			var oldestTime time.Time
			first := true
			for k, e := range t.entries {
				if first || e.lastSeen.Before(oldestTime) {
					oldestTime = e.lastSeen
					oldestIP = k
					first = false
				}
			}
			if oldestIP != "" {
				delete(t.entries, oldestIP)
			}
		}
		entry = &throttleEntry{}
		t.entries[ip] = entry
	}

	valid := entry.failures[:0]
	for _, ts := range entry.failures {
		if now.Sub(ts) < 60*time.Second {
			valid = append(valid, ts)
		}
	}
	entry.failures = append(valid, now)
	entry.lastSeen = now
}
