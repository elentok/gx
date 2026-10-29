package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	webAuthFileName       = "web-auth.json"
	webPasswordMinLen     = 12
	webLoginIdleLimit     = 90 * 24 * time.Hour
	webLoginWriteInterval = time.Minute
	webCookieBytes        = 32
	webLoginIDBytes       = 16
	argonTime             = 2
	argonMemoryKiB        = 19 * 1024
	argonThreads          = 1
	argonKeyLen           = 32
	argonSaltLen          = 16
)

var (
	// ErrWebAuthRefused is the one refusal for a wrong password, TOTP code or Setup code.
	ErrWebAuthRefused = errors.New("web auth refused")
	// ErrSetupNotPending means a password is already set.
	ErrSetupNotPending = errors.New("web auth setup is not pending")
	// ErrPasswordTooShort means the password breaks the length rule.
	ErrPasswordTooShort = errors.New("password is too short")
)

// WebAuthWaitError means the caller is rate limited.
type WebAuthWaitError struct{ Seconds int }

func (e *WebAuthWaitError) Error() string {
	return fmt.Sprintf("too many attempts, wait %d seconds", e.Seconds)
}

// WebLogin is one signed-in browser, as shown in the device list.
type WebLogin struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UserAgent string    `json:"user_agent"`
	Created   time.Time `json:"created"`
	LastUsed  time.Time `json:"last_used"`
	LastIP    string    `json:"last_ip"`
}

type webLoginRecord struct {
	WebLogin
	CookieHash string `json:"cookie_hash"`
	// savedAt is the last used time on disk, to write at most once a minute.
	savedAt time.Time
}

type webPasswordHash struct {
	Hash    []byte `json:"hash"`
	Salt    []byte `json:"salt"`
	Time    uint32 `json:"time"`
	MemKiB  uint32 `json:"memory_kib"`
	Threads uint8  `json:"threads"`
}

type webAuthFile struct {
	Password     *webPasswordHash  `json:"password,omitempty"`
	TOTPSecret   string            `json:"totp_secret,omitempty"`
	LastTOTPStep int64             `json:"last_totp_step"`
	Logins       []*webLoginRecord `json:"logins"`
}

// WebAuth is the web auth store. All state lives in one file in the state dir.
type WebAuth struct {
	dir     string
	now     func() time.Time
	limiter *authLimiter

	mu           sync.Mutex
	file         webAuthFile
	setupCode    string
	setupCreated time.Time
	// pendingSecret is the TOTP secret handed out by SetupStart, tied to pendingCode.
	pendingSecret string
	pendingCode   string
}

// OpenWebAuth loads the store in stateDir, or starts empty when it has no file.
func OpenWebAuth(stateDir string, now func() time.Time) (*WebAuth, error) {
	f, err := loadWebAuthFile(stateDir)
	if err != nil {
		return nil, err
	}
	return &WebAuth{dir: stateDir, now: now, limiter: newAuthLimiter(now), file: f}, nil
}

// SeedWebLogin adds a web login to the store in stateDir before the server
// starts. It returns the login's cookie value.
func SeedWebLogin(stateDir, userAgent string, lastUsed time.Time) (string, error) {
	f, err := loadWebAuthFile(stateDir)
	if err != nil {
		return "", err
	}
	cookie, rec, err := newWebLoginRecord(userAgent, "", lastUsed)
	if err != nil {
		return "", err
	}
	f.Logins = append(f.Logins, rec)
	if err := saveWebAuthFile(stateDir, f); err != nil {
		return "", err
	}
	return cookie, nil
}

// SetupState reports whether setup is pending and, if so, the Setup code to show.
func (w *WebAuth) SetupState() (pending bool, code string, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file.Password != nil {
		return false, "", nil
	}
	code, err = w.currentSetupCodeLocked()
	if err != nil {
		return false, "", err
	}
	return true, formatSetupCode(code), nil
}

// SetupStart checks the Setup code and returns a new TOTP secret tied to it.
func (w *WebAuth) SetupStart(code, ip string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkSetupCodeLocked(code, ip); err != nil {
		return "", err
	}
	secret, err := newTOTPSecret()
	if err != nil {
		return "", err
	}
	w.pendingSecret, w.pendingCode = secret, w.setupCode
	return secret, nil
}

