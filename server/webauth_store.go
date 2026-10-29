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
	webAuthFileName         = "web-auth.json"
	webPasswordMinLen       = 12
	webLoginIdleLimit       = 90 * 24 * time.Hour
	webLoginWriteInterval   = time.Minute
	webCookieBytes          = 32
	webLoginIDBytes         = 16
	argonTime               = 2
	argonMemoryKiB          = 19 * 1024
	argonThreads            = 1
	argonKeyLen             = 32
	argonSaltLen            = 16
	pendingAuthenticatorTTL = 10 * time.Minute
	pendingIDBytes          = 16
	totpAccountLabel        = "web"
)

var (
	// ErrWebAuthRefused is the one refusal for a wrong password, TOTP code or Setup code.
	ErrWebAuthRefused = errors.New("web auth refused")
	// ErrTOTPMismatch is ErrWebAuthRefused for the setup code that does not match the secret.
	ErrTOTPMismatch = fmt.Errorf("totp code does not match: %w", ErrWebAuthRefused)
	// ErrSetupNotPending means a password is already set.
	ErrSetupNotPending = errors.New("web auth setup is not pending")
	// ErrPasswordTooShort means the password breaks the length rule.
	ErrPasswordTooShort = errors.New("password is too short")
	// ErrWebLoginNotFound means no web login has the given id.
	ErrWebLoginNotFound = errors.New("web login not found")
	// ErrWebLoginNameInvalid means the name is empty or too long.
	ErrWebLoginNameInvalid = errors.New("web login name is empty or too long")
	// ErrSetupCodeRequired means login needs the Setup code because the authenticator was cleared.
	ErrSetupCodeRequired = errors.New("setup code required")
)

// AuthenticatorNeededError is Login's answer when the password is right but
// the account has no saved authenticator. AddAuthenticator takes PendingID and
// a code for Secret.
type AuthenticatorNeededError struct {
	PendingID string
	Secret    string
	URI       string
}

func (e *AuthenticatorNeededError) Error() string { return "authenticator needed" }

type pendingAuthenticator struct {
	secret  string
	created time.Time
}

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
	Password     *webPasswordHash `json:"password,omitempty"`
	TOTPSecret   string           `json:"totp_secret,omitempty"`
	LastTOTPStep int64            `json:"last_totp_step"`
	// AuthenticatorResetPending is set by ClearAuthenticator and cleared when a new authenticator is added.
	AuthenticatorResetPending bool              `json:"authenticator_reset_pending,omitempty"`
	Logins                    []*webLoginRecord `json:"logins"`
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
	// resetSecret is the new TOTP secret from AuthenticatorResetBegin, not saved until finish.
	resetSecret string
	// pending holds the secrets Login handed out, by pending id.
	pending map[string]pendingAuthenticator
}

// OpenWebAuth loads the store in stateDir, or starts empty when it has no file.
func OpenWebAuth(stateDir string, now func() time.Time) (*WebAuth, error) {
	f, err := loadWebAuthFile(stateDir)
	if err != nil {
		return nil, err
	}
	return &WebAuth{dir: stateDir, now: now, limiter: newAuthLimiter(now), file: f, pending: map[string]pendingAuthenticator{}}, nil
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
	pending, code, _, err = w.SetupStateWithExpiry()
	return pending, code, err
}

// SetupStateWithExpiry is SetupState plus the moment the Setup code stops working.
func (w *WebAuth) SetupStateWithExpiry() (pending bool, code string, expires time.Time, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file.Password != nil {
		return false, "", time.Time{}, nil
	}
	code, err = w.currentSetupCodeLocked()
	if err != nil {
		return false, "", time.Time{}, err
	}
	return true, formatSetupCode(code), w.setupCreated.Add(setupCodeLifetime), nil
}

// AuthenticatorResetPending reports whether the CLI reset cleared the
// authenticator and no new one was added yet.
func (w *WebAuth) AuthenticatorResetPending() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.AuthenticatorResetPending
}

// SetupStart checks the Setup code and returns a new TOTP secret tied to it.
// With TOTP off it returns no secret.
func (w *WebAuth) SetupStart(totpOn bool, code, ip string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkSetupCodeLocked(code, ip); err != nil {
		return "", err
	}
	if !totpOn {
		return "", nil
	}
	secret, err := newTOTPSecret()
	if err != nil {
		return "", err
	}
	w.pendingSecret, w.pendingCode = secret, w.setupCode
	return secret, nil
}

