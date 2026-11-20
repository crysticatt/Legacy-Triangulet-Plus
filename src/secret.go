package main

import (
	"encoding/json"
	"strings"
)

// Saved passwords and the login token are not kept as plain text in config.json.
// Windows: encrypted with the Windows login (DPAPI), so the file only works for this user on this PC.
// Mac: stored in the login Keychain; config.json keeps only a marker.
// If protecting fails, the value stays plain (the app must never lose a login), and a plain value
// found in config.json is protected the next time the file is saved.

var (
	sealImpl   func(label, plain string) (string, bool) // returns the stored form
	unsealImpl func(label, stored string) (string, bool)
)

const secMark = "protected:"

func sealSecret(label, plain string) string {
	if plain == "" || sealImpl == nil || strings.HasPrefix(plain, secMark) {
		return plain
	}
	if s, ok := sealImpl(label, plain); ok {
		return secMark + s
	}
	return plain
}

func openSecret(label, stored string) (string, bool) {
	if !strings.HasPrefix(stored, secMark) {
		return stored, true // plain (older file or protection not available)
	}
	if unsealImpl == nil {
		return "", false
	}
	return unsealImpl(label, strings.TrimPrefix(stored, secMark))
}

// sealConfig returns a copy of cfg that is safe to write to disk.
func sealConfig(cfg Config) Config {
	cfg.Password = sealSecret("password", cfg.Password)
	cfg.Token = sealSecret("token", cfg.Token)
	acc := make([]Account, len(cfg.Accounts))
	for i, a := range cfg.Accounts {
		acc[i] = Account{Name: a.Name, Password: sealSecret("acct:"+strings.ToLower(a.Name), a.Password)}
	}
	cfg.Accounts = acc
	return cfg
}

// openConfig undoes sealConfig after loading. It reports whether anything could not be read.
func openConfig(cfg *Config) (failed bool) {
	var ok bool
	if cfg.Password, ok = openSecret("password", cfg.Password); !ok {
		failed = true
	}
	if cfg.Token, ok = openSecret("token", cfg.Token); !ok {
		failed = true
	}
	for i := range cfg.Accounts {
		if cfg.Accounts[i].Password, ok = openSecret("acct:"+strings.ToLower(cfg.Accounts[i].Name), cfg.Accounts[i].Password); !ok {
			failed = true
		}
	}
	return
}

// hasPlainSecret reports whether config.json text holds a non-empty password or token that is not protected yet.
func hasPlainSecret(raw string) bool {
	var c Config
	if json.Unmarshal([]byte(raw), &c) != nil {
		return false
	}
	plain := func(v string) bool { return v != "" && !strings.HasPrefix(v, secMark) }
	if plain(c.Password) || plain(c.Token) {
		return true
	}
	for _, a := range c.Accounts {
		if plain(a.Password) {
			return true
		}
	}
	return false
}
