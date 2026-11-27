package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	errRejected = errors.New("rejected: wrong username or password?")
	errNoCreds  = errors.New("no saved username/password")
	errStopped  = errors.New("login cancelled")

	// retryEvery is how long LoginRetry waits after a login that gave no token.
	retryEvery = 10 * time.Second
)

// loginRejected is the site saying no to the username/password. It keeps the site's own
// words so the window shows the real reason.
type loginRejected struct {
	code int
	body string // ": <what the site said>" or ""
}

func (e loginRejected) Error() string {
	return fmt.Sprintf("rejected by the site (HTTP %d%s): wrong username or password?", e.code, e.body)
}

func (e loginRejected) Is(target error) bool { return target == errRejected }

// looksLikeBadLogin tells a "wrong username or password" 403 from a plain block.
func looksLikeBadLogin(body string) bool {
	b := strings.ToLower(body)
	for _, w := range []string{"password", "username", "credential", "invalid", "incorrect", "wrong", "not found", "no such user", "unauthorized"} {
		if strings.Contains(b, w) {
			return true
		}
	}
	return false
}

// LoginRetry logs in, and when the site gives no token back it tries again every
// retryEvery, forever. It gives up only for a wrong password or when cancel() is true.
func (a *App) LoginRetry(cancel func() bool) error {
	for {
		if cancel != nil && cancel() {
			return errStopped
		}
		err := a.Login()
		if err == nil || errors.Is(err, errRejected) || errors.Is(err, errNoCreds) {
			return err
		}
		a.mu.Lock()
		a.info = fmt.Sprintf("%v - trying again in %ds", err, int(retryEvery.Seconds()))
		a.mu.Unlock()
		end := time.Now().Add(retryEvery)
		for time.Now().Before(end) {
			if cancel != nil && cancel() {
				return errStopped
			}
			step := 100 * time.Millisecond
			if r := time.Until(end); r < step {
				step = r
			}
			time.Sleep(step)
		}
	}
}

func (a *App) setTokenLocked(tok string) {
	a.cfg.Token = tok
	a.tokenIdx = 0
	a.expiryWarned = false
	_, a.exp, _ = tokenInfo(tok)
	a.saveConfigLocked()
}

// The username and password live in plain text in config.json so they are easy to edit.
func (a *App) hasCredsLocked() bool {
	return a.cfg.Username != "" && a.cfg.Password != ""
}

func (a *App) credsLocked() (user, pass string, ok bool) {
	if !a.hasCredsLocked() {
		return "", "", false
	}
	return a.cfg.Username, a.cfg.Password, true
}

// SetCredentials saves the username and password.
func (a *App) SetCredentials(user, pass string) error {
	user = strings.TrimSpace(user)
	if user == "" || pass == "" {
		return errors.New("enter both a username and a password")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg.Username = user
	a.cfg.Password = pass
	a.saveConfigLocked()
	return nil
}

func (a *App) ForgetCredentials() {
	a.mu.Lock()
	a.cfg.Username, a.cfg.Password = "", ""
	a.info = "Saved login removed"
	a.saveConfigLocked()
	a.mu.Unlock()
}

func (a *App) needRefresh() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.hasCredsLocked() && !a.exp.IsZero() &&
		time.Until(a.exp) < 45*time.Second && time.Since(a.lastLoginTry) > 30*time.Second
}

func (a *App) canRelogin() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.hasCredsLocked() && time.Since(a.lastLoginTry) > 5*time.Second
}

// EnsureToken logs in if there is no usable token and a login is saved.
func (a *App) EnsureToken() error {
	a.mu.Lock()
	need := a.cfg.Token == "" || (!a.exp.IsZero() && time.Until(a.exp) < 45*time.Second)
	has := a.hasCredsLocked()
	a.mu.Unlock()
	if need && has {
		return a.Login()
	}
	return nil
}

// StartAsync logs in first if needed, then starts opening.
func (a *App) StartAsync() {
	go func() {
		if err := a.EnsureToken(); err != nil {
			return // Login already recorded the reason in the window
		}
		a.Start()
	}()
}

