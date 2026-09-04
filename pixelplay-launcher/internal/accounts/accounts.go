package accounts

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"pixelplay-launcher/internal/paths"
)

var mu sync.Mutex

type Token struct {
	Value string `json:"value"`
}

type PixelplayCredentials struct {
	Production   *Token `json:"production"`
	Experimental *Token `json:"experimental"`
}

// Credentials matches the Tauri accounts.json shape.
type Credentials struct {
	ID                   string               `json:"id"`
	Username             string               `json:"username"`
	AccessToken          string               `json:"access_token"`
	RefreshToken         string               `json:"refresh_token"`
	Expires              string               `json:"expires"`
	PixelplayCredentials PixelplayCredentials `json:"pixelplay_credentials"`
	Active               bool                 `json:"active"`
}

type storeFile struct {
	Accounts []Credentials `json:"accounts"`
	Token    any           `json:"token"`
}

func accountsPath() string {
	return paths.AccountsFile()
}

func load() (*storeFile, error) {
	p := accountsPath()
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &storeFile{Accounts: []Credentials{}}, nil
		}
		return nil, err
	}
	var st storeFile
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("accounts.json: %w", err)
	}
	if st.Accounts == nil {
		st.Accounts = []Credentials{}
	}
	return &st, nil
}

func save(st *storeFile) error {
	if err := os.MkdirAll(paths.DataDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	p := accountsPath()
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (c Credentials) Offline() bool {
	return strings.TrimSpace(c.AccessToken) == ""
}

func canonUUID(id string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(id), "-", ""))
}

func sameID(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) {
		return true
	}
	ca, cb := canonUUID(a), canonUUID(b)
	return len(ca) == 32 && len(cb) == 32 && ca == cb
}

func matchAccount(a Credentials, id string) bool {
	if sameID(a.ID, id) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(a.Username), strings.TrimSpace(id))
}

// ForUI is the shape the React store expects (minecraft_username + expires_at aliases).
func (c Credentials) ForUI() map[string]any {
	name := c.Username
	return map[string]any{
		"id":                    c.ID,
		"username":              name,
		"minecraft_username":    name,
		"active":                c.Active,
		"access_token":          c.AccessToken,
		"refresh_token":         c.RefreshToken,
		"expires":               c.Expires,
		"expires_at":            c.Expires,
		"offline":               c.Offline(),
		"pixelplay_credentials": c.PixelplayCredentials,
	}
}

func ForUIList(list []Credentials) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		out = append(out, c.ForUI())
	}
	return out
}

// offlineUUID matches Java UUID.nameUUIDFromBytes(("OfflinePlayer:"+name).getBytes(UTF_8)).
func offlineUUID(name string) string {
	sum := md5.Sum([]byte("OfflinePlayer:" + name))
	sum[6] = (sum[6] & 0x0f) | 0x30
	sum[8] = (sum[8] & 0x3f) | 0x80
	id, err := uuid.FromBytes(sum[:])
	if err != nil {
		return uuid.NewString()
	}
	return id.String()
}

func List() ([]Credentials, error) {
	mu.Lock()
	defer mu.Unlock()
	st, err := load()
	if err != nil {
		return nil, err
	}
	return st.Accounts, nil
}

func Active() (*Credentials, error) {
	list, err := List()
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Active {
			c := list[i]
			return &c, nil
		}
	}
	return nil, nil
}

func SetActive(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("falta id de cuenta")
	}
	mu.Lock()
	defer mu.Unlock()
	st, err := load()
	if err != nil {
		return err
	}
	idx := -1
	for i := range st.Accounts {
		if matchAccount(st.Accounts[i], id) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("cuenta no encontrada: %s", id)
	}
	for i := range st.Accounts {
		st.Accounts[i].Active = i == idx
	}
	return save(st)
}

func Remove(id string) error {
	mu.Lock()
	defer mu.Unlock()
	st, err := load()
	if err != nil {
		return err
	}
	out := st.Accounts[:0]
	for _, a := range st.Accounts {
		if !matchAccount(a, id) {
			out = append(out, a)
		}
	}
	st.Accounts = out
	// keep one active if possible
	hasActive := false
	for _, a := range st.Accounts {
		if a.Active {
			hasActive = true
			break
		}
	}
	if !hasActive && len(st.Accounts) > 0 {
		st.Accounts[0].Active = true
	}
	return save(st)
}

func Upsert(creds Credentials) (Credentials, error) {
	mu.Lock()
	defer mu.Unlock()
	st, err := load()
	if err != nil {
		return Credentials{}, err
	}
	found := false
	for i := range st.Accounts {
		st.Accounts[i].Active = false
		if sameID(st.Accounts[i].ID, creds.ID) || strings.EqualFold(st.Accounts[i].Username, creds.Username) {
			creds.Active = true
			st.Accounts[i] = creds
			found = true
		}
	}
	if !found {
		creds.Active = true
		st.Accounts = append(st.Accounts, creds)
	}
	if err := save(st); err != nil {
		return Credentials{}, err
	}
	return creds, nil
}

func CreateOffline(username string, idOpt string) (Credentials, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return Credentials{}, fmt.Errorf("el usuario no puede estar vacío")
	}
	id := strings.TrimSpace(idOpt)
	if id != "" {
		if _, err := uuid.Parse(id); err != nil {
			return Credentials{}, fmt.Errorf("uuid inválido: %w", err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	st, err := load()
	if err != nil {
		return Credentials{}, err
	}

	for _, a := range st.Accounts {
		if strings.EqualFold(a.Username, username) && !a.Offline() {
			return Credentials{}, fmt.Errorf("ya existe una cuenta Microsoft con ese nombre")
		}
	}

	expires := time.Now().UTC().AddDate(10, 0, 0).Format(time.RFC3339Nano)
	for i := range st.Accounts {
		st.Accounts[i].Active = false
	}
	for i := range st.Accounts {
		if strings.EqualFold(st.Accounts[i].Username, username) && st.Accounts[i].Offline() {
			if id != "" {
				st.Accounts[i].ID = id
			}
			st.Accounts[i].Username = username
			st.Accounts[i].Expires = expires
			st.Accounts[i].Active = true
			if err := save(st); err != nil {
				return Credentials{}, err
			}
			return st.Accounts[i], nil
		}
	}

	if id == "" {
		id = offlineUUID(username)
	}
	creds := Credentials{
		ID:           id,
		Username:     username,
		AccessToken:  "",
		RefreshToken: "",
		Expires:      expires,
		PixelplayCredentials: PixelplayCredentials{
			Production:   nil,
			Experimental: nil,
		},
		Active: true,
	}
	st.Accounts = append(st.Accounts, creds)
	if err := save(st); err != nil {
		return Credentials{}, err
	}
	return creds, nil
}