// SetupFinish checks the Setup code, password and, with TOTP on, one TOTP code,
// saves them and returns the first web login's cookie. Nothing is saved when
// any check fails.
func (w *WebAuth) SetupFinish(totpOn bool, code, password, totpCode, ip, userAgent string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.checkSetupCodeLocked(code, ip); err != nil {
		return "", err
	}
	var secret string
	var step int64
	if totpOn {
		if w.pendingSecret == "" || w.pendingCode != w.setupCode {
			return "", ErrWebAuthRefused
		}
		secret = w.pendingSecret
	}
	if err := checkWebPassword(password); err != nil {
		return "", err
	}
	if totpOn {
		var ok bool
		if step, ok = verifyTOTP(secret, totpCode, w.now(), 0); !ok {
			w.limiter.Fail(ip)
			return "", ErrTOTPMismatch
		}
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
		TOTPSecret:   secret,
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

// Login checks the password and, with TOTP on, a TOTP code and returns a new
// web login's cookie. With TOTP on and no saved secret, a correct password
// answers with an *AuthenticatorNeededError instead. While the authenticator
// reset is pending (TOTP on), the Setup code is needed first.
func (w *WebAuth) Login(totpOn bool, password, totpCode, setupCode, ip, userAgent string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if wait := w.limiter.Wait(ip); wait > 0 {
		return "", &WebAuthWaitError{Seconds: wait}
	}
	if w.file.Password == nil {
		return "", ErrWebAuthRefused
	}
	gated := totpOn && w.file.AuthenticatorResetPending
	if gated {
		if setupCode == "" {
			return "", ErrSetupCodeRequired
		}
		if err := w.compareSetupCodeLocked(setupCode, ip); err != nil {
			return "", err
		}
	}
	if totpOn && (gated || w.file.TOTPSecret == "") {
		if !w.passwordOKLocked(password) {
			w.limiter.Fail(ip)
			return "", ErrWebAuthRefused
		}
		return "", w.newPendingAuthenticatorLocked()
	}
	step, err := w.checkCredentialsLocked(totpOn, password, totpCode, ip)
	if err != nil {
		return "", err
	}
	next := w.file
	next.LastTOTPStep = step
	return w.addLoginLocked(next, ip, userAgent)
}

// AddAuthenticator takes a pending id from Login and one code for its secret,
// saves the secret and returns a new web login's cookie. A pending id works
// once and for 10 minutes. A wrong code saves nothing and counts as a failure.
func (w *WebAuth) AddAuthenticator(pendingID, totpCode, ip, userAgent string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if wait := w.limiter.Wait(ip); wait > 0 {
		return "", &WebAuthWaitError{Seconds: wait}
	}
	p, ok := w.pending[pendingID]
	if ok && w.now().Sub(p.created) >= pendingAuthenticatorTTL {
		delete(w.pending, pendingID)
		ok = false
	}
	if !ok {
		return "", ErrWebAuthRefused
	}
	step, ok := verifyTOTP(p.secret, totpCode, w.now(), 0)
	if !ok {
		w.limiter.Fail(ip)
		return "", ErrWebAuthRefused
	}
	next := w.file
	next.TOTPSecret, next.LastTOTPStep, next.AuthenticatorResetPending = p.secret, step, false
	cookie, err := w.addLoginLocked(next, ip, userAgent)
	if err != nil {
		return "", err
	}
	delete(w.pending, pendingID)
	w.setupCode = ""
	return cookie, nil
}

// ClearAuthenticator is the CLI reset: it clears the TOTP secret, marks the
// reset pending, signs out every web login and returns their ids with a new
// Setup code.
func (w *WebAuth) ClearAuthenticator() (removed []string, setupCode string, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	next, removed := signOutExcept(w.file, func(*webLoginRecord) bool { return false })
	next.TOTPSecret, next.LastTOTPStep, next.AuthenticatorResetPending = "", 0, true
	if err := w.commitLocked(next); err != nil {
		return nil, "", err
	}
	w.pending = map[string]pendingAuthenticator{}
	w.resetSecret = ""
	w.setupCode = ""
	code, err := w.currentSetupCodeLocked()
	if err != nil {
		return nil, "", err
	}
	return removed, formatSetupCode(code), nil
}

// addLoginLocked saves next with a new web login and returns its cookie.
func (w *WebAuth) addLoginLocked(next webAuthFile, ip, userAgent string) (string, error) {
	cookie, rec, err := newWebLoginRecord(userAgent, ip, w.now())
	if err != nil {
		return "", err
	}
	next.Logins = append(w.file.Logins[:len(w.file.Logins):len(w.file.Logins)], rec)
	if err := w.commitLocked(next); err != nil {
		return "", err
	}
	w.limiter.Success(ip)
	return cookie, nil
}

// newPendingAuthenticatorLocked holds a new secret in memory and returns the
// answer that hands it out.
func (w *WebAuth) newPendingAuthenticatorLocked() error {
	now := w.now()
	for id, p := range w.pending {
		if now.Sub(p.created) >= pendingAuthenticatorTTL {
			delete(w.pending, id)
		}
	}
	secret, err := newTOTPSecret()
	if err != nil {
		return err
	}
	raw := make([]byte, pendingIDBytes)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	id := hex.EncodeToString(raw)
	w.pending[id] = pendingAuthenticator{secret: secret, created: now}
	return &AuthenticatorNeededError{PendingID: id, Secret: secret, URI: totpURI(secret, totpAccountLabel)}
}

func (w *WebAuth) passwordOKLocked(password string) bool {
	return checkWebPassword(password) == nil && verifyWebPassword(w.file.Password, password)
}

// checkCredentialsLocked applies the rate limit, then checks the password and,
// with TOTP on, a code from the current secret. It returns the code's step (the
// last one with TOTP off). A wrong check counts as a failure.
func (w *WebAuth) checkCredentialsLocked(totpOn bool, password, totpCode, ip string) (int64, error) {
	if wait := w.limiter.Wait(ip); wait > 0 {
		return 0, &WebAuthWaitError{Seconds: wait}
	}
	if w.file.Password == nil {
		return 0, ErrWebAuthRefused
	}
	// With no saved secret there is no code to ask for ("Add authenticator").
	totpOn = totpOn && w.file.TOTPSecret != ""
	// Both checks always run so a refusal does not show which part was wrong.
	passOK := w.passwordOKLocked(password)
	step, totpOK := w.file.LastTOTPStep, true
	if totpOn {
		step, totpOK = verifyTOTP(w.file.TOTPSecret, totpCode, w.now(), w.file.LastTOTPStep)
	}
	if !passOK || !totpOK {
		w.limiter.Fail(ip)
		return 0, ErrWebAuthRefused
	}
	return step, nil
}

// RenameLogin sets the name of one web login.
func (w *WebAuth) RenameLogin(id, name string) error {
	if n := utf8.RuneCountInString(name); n == 0 || n > deviceNameMaxLen {
		return ErrWebLoginNameInvalid
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	next := w.file
	next.Logins = make([]*webLoginRecord, len(w.file.Logins))
	found := false
	for i, rec := range w.file.Logins {
		next.Logins[i] = rec
		if rec.ID == id {
			renamed := *rec
			renamed.Name = name
			next.Logins[i] = &renamed
			found = true
		}
	}
	if !found {
		return ErrWebLoginNotFound
	}
	return w.commitLocked(next)
}

// RevokeLogin signs out one web login and returns its id.
func (w *WebAuth) RevokeLogin(id string) ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	next, removed := signOutExcept(w.file, func(rec *webLoginRecord) bool { return rec.ID != id })
	if len(removed) == 0 {
		return nil, ErrWebLoginNotFound
	}
	if err := w.commitLocked(next); err != nil {
		return nil, err
	}
	return removed, nil
}

// ChangePassword checks the current password and a TOTP code, saves the new
// password and signs out every web login except keepID. It returns the ids
// signed out.
func (w *WebAuth) ChangePassword(totpOn bool, current, totpCode, newPassword, keepID, ip string) ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := checkWebPassword(newPassword); err != nil {
		return nil, err
	}
	step, err := w.checkCredentialsLocked(totpOn, current, totpCode, ip)
	if err != nil {
		return nil, err
	}
	hash, err := hashWebPassword(newPassword)
	if err != nil {
		return nil, err
	}
	next, removed := signOutExcept(w.file, func(rec *webLoginRecord) bool { return rec.ID == keepID })
	next.Password, next.LastTOTPStep = hash, step
	if err := w.commitLocked(next); err != nil {
		return nil, err
	}
	w.limiter.Success(ip)
	return removed, nil
}

// SetPassword is the CLI path: it saves the new password without checks and
// signs out every web login. It returns the ids signed out.
func (w *WebAuth) SetPassword(newPassword string) ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := checkWebPassword(newPassword); err != nil {
		return nil, err
	}
	hash, err := hashWebPassword(newPassword)
	if err != nil {
		return nil, err
	}
	next, removed := signOutExcept(w.file, func(*webLoginRecord) bool { return false })
	next.Password = hash
	if err := w.commitLocked(next); err != nil {
		return nil, err
	}
	return removed, nil
}

