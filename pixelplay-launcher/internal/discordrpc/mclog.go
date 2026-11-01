package discordrpc

import (
	"net"
	"regexp"
	"strings"
)

var (
	reConnecting = regexp.MustCompile(`(?i)Connecting to ([^,\s:]+)(?:,\s*(\d+)|:(\d+))?`)
	reDisconnect = regexp.MustCompile(`(?i)Client disconnected`)
)

// ParseConnecting extracts a multiplayer host from a Minecraft log line.
// Vanilla/NeoForge: "Connecting to mc.pixelplay.gg, 25565"
func ParseConnecting(line string) (host string, ok bool) {
	m := reConnecting.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	host = strings.TrimSpace(m[1])
	if !looksLikeServer(host) {
		return "", false
	}
	return host, true
}

// IsDisconnect reports a leave/kick/timeout from the current server.
func IsDisconnect(line string) bool {
	return reDisconnect.MatchString(line)
}

type Session struct {
	Host    string
	Seconds int64
}

func ExtractSessions(logText string) []Session {
	reVoice := "voice chat"
	var out []Session
	var cur string
	for _, line := range strings.Split(logText, "\n") {
		if strings.Contains(strings.ToLower(line), reVoice) {
			continue
		}
		if host, ok := ParseConnecting(line); ok {
			if cur != "" {
				out = append(out, Session{Host: cur})
			}
			cur = host
			continue
		}
		if cur != "" && IsDisconnect(line) {
			out = append(out, Session{Host: cur})
			cur = ""
		}
	}
	if cur != "" {
		out = append(out, Session{Host: cur})
	}
	return out
}

func looksLikeServer(host string) bool {
	h := strings.ToLower(strings.Trim(host, "[]"))
	if h == "" || h == "server" || h == "the" {
		return false
	}
	if h == "localhost" || h == "127.0.0.1" {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return true
	}
	return strings.Contains(h, ".")
}