// SetupFinish checks the Setup code, password and one TOTP code, saves them and
// returns the first web login's cookie. Nothing is saved when any check fails.
func (w *WebAuth) SetupFinish(code, password, totpCode, ip, userAgent string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkSetupCodeLocked(code, ip); err != nil {
		return "", err
	}
	if w.pendingSecret == "" || w.pendingCode != w.setupCode {
		return "", ErrWebAuthRefused
	}
	if err := checkWebPassword(password); err != nil {
		return "", err
	}
	step, ok := verifyTOTP(w.pendingSecret, totpCode, w.now(), 0)
	if !ok {
		w.limiter.Fail(ip)
		return "", ErrWebAuthRefused
	}
	hash, err := hashWebPassword(password)
	if err != nil {
		return "", err
	}
	cookie, rec, err := newWebLoginRecord(userAgent, ip, w.now())
	if err != nil {
		return "", err
	}
	next := webAuthFile{
		Password:     hash,
		TOTPSecret:   w.pendingSecret,
		LastTOTPStep: step,
		Logins:       append(w.file.Logins[:len(w.file.Logins):len(w.file.Logins)], rec),
	}
	if err := saveWebAuthFile(w.dir, next); err != nil {
		return "", err
	}
	w.file = next
	w.setupCode, w.pendingSecret, w.pendingCode = "", "", ""
	w.limiter.Success(ip)
	return cookie, nil
}

// Login checks the password and a TOTP code and returns a new web login's cookie.
func (w *WebAuth) Login(password, totpCode, ip, userAgent string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if wait := w.limiter.Wait(ip); wait > 0 {
		return "", &WebAuthWaitError{Seconds: wait}
	}
	if w.file.Password == nil {
		return "", ErrWebAuthRefused
	}
	// Both checks always run so a refusal does not show which part was wrong.
	passOK := checkWebPassword(password) == nil && verifyWebPassword(w.file.Password, password)
	step, totpOK := verifyTOTP(w.file.TOTPSecret, totpCode, w.now(), w.file.LastTOTPStep)
	if !passOK || !totpOK {
		w.limiter.Fail(ip)
		return "", ErrWebAuthRefused
	}
	cookie, rec, err := newWebLoginRecord(userAgent, ip, w.now())
	if err != nil {
		return "", err
	}
	next := w.file
	next.LastTOTPStep = step
	next.Logins = append(w.file.Logins[:len(w.file.Logins):len(w.file.Logins)], rec)
	if err := saveWebAuthFile(w.dir, next); err != nil {
		return "", err
	}
	w.file = next
	w.limiter.Success(ip)
	return cookie, nil
}

// Lookup finds the web login for a cookie and renews its last used time. A web
// login idle for 90 days is refused and dropped.
func (w *WebAuth) Lookup(cookie, ip string) (WebLogin, bool) {
	sum := sha256.Sum256([]byte(cookie))
	w.mu.Lock()
	defer w.mu.Unlock()
	var found *webLoginRecord
	for _, rec := range w.file.Logins {
		want, err := hex.DecodeString(rec.CookieHash)
		if err == nil && subtle.ConstantTimeCompare(want, sum[:]) == 1 {
			found = rec
		}
	}
	if found == nil {
		return WebLogin{}, false
	}
	now := w.now()
	if now.Sub(found.LastUsed) >= webLoginIdleLimit {
		w.dropLoginLocked(found.ID)
		return WebLogin{}, false
	}
	found.LastUsed, found.LastIP = now, ip
	if now.Sub(found.savedAt) >= webLoginWriteInterval {
		if err := saveWebAuthFile(w.dir, w.file); err == nil {
			for _, rec := range w.file.Logins {
				rec.savedAt = rec.LastUsed
			}
		}
	}
	return found.WebLogin, true
}

// List returns the web logins, oldest first.
func (w *WebAuth) List() []WebLogin {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]WebLogin, 0, len(w.file.Logins))
	for _, rec := range w.file.Logins {
		out = append(out, rec.WebLogin)
	}
	return out
}

