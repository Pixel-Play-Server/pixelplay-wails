package worlds

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"pixelplay-launcher/internal/mcnbt"
	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/profiles"
)

type WorldInfo struct {
	FolderName       string  `json:"folder_name"`
	DisplayName      *string `json:"display_name"`
	LastPlayed       *int64  `json:"last_played"`
	IconPath         *string `json:"icon_path"`
	GameMode         *int32  `json:"game_mode"`
	Difficulty       *int    `json:"difficulty"`
	DifficultyLocked *bool   `json:"difficulty_locked"`
	IsHardcore       *bool   `json:"is_hardcore"`
	VersionName      *string `json:"version_name"`
}

type ServerInfo struct {
	Name            *string `json:"name"`
	Address         *string `json:"address"`
	IconBase64      *string `json:"icon_base64"`
	AcceptsTextures *int    `json:"accepts_textures"`
	PreviewsChat    *int    `json:"previews_chat"`
	IsFeatured      *bool   `json:"is_featured"`
}

func savesDir(profileID string) (string, error) {
	p, err := profiles.Get(profileID)
	if err != nil {
		return "", err
	}
	return filepath.Join(paths.ProfileDir(p.Path), "saves"), nil
}

func ListWorlds(profileID string) ([]WorldInfo, error) {
	dir, err := savesDir(profileID)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []WorldInfo{}, nil
		}
		return nil, err
	}
	out := make([]WorldInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		w := readWorld(filepath.Join(dir, e.Name()), e.Name())
		out = append(out, w)
	}
	return out, nil
}

func readWorld(folder, name string) WorldInfo {
	info := WorldInfo{FolderName: name}
	icon := filepath.Join(folder, "icon.png")
	if _, err := os.Stat(icon); err == nil {
		info.IconPath = &icon
	}
	level := filepath.Join(folder, "level.dat")
	f, err := os.Open(level)
	if err != nil {
		return info
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return info
	}
	defer gr.Close()
	_, root, err := mcnbt.ReadNamed(gr)
	if err != nil {
		return info
	}
	data := mcnbt.AsMap(mcnbt.AsMap(root)["Data"])
	if data == nil {
		data = mcnbt.AsMap(root)
	}
	if s := mcnbt.AsString(data["LevelName"]); s != "" {
		info.DisplayName = &s
	}
	if lp := mcnbt.AsInt64(data["LastPlayed"]); lp != 0 {
		info.LastPlayed = &lp
	}
	if _, ok := data["GameType"]; ok {
		gm := mcnbt.AsInt32(data["GameType"])
		info.GameMode = &gm
	}
	if _, ok := data["Difficulty"]; ok {
		d := int(mcnbt.AsByte(data["Difficulty"]))
		info.Difficulty = &d
	}
	if _, ok := data["DifficultyLocked"]; ok {
		b := mcnbt.AsByte(data["DifficultyLocked"]) != 0
		info.DifficultyLocked = &b
	}
	if _, ok := data["hardcore"]; ok {
		b := mcnbt.AsByte(data["hardcore"]) != 0
		info.IsHardcore = &b
	}
	if ver := mcnbt.AsMap(data["Version"]); ver != nil {
		if s := mcnbt.AsString(ver["Name"]); s != "" {
			info.VersionName = &s
		}
	}
	return info
}

func DeleteWorld(profileID, folder string) error {
	dir, err := savesDir(profileID)
	if err != nil {
		return err
	}
	target := filepath.Join(dir, filepath.Base(folder))
	if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), filepath.Clean(dir)+string(os.PathSeparator)) {
		return fmt.Errorf("carpeta de mundo inválida")
	}
	return os.RemoveAll(target)
}