// AuthenticatorResetBegin checks the password and a TOTP code from the current
// secret and returns a new pending secret. The old secret keeps working until
// AuthenticatorResetFinish succeeds.
func (w *WebAuth) AuthenticatorResetBegin(totpOn bool, password, totpCode, ip string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	step, err := w.checkCredentialsLocked(totpOn, password, totpCode, ip)
	if err != nil {
		return "", err
	}
	secret, err := newTOTPSecret()
	if err != nil {
		return "", err
	}
	next := w.file
	next.LastTOTPStep = step
	if err := w.commitLocked(next); err != nil {
		return "", err
	}
	w.resetSecret = secret
	w.limiter.Success(ip)
	return secret, nil
}

// AuthenticatorResetFinish checks a code from the pending secret, saves the
// secret and signs out every web login except keepID. It returns the ids
// signed out.
func (w *WebAuth) AuthenticatorResetFinish(totpCode, keepID, ip string) ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if wait := w.limiter.Wait(ip); wait > 0 {
		return nil, &WebAuthWaitError{Seconds: wait}
	}
	if w.resetSecret == "" {
		return nil, ErrWebAuthRefused
	}
	if _, ok := verifyTOTP(w.resetSecret, totpCode, w.now(), 0); !ok {
		w.limiter.Fail(ip)
		return nil, ErrWebAuthRefused
	}
	next, removed := signOutExcept(w.file, func(rec *webLoginRecord) bool { return rec.ID == keepID })
	next.TOTPSecret, next.LastTOTPStep = w.resetSecret, 0
	if err := w.commitLocked(next); err != nil {
		return nil, err
	}
	w.resetSecret = ""
	w.limiter.Success(ip)
	return removed, nil
}

