package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// A backup is one file with your settings, saved logins, History and pull log. It is encrypted with a
// passphrase you choose (AES-256-GCM, key from PBKDF2-SHA256), so it is safe to keep in the cloud or
// carry to another PC, where the saved passwords inside can be read again.

const (
	backupMagic = "TRIBAK1\n"
	backupIters = 200_000
	minPassLen  = 6
)

type backupData struct {
	Version int             `json:"version"` // app version that wrote it
	Made    string          `json:"made"`
	Config  Config          `json:"config"`
	History json.RawMessage `json:"history,omitempty"`
	PullLog string          `json:"pullLog,omitempty"`
}

func pbkdf2(pass, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(sha256.New, pass)
	var out []byte
	for block := 1; len(out) < keyLen; block++ {
		prf.Reset()
		prf.Write(salt)
		var idx [4]byte
		binary.BigEndian.PutUint32(idx[:], uint32(block))
		prf.Write(idx[:])
		u := prf.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iter; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func sealBackup(plain []byte, pass string) ([]byte, error) {
	if len([]rune(pass)) < minPassLen {
		return nil, errors.New("the backup passphrase needs at least 6 characters")
	}
	salt := make([]byte, 16)
	nonce := make([]byte, 12)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	blk, err := aes.NewCipher(pbkdf2([]byte(pass), salt, backupIters, 32))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(blk)
	if err != nil {
		return nil, err
	}
	out := append([]byte(backupMagic), salt...)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, plain, []byte(backupMagic)), nil
}

func openBackup(file []byte, pass string) ([]byte, error) {
	hdr := len(backupMagic) + 16 + 12
	if len(file) < hdr+16 || !bytes.HasPrefix(file, []byte(backupMagic)) {
		return nil, errors.New("this is not a Triangulet Opener backup file")
	}
	salt := file[len(backupMagic) : len(backupMagic)+16]
	nonce := file[len(backupMagic)+16 : hdr]
	blk, err := aes.NewCipher(pbkdf2([]byte(pass), salt, backupIters, 32))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(blk)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, file[hdr:], []byte(backupMagic))
	if err != nil {
		return nil, errors.New("wrong passphrase, or the file is damaged")
	}
	return plain, nil
}

// ExportBackup returns the encrypted backup file.
func (h *Hub) ExportBackup(pass string) ([]byte, error) {
	h.saveHistory()
	d := backupData{Version: versionInt(), Made: time.Now().Format(time.RFC3339), Config: h.cfg()}
	if b, err := os.ReadFile(histPath(h.primary.dir)); err == nil && json.Valid(b) {
		d.History = b
	}
	if b, err := os.ReadFile(filepath.Join(h.primary.dir, "pull-log.csv")); err == nil && len(b) < 8<<20 {
		d.PullLog = string(b)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	return sealBackup(raw, pass)
}

// ImportBackup replaces the settings, saved logins and history with the ones in the file.
func (h *Hub) ImportBackup(file []byte, pass string) error {
	raw, err := openBackup(file, pass)
	if err != nil {
		return err
	}
	var d backupData
	if err := json.Unmarshal(raw, &d); err != nil || d.Config.BaseURL == "" {
		return errors.New("the backup file has no settings in it")
	}
	if h.Snapshot().Running {
		h.Stop()
	}
	keepDir := h.primary.dir
	h.update(func(c *Config) { *c = d.Config })
	if len(d.History) > 0 {
		var list []HistBucket
		if json.Unmarshal(d.History, &list) == nil {
			sort.Slice(list, func(i, j int) bool { return list[i].T < list[j].T })
			h.histMu.Lock()
			h.hist, h.histDirty = list, true
			h.histMu.Unlock()
			h.saveHistory()
		}
	}
	if d.PullLog != "" && !strings.Contains(d.PullLog, "\x00") {
		p := filepath.Join(keepDir, "pull-log.csv")
		if _, err := os.Stat(p); os.IsNotExist(err) { // never overwrite a log that already has entries
			_ = os.WriteFile(p, []byte(d.PullLog), 0o600)
		}
	}
	return nil
}

// backupFolder is where a backup is written and looked for.
func backupFolders() []string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return nil
	}
	return []string{filepath.Join(home, "Desktop"), filepath.Join(home, "Downloads"), filepath.Join(home, "Documents")}
}

func backupFileName() string {
	return "TrianguletOpener-backup-" + time.Now().Format("2006-01-02") + ".tribackup"
}

// SaveBackupFile writes a backup to the Desktop (or the home folder) and returns where it went.
func (h *Hub) SaveBackupFile(pass string) (string, error) {
	data, err := h.ExportBackup(pass)
	if err != nil {
		return "", err
	}
	dirs := backupFolders()
	if len(dirs) == 0 {
		dirs = []string{h.primary.dir}
	}
	var last error
	for _, d := range append(dirs[:1], h.primary.dir) {
		p := filepath.Join(d, backupFileName())
		if err := os.WriteFile(p, data, 0o600); err != nil {
			last = err
			continue
		}
		return p, nil
	}
	return "", last
}

// LoadBackupFile imports the newest .tribackup file found on the Desktop, Downloads or Documents.
func (h *Hub) LoadBackupFile(pass string) (string, error) {
	var best string
	var bt time.Time
	for _, d := range backupFolders() {
		list, _ := filepath.Glob(filepath.Join(d, "*.tribackup"))
		for _, p := range list {
			if st, err := os.Stat(p); err == nil && st.ModTime().After(bt) {
				best, bt = p, st.ModTime()
			}
		}
	}
	if best == "" {
		return "", errors.New("no .tribackup file found on your Desktop, Downloads or Documents. Copy it there and try again")
	}
	data, err := os.ReadFile(best)
	if err != nil {
		return "", err
	}
	return best, h.ImportBackup(data, pass)
}

func versionInt() int {
	if v := parseVer(appVersion); len(v) > 0 {
		return v[0]
	}
	return 0
}