func CopyWorld(sourceProfile, sourceFolder, targetProfile, targetName string) (string, error) {
	srcRoot, err := savesDir(sourceProfile)
	if err != nil {
		return "", err
	}
	dstRoot, err := savesDir(targetProfile)
	if err != nil {
		return "", err
	}
	src := filepath.Join(srcRoot, filepath.Base(sourceFolder))
	if _, err := os.Stat(src); err != nil {
		return "", err
	}
	base := strings.TrimSpace(targetName)
	if base == "" {
		base = sourceFolder
	}
	destName := uniqueDir(dstRoot, base)
	if err := copyDir(src, filepath.Join(dstRoot, destName)); err != nil {
		return "", err
	}
	return destName, nil
}

func uniqueDir(root, name string) string {
	cand := name
	for i := 0; i < 1000; i++ {
		if i > 0 {
			cand = fmt.Sprintf("%s (%d)", name, i)
		}
		if _, err := os.Stat(filepath.Join(root, cand)); os.IsNotExist(err) {
			return cand
		}
	}
	return fmt.Sprintf("%s-%d", name, time.Now().Unix())
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_ = os.MkdirAll(filepath.Dir(target), 0o755)
		return os.WriteFile(target, in, info.Mode())
	})
}

func IsLocked(profileID, folder string) (bool, error) {
	dir, err := savesDir(profileID)
	if err != nil {
		return false, err
	}
	lock := filepath.Join(dir, filepath.Base(folder), "session.lock")
	if _, err := os.Stat(lock); err != nil {
		return false, nil
	}
	f, err := os.OpenFile(lock, os.O_RDWR, 0)
	if err != nil {
		return true, nil
	}
	defer f.Close()
	return false, nil
}

func ListServers(profileID string) ([]ServerInfo, error) {
	p, err := profiles.Get(profileID)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(paths.ProfileDir(p.Path), "servers.dat")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []ServerInfo{}, nil
		}
		return nil, err
	}
	_, root, err := mcnbt.ReadNamed(bytes.NewReader(b))
	if err != nil {
		return []ServerInfo{}, nil
	}
	list, _ := mcnbt.AsMap(root)["servers"].([]any)
	out := make([]ServerInfo, 0, len(list))
	for _, item := range list {
		m := mcnbt.AsMap(item)
		if m == nil {
			continue
		}
		s := ServerInfo{}
		if n := mcnbt.AsString(m["name"]); n != "" {
			s.Name = &n
		}
		if a := mcnbt.AsString(m["ip"]); a != "" {
			s.Address = &a
		}
		if ic := mcnbt.AsString(m["icon"]); ic != "" {
			s.IconBase64 = &ic
		}
		if _, ok := m["acceptTextures"]; ok {
			v := int(mcnbt.AsByte(m["acceptTextures"]))
			s.AcceptsTextures = &v
		}
		out = append(out, s)
	}
	return out, nil
}

type PingInfo struct {
	Description     *string `json:"description"`
	DescriptionJSON any     `json:"description_json"`
	VersionName     *string `json:"version_name"`
	VersionProtocol *int    `json:"version_protocol"`
	PlayersOnline   *int    `json:"players_online"`
	PlayersMax      *int    `json:"players_max"`
	FaviconBase64   *string `json:"favicon_base64"`
	LatencyMS       *int64  `json:"latency_ms"`
	Error           *string `json:"error"`
}

func Ping(address string) PingInfo {
	host, port := splitAddr(address)
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 5*time.Second)
	if err != nil {
		msg := err.Error()
		return PingInfo{Error: &msg}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	if err := writeHandshake(conn, host, port); err != nil {
		msg := err.Error()
		return PingInfo{Error: &msg}
	}
	if _, err := conn.Write([]byte{0x01, 0x00}); err != nil {
		msg := err.Error()
		return PingInfo{Error: &msg}
	}
	_, payload, err := readPacket(conn)
	if err != nil {
		msg := err.Error()
		return PingInfo{Error: &msg}
	}
	lat := time.Since(start).Milliseconds()
	info := PingInfo{LatencyMS: &lat}
	if len(payload) == 0 {
		msg := "respuesta vacía"
		info.Error = &msg
		return info
	}
	s, _, err := readMCString(payload)
	if err != nil {
		s = string(payload)
	}
	var raw map[string]any
	if json.Unmarshal([]byte(s), &raw) == nil {
		info.DescriptionJSON = raw["description"]
		info.Description = motdText(raw["description"])
		if v, ok := raw["version"].(map[string]any); ok {
			if n, ok := v["name"].(string); ok {
				info.VersionName = &n
			}
			if p, ok := v["protocol"].(float64); ok {
				n := int(p)
				info.VersionProtocol = &n
			}
		}
		if p, ok := raw["players"].(map[string]any); ok {
			if o, ok := p["online"].(float64); ok {
				n := int(o)
				info.PlayersOnline = &n
			}
			if m, ok := p["max"].(float64); ok {
				n := int(m)
				info.PlayersMax = &n
			}
		}
		if fav, ok := raw["favicon"].(string); ok {
			fav = strings.TrimPrefix(fav, "data:image/png;base64,")
			info.FaviconBase64 = &fav
		}
	} else {
		info.Description = &s
	}
	return info
}

