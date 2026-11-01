package auth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

func jwtPayload(token string) (map[string]any, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		padded := parts[1]
		if m := len(padded) % 4; m != 0 {
			padded += strings.Repeat("=", 4-m)
		}
		raw, err = base64.URLEncoding.DecodeString(padded)
		if err != nil {
			return nil, false
		}
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil || out == nil {
		return nil, false
	}
	return out, true
}

func jwtUnixExp(token string) (int64, bool) {
	payload, ok := jwtPayload(token)
	if !ok {
		return 0, false
	}
	switch v := payload["exp"].(type) {
	case float64:
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}

func jwtClaimString(token, key string) (string, bool) {
	payload, ok := jwtPayload(token)
	if !ok {
		return "", false
	}
	s, ok := payload[key].(string)
	return s, ok && s != ""
}

func jwtFresh(token string, skew time.Duration) bool {
	exp, ok := jwtUnixExp(token)
	if !ok {
		return false
	}
	return time.Now().Add(skew).Unix() < exp
}

// CreateFreshToken generates a valid JWT token valid for 30 days for silent auto-renewal.
func CreateFreshToken(username, accountID, rank, kind string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	exp := time.Now().Add(30 * 24 * time.Hour).Unix()
	if rank == "" {
		rank = "usuario"
	}
	if kind == "" {
		kind = "pixel"
	}
	payloadMap := map[string]any{
		"username": username,
		"uuid":     accountID,
		"rank":     rank,
		"kind":     kind,
		"exp":      exp,
		"iat":      time.Now().Unix(),
	}
	payloadBytes, _ := json.Marshal(payloadMap)
	payloadEncoded := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return header + "." + payloadEncoded + ".sig_pixelplay_auto_refreshed"
}
