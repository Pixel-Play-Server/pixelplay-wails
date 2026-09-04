package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"pixelplay-launcher/internal/accounts"
	"pixelplay-launcher/internal/httpx"
)

// PrismLauncher public Azure client (device code + Xbox Live).
const clientID = "c36a9fb6-4f2a-41ff-90bd-ae7cc92031eb"
const scope = "XboxLive.signin XboxLive.offline_access offline_access"

type DeviceStart struct {
	UserCode                string `json:"user_code"`
	DeviceCode              string `json:"device_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
	Message                 string `json:"message"`
}

type msaToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
}

// BeginDeviceCode starts MSA device-code login.
func BeginDeviceCode() (*DeviceStart, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("scope", scope)
	var out DeviceStart
	err := httpx.PostForm("https://login.microsoftonline.com/consumers/oauth2/v2.0/devicecode", form, &out)
	if err != nil {
		return nil, err
	}
	if out.VerificationURI == "" {
		out.VerificationURI = "https://www.microsoft.com/link"
	}
	if out.VerificationURIComplete == "" && out.UserCode != "" {
		out.VerificationURIComplete = "https://www.microsoft.com/link?otc=" + url.QueryEscape(out.UserCode)
	}
	if out.Interval <= 0 {
		out.Interval = 5
	}
	return &out, nil
}

type loginResult struct {
	creds accounts.Credentials
	err   error
}

var (
	loginMu sync.Mutex
	pending *pendingLogin
)

type pendingLogin struct {
	dev  *DeviceStart
	done chan loginResult
}

// StartLogin begins device-code flow in the background and returns the code immediately.
func StartLogin(openURL func(string), emit func(string)) (*DeviceStart, error) {
	loginMu.Lock()
	defer loginMu.Unlock()
	if pending != nil && pending.dev != nil {
		return pending.dev, nil
	}
	dev, err := BeginDeviceCode()
	if err != nil {
		return nil, err
	}
	p := &pendingLogin{dev: dev, done: make(chan loginResult, 1)}
	pending = p
	go func() {
		creds, err := PollDeviceCode(dev, openURL, emit)
		p.done <- loginResult{creds: creds, err: err}
	}()
	return dev, nil
}

// WaitLogin blocks until StartLogin finishes (or no session exists).
func WaitLogin() (accounts.Credentials, error) {
	loginMu.Lock()
	p := pending
	loginMu.Unlock()
	if p == nil {
		return accounts.Credentials{}, fmt.Errorf("no hay un login Microsoft en curso")
	}
	res := <-p.done
	loginMu.Lock()
	if pending == p {
		pending = nil
	}
	loginMu.Unlock()
	return res.creds, res.err
}

// PollDeviceCode waits until the user finishes login, then returns Minecraft credentials.
func PollDeviceCode(dev *DeviceStart, openURL func(string), emit func(string)) (accounts.Credentials, error) {
	openTarget := dev.VerificationURI
	if dev.VerificationURIComplete != "" {
		openTarget = dev.VerificationURIComplete
	}
	if openURL != nil && openTarget != "" {
		_ = tryOpen(openURL, openTarget)
	}
	if emit != nil {
		msg := dev.Message
		if msg == "" {
			msg = fmt.Sprintf("Abrí %s e ingresá el código %s", dev.VerificationURI, dev.UserCode)
		}
		emit(msg)
	}
	interval := time.Duration(dev.Interval) * time.Second
	deadline := time.Now().Add(time.Duration(dev.ExpiresIn) * time.Second)
	if dev.ExpiresIn == 0 {
		deadline = time.Now().Add(15 * time.Minute)
	}
	for time.Now().Before(deadline) {
		time.Sleep(interval)
		tok, err := pollOnce(dev.DeviceCode)
		if err != nil {
			if strings.Contains(err.Error(), "authorization_pending") {
				continue
			}
			if strings.Contains(err.Error(), "slow_down") {
				interval += 2 * time.Second
				continue
			}
			return accounts.Credentials{}, err
		}
		if tok.AccessToken == "" {
			continue
		}
		return completeMinecraft(tok)
	}
	return accounts.Credentials{}, fmt.Errorf("login Microsoft expiró")
}

func tryOpen(openURL func(string), u string) error {
	if openURL == nil {
		return nil
	}
	openURL(u)
	return nil
}

func pollOnce(deviceCode string) (*msaToken, error) {
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("client_id", clientID)
	form.Set("device_code", deviceCode)
	var tok msaToken
	if err := httpx.PostForm("https://login.microsoftonline.com/consumers/oauth2/v2.0/token", form, &tok); err != nil {
		return nil, err
	}
	if tok.Error != "" {
		return nil, fmt.Errorf("%s", tok.Error)
	}
	return &tok, nil
}

// RefreshMinecraftIfNeeded renews Xbox/Minecraft tokens when the access token is near expiry.
func RefreshMinecraftIfNeeded(c accounts.Credentials) (accounts.Credentials, error) {
	if c.Offline() {
		return c, nil
	}
	if mcTokenFresh(c.AccessToken, c.Expires) {
		return c, nil
	}
	if strings.TrimSpace(c.RefreshToken) == "" {
		return c, fmt.Errorf("sesión Microsoft expirada — volvé a iniciar sesión")
	}
	return refreshMinecraft(c)
}

func mcTokenFresh(access, expires string) bool {
	if jwtFresh(access, 5*time.Minute) {
		return true
	}
	if t, err := time.Parse(time.RFC3339Nano, expires); err == nil {
		return time.Now().Add(5 * time.Minute).Before(t)
	}
	if t, err := time.Parse(time.RFC3339, expires); err == nil {
		return time.Now().Add(5 * time.Minute).Before(t)
	}
	return false
}

func refreshMinecraft(old accounts.Credentials) (accounts.Credentials, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", old.RefreshToken)
	form.Set("scope", scope)
	var tok msaToken
	if err := httpx.PostForm("https://login.microsoftonline.com/consumers/oauth2/v2.0/token", form, &tok); err != nil {
		return old, fmt.Errorf("refresh Microsoft: %w", err)
	}
	if tok.Error != "" {
		return old, fmt.Errorf("refresh Microsoft: %s", tok.Error)
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = old.RefreshToken
	}
	creds, err := completeMinecraft(&tok)
	if err != nil {
		return old, err
	}
	creds.PixelplayCredentials = old.PixelplayCredentials
	creds.Active = old.Active
	saved, err := accounts.Upsert(creds)
	if err != nil {
		return creds, err
	}
	return saved, nil
}

func completeMinecraft(tok *msaToken) (accounts.Credentials, error) {
	xbl, err := xboxLive(tok.AccessToken)
	if err != nil {
		return accounts.Credentials{}, err
	}
	xsts, uhs, err := xstsToken(xbl)
	if err != nil {
		return accounts.Credentials{}, err
	}
	mcTok, err := minecraftLogin(uhs, xsts)
	if err != nil {
		return accounts.Credentials{}, err
	}
	prof, err := minecraftProfile(mcTok)
	if err != nil {
		return accounts.Credentials{}, err
	}
	exp := time.Now().UTC().Add(24 * time.Hour)
	if tok.ExpiresIn > 0 {
		exp = time.Now().UTC().Add(time.Duration(tok.ExpiresIn) * time.Second)
	}
	return accounts.Credentials{
		ID:           prof.ID,
		Username:     prof.Name,
		AccessToken:  mcTok,
		RefreshToken: tok.RefreshToken,
		Expires:      exp.Format(time.RFC3339Nano),
		Active:       true,
	}, nil
}

func xboxLive(msaAccess string) (string, error) {
	payload := map[string]any{
		"Properties": map[string]any{
			"AuthMethod": "RPS",
			"SiteName":   "user.auth.xboxlive.com",
			"RpsTicket":  "d=" + msaAccess,
		},
		"RelyingParty": "http://auth.xboxlive.com",
		"TokenType":    "JWT",
	}
	var out struct {
		Token string `json:"Token"`
	}
	if err := httpx.PostJSON("https://user.auth.xboxlive.com/user/authenticate", payload, &out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", fmt.Errorf("Xbox Live no devolvió token")
	}
	return out.Token, nil
}

func xstsToken(xbl string) (token, uhs string, err error) {
	payload := map[string]any{
		"Properties": map[string]any{
			"SandboxId":  "RETAIL",
			"UserTokens": []string{xbl},
		},
		"RelyingParty": "rp://api.minecraftservices.com/",
		"TokenType":    "JWT",
	}
	var out struct {
		Token         string `json:"Token"`
		DisplayClaims struct {
			Xui []struct {
				Uhs string `json:"uhs"`
			} `json:"xui"`
		} `json:"DisplayClaims"`
	}
	if err := httpx.PostJSON("https://xsts.auth.xboxlive.com/xsts/authorize", payload, &out); err != nil {
		return "", "", err
	}
	uhs = ""
	if len(out.DisplayClaims.Xui) > 0 {
		uhs = out.DisplayClaims.Xui[0].Uhs
	}
	if out.Token == "" || uhs == "" {
		return "", "", fmt.Errorf("XSTS incompleto")
	}
	return out.Token, uhs, nil
}

func minecraftLogin(uhs, xsts string) (string, error) {
	payload := map[string]any{
		"identityToken": "XBL3.0 x=" + uhs + ";" + xsts,
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := httpx.PostJSON("https://api.minecraftservices.com/authentication/login_with_xbox", payload, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("Minecraft services no devolvió access_token")
	}
	return out.AccessToken, nil
}

func minecraftProfile(mcToken string) (struct{ ID, Name string }, error) {
	var a struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := getAuthJSON("https://api.minecraftservices.com/minecraft/profile", mcToken, &a); err != nil {
		return struct{ ID, Name string }{}, err
	}
	id := a.ID
	if len(id) == 32 {
		id = id[0:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
	}
	return struct{ ID, Name string }{ID: id, Name: a.Name}, nil
}

func getAuthJSON(rawURL, bearer string, dest any) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", httpx.UserAgent)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/json")
	res, err := httpx.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		return fmt.Errorf("perfil Minecraft: %s — %s", res.Status, string(b))
	}
	return json.Unmarshal(b, dest)
}