func splitAddr(address string) (string, int) {
	address = strings.TrimSpace(address)
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return address, 25565
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return host, 25565
	}
	return host, port
}

func writeHandshake(conn net.Conn, host string, port int) error {
	var body bytes.Buffer
	writeVarInt(&body, 0) // packet id
	writeVarInt(&body, -1)
	writeMCString(&body, host)
	_ = binaryWriteU16(&body, uint16(port))
	writeVarInt(&body, 1)
	var pkt bytes.Buffer
	writeVarInt(&pkt, body.Len())
	pkt.Write(body.Bytes())
	_, err := conn.Write(pkt.Bytes())
	return err
}

func binaryWriteU16(w io.Writer, v uint16) error {
	_, err := w.Write([]byte{byte(v >> 8), byte(v)})
	return err
}

func writeVarInt(w io.Writer, n int) {
	u := uint32(n)
	for {
		b := byte(u & 0x7F)
		u >>= 7
		if u != 0 {
			b |= 0x80
		}
		w.Write([]byte{b})
		if u == 0 {
			break
		}
	}
}

func writeMCString(w io.Writer, s string) {
	writeVarInt(w, len(s))
	io.WriteString(w, s)
}

func readPacket(conn net.Conn) (int, []byte, error) {
	length, err := readVarIntConn(conn)
	if err != nil {
		return 0, nil, err
	}
	if length <= 0 || length > 1<<20 {
		return 0, nil, fmt.Errorf("paquete ping inválido")
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return 0, nil, err
	}
	id, rest, err := readVarIntBytes(buf)
	if err != nil {
		return 0, buf, nil
	}
	return id, rest, nil
}

func readVarIntConn(r io.Reader) (int, error) {
	var n, shift int
	for i := 0; i < 5; i++ {
		var b [1]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		n |= int(b[0]&0x7F) << shift
		if b[0]&0x80 == 0 {
			return n, nil
		}
		shift += 7
	}
	return 0, fmt.Errorf("varint demasiado largo")
}

func readVarIntBytes(b []byte) (int, []byte, error) {
	var n, shift int
	for i := 0; i < len(b) && i < 5; i++ {
		n |= int(b[i]&0x7F) << shift
		if b[i]&0x80 == 0 {
			return n, b[i+1:], nil
		}
		shift += 7
	}
	return 0, nil, fmt.Errorf("varint inválido")
}

func readMCString(b []byte) (string, []byte, error) {
	n, rest, err := readVarIntBytes(b)
	if err != nil {
		return "", nil, err
	}
	if n < 0 || n > len(rest) {
		return "", nil, fmt.Errorf("string ping inválido")
	}
	return string(rest[:n]), rest[n:], nil
}

func motdText(v any) *string {
	switch t := v.(type) {
	case string:
		return &t
	case map[string]any:
		if s, ok := t["text"].(string); ok {
			extra := s
			if arr, ok := t["extra"].([]any); ok {
				for _, e := range arr {
					if p := motdText(e); p != nil {
						extra += *p
					}
				}
			}
			return &extra
		}
	}
	return nil
}