func (w *WebAuth) dropLoginLocked(id string) {
	kept := make([]*webLoginRecord, 0, len(w.file.Logins))
	for _, rec := range w.file.Logins {
		if rec.ID != id {
			kept = append(kept, rec)
		}
	}
	next := w.file
	next.Logins = kept
	// A failed write leaves the login in the file; a new module drops it again on lookup.
	_ = saveWebAuthFile(w.dir, next)
	w.file = next
}

// currentSetupCodeLocked returns the pending code, making a new one when there
// is none or it expired. A new code drops any TOTP secret tied to the old one.
func (w *WebAuth) currentSetupCodeLocked() (string, error) {
	if w.setupCode != "" && !setupCodeExpired(w.setupCreated, w.now()) {
		return w.setupCode, nil
	}
	code, err := newSetupCode()
	if err != nil {
		return "", err
	}
	w.setupCode, w.setupCreated = code, w.now()
	w.pendingSecret, w.pendingCode = "", ""
	return code, nil
}

// checkSetupCodeLocked applies the rate limit and compares the Setup code.
func (w *WebAuth) checkSetupCodeLocked(input, ip string) error {
	if wait := w.limiter.Wait(ip); wait > 0 {
		return &WebAuthWaitError{Seconds: wait}
	}
	if w.file.Password != nil {
		return ErrSetupNotPending
	}
	current, err := w.currentSetupCodeLocked()
	if err != nil {
		return err
	}
	parsed, ok := parseSetupCode(input)
	if !ok || subtle.ConstantTimeCompare([]byte(parsed), []byte(current)) != 1 {
		w.limiter.Fail(ip)
		return ErrWebAuthRefused
	}
	return nil
}

func checkWebPassword(password string) error {
	if utf8.RuneCountInString(password) < webPasswordMinLen {
		return ErrPasswordTooShort
	}
	return nil
}

func hashWebPassword(password string) (*webPasswordHash, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	h := &webPasswordHash{Salt: salt, Time: argonTime, MemKiB: argonMemoryKiB, Threads: argonThreads}
	h.Hash = argon2.IDKey([]byte(password), salt, h.Time, h.MemKiB, h.Threads, argonKeyLen)
	return h, nil
}

func verifyWebPassword(h *webPasswordHash, password string) bool {
	got := argon2.IDKey([]byte(password), h.Salt, h.Time, h.MemKiB, h.Threads, uint32(len(h.Hash)))
	return subtle.ConstantTimeCompare(got, h.Hash) == 1
}

// newWebLoginRecord makes a record and the cookie value it belongs to. The
// record keeps only the cookie's SHA-256, and its id is random, not derived from it.
func newWebLoginRecord(userAgent, ip string, at time.Time) (string, *webLoginRecord, error) {
	raw := make([]byte, webCookieBytes)
	id := make([]byte, webLoginIDBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	if _, err := rand.Read(id); err != nil {
		return "", nil, err
	}
	cookie := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(cookie))
	return cookie, &webLoginRecord{
		WebLogin: WebLogin{
			ID:        hex.EncodeToString(id),
			Name:      deviceName(userAgent),
			UserAgent: userAgent,
			Created:   at,
			LastUsed:  at,
			LastIP:    ip,
		},
		CookieHash: hex.EncodeToString(sum[:]),
		savedAt:    at,
	}, nil
}

func loadWebAuthFile(stateDir string) (webAuthFile, error) {
	var f webAuthFile
	data, err := os.ReadFile(filepath.Join(stateDir, webAuthFileName))
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return f, fmt.Errorf("read %s: %w", webAuthFileName, err)
	}
	for _, rec := range f.Logins {
		rec.savedAt = rec.LastUsed
	}
	return f, nil
}

// saveWebAuthFile writes a temp file and renames it, so a reader never sees a half-written file.
func saveWebAuthFile(stateDir string, f webAuthFile) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(stateDir, webAuthFileName+".tmp*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(stateDir, webAuthFileName)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
