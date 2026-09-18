package server

import (
	"math"
	"net"
	"net/netip"
	"sync"
	"time"
)

const maxLoginPeers = 1024

type attemptWindow struct {
	count int
	reset time.Time
}

type loginLimiter struct {
	mu     sync.Mutex
	now    func() time.Time
	peers  map[string]attemptWindow
	global attemptWindow
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{now: time.Now, peers: make(map[string]attemptWindow)}
}

func (l *loginLimiter) allow(peer string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if !now.Before(l.global.reset) {
		l.global = attemptWindow{reset: now.Add(time.Minute)}
	}
	retry := func(reset time.Time) int { return max(1, int(math.Ceil(reset.Sub(now).Seconds()))) }
	if l.global.count >= 60 {
		return retry(l.global.reset)
	}
	for key, window := range l.peers {
		if !now.Before(window.reset) {
			delete(l.peers, key)
		}
	}
	window, exists := l.peers[peer]
	if !exists {
		if len(l.peers) >= maxLoginPeers {
			return 60
		}
		window = attemptWindow{reset: now.Add(time.Minute)}
	}
	if window.count >= 5 {
		return retry(window.reset)
	}
	window.count++
	l.global.count++
	l.peers[peer] = window
	return 0
}

func peerAddress(remote string) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return "unknown"
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return "unknown"
	}
	// Forwarded headers are intentionally ignored until trusted proxies are configured.
	return ip.Unmap().String()
}