// WebReset clears the password, TOTP secret, web logins and rate limits, and
// makes a new Setup code. It returns the ids signed out.
func (w *WebAuth) WebReset() ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	removed := make([]string, 0, len(w.file.Logins))
	for _, rec := range w.file.Logins {
		removed = append(removed, rec.ID)
	}
	if err := w.commitLocked(webAuthFile{}); err != nil {
		return nil, err
	}
	w.limiter.Reset()
	w.setupCode, w.pendingSecret, w.pendingCode, w.resetSecret = "", "", "", ""
	w.pending = map[string]pendingAuthenticator{}
	if _, err := w.currentSetupCodeLocked(); err != nil {
		return nil, err
	}
	return removed, nil
}

// commitLocked saves next and makes it the current state.
func (w *WebAuth) commitLocked(next webAuthFile) error {
	if err := saveWebAuthFile(w.dir, next); err != nil {
		return err
	}
	w.file = next
	return nil
}

// signOutExcept returns a copy of f that keeps only the logins keep accepts,
// and the ids of the others.
func signOutExcept(f webAuthFile, keep func(*webLoginRecord) bool) (webAuthFile, []string) {
	kept := make([]*webLoginRecord, 0, len(f.Logins))
	removed := []string{}
	for _, rec := range f.Logins {
		if keep(rec) {
			kept = append(kept, rec)
		} else {
			removed = append(removed, rec.ID)
		}
	}
	f.Logins = kept
	return f, removed
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
	return w.compareSetupCodeLocked(input, ip)
}

// compareSetupCodeLocked compares the Setup code. A wrong code counts as a failure.
func (w *WebAuth) compareSetupCodeLocked(input, ip string) error {
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