var loginPaths = []string{"/api/login", "/api/auth/login", "/auth/login", "/login"}
var loginFields = []string{"name", "username", "user", "email"}

// Login signs in with the saved username/password and stores the token it gets back.
// The site's login endpoint isn't documented, so the usual paths are tried and the
// one that works is remembered in config.json (loginPath / loginField).
func (a *App) Login() (err error) {
	a.mu.Lock()
	user, pass, ok := a.credsLocked()
	a.lastLoginTry = time.Now()
	base := a.cfg.BaseURL
	paths, fields := loginPaths, loginFields
	if a.cfg.LoginPath != "" {
		paths = []string{a.cfg.LoginPath}
	}
	if a.cfg.LoginField != "" {
		fields = []string{a.cfg.LoginField}
	}
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		if err != nil {
			a.info = "Login failed: " + err.Error()
		} else {
			a.info = "Logged in as " + user
		}
		a.mu.Unlock()
		if err != nil {
			a.logf("login failed: %v", err)
		}
	}()

	if !ok {
		return errNoCreds
	}
	var tried []string
	var notAccepted bool
	var rej *loginRejected
	var blocked string
	single := len(paths) == 1 && len(fields) == 1 // an address that worked before: a "no" is a real "no"
	for _, p := range paths {
		for _, f := range fields {
			status, tok, body, e := a.tryLogin(base+p, f, user, pass)
			if e != nil {
				return fmt.Errorf("could not reach the site: %v", e)
			}
			if tok != "" {
				a.mu.Lock()
				a.setTokenLocked(tok)
				a.notice = ""
				a.mu.Unlock()
				// make sure the site really accepts what login handed back
				if verr := a.verifyToken(); verr != nil {
					notAccepted = true
					tried = append(tried, fmt.Sprintf("%s:%s=token not accepted", p, f))
					continue
				}
				a.mu.Lock()
				if a.cfg.LoginPath != p || a.cfg.LoginField != f {
					a.cfg.LoginPath, a.cfg.LoginField = p, f
					a.saveConfigLocked()
				}
				a.mu.Unlock()
				return nil
			}
			switch {
			case status == 401 || (status == 403 && looksLikeBadLogin(body)):
				r := loginRejected{status, body}
				if single {
					return r
				}
				// the first address or field name tried may simply be the wrong one: keep looking
				if rej == nil {
					rej = &r
				}
			case status == 403:
				blocked = fmt.Sprintf("the site refused the login (HTTP 403%s)", body)
			}
			tried = append(tried, fmt.Sprintf("%s:%s=%d", p, f, status))
		}
	}
	if rej != nil {
		return *rej
	}
	if blocked != "" {
		return errors.New(blocked + ". It may be blocking quick repeated logins: wait a minute")
	}
	if notAccepted {
		return fmt.Errorf("login returned a token but the site says it is unauthorized (%s)", strings.Join(tried, " "))
	}
	return fmt.Errorf("no token came back (tried %s). The login address may differ: set loginPath in config.json", strings.Join(tried, " "))
}

// verifyToken asks the site for the capsule list to confirm the current token works.
func (a *App) verifyToken() error {
	resp, err := a.do("GET", "/data/trians", nil)
	if err != nil {
		return nil // a network hiccup is not proof that the token is bad
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if isAuthFailure(resp.StatusCode, raw) {
		return authError{resp.StatusCode}
	}
	return nil
}

func (a *App) tryLogin(url, userField, user, pass string) (status int, token string, reply string, err error) {
	body, _ := json.Marshal(map[string]string{userField: user, "password": pass})
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return 0, "", "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return 0, "", "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, "", snippet(b), nil
	}
	// The token may be in the JSON body, a header, or a cookie.
	var sb strings.Builder
	sb.Write(b)
	for k, vs := range resp.Header {
		if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "Set-Cookie") || strings.HasPrefix(strings.ToLower(k), "x-") {
			for _, v := range vs {
				sb.WriteString("\n")
				sb.WriteString(v)
			}
		}
	}
	return resp.StatusCode, jwtRe.FindString(sb.String()), "", nil
}
