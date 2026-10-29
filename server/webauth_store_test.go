package server

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	storeTestPassword = "correct horse battery"
	storeTestUA       = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1"
)

type storeClock struct{ t time.Time }

func (c *storeClock) Now() time.Time { return c.t }

func newStoreForTest(t *testing.T) (*WebAuth, *storeClock, string) {
	t.Helper()
	dir := t.TempDir()
	clock := &storeClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	return openStoreForTest(t, dir, clock), clock, dir
}

func openStoreForTest(t *testing.T, dir string, clock *storeClock) *WebAuth {
	t.Helper()
	w, err := OpenWebAuth(dir, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// finishStoreSetup runs setup and returns the first cookie and the TOTP secret.
func finishStoreSetup(t *testing.T, w *WebAuth, clock *storeClock) (cookie, secret string) {
	t.Helper()
	_, code, err := w.SetupState()
	if err != nil {
		t.Fatal(err)
	}
	secret, err = w.SetupStart(true, code, "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	totp, _ := TOTPCode(secret, totpStepAt(clock.Now()))
	cookie, err = w.SetupFinish(true, code, storeTestPassword, totp, "1.1.1.1", storeTestUA)
	if err != nil {
		t.Fatal(err)
	}
	return cookie, secret
}

func loginTOTPOn(w *WebAuth, password, totp, ip, userAgent string) (string, error) {
	return w.Login(true, password, totp, "", ip, userAgent)
}

// finishStoreSetupTOTPOff runs setup with TOTP off and returns the first cookie.
func finishStoreSetupTOTPOff(t *testing.T, w *WebAuth) string {
	t.Helper()
	_, code, err := w.SetupState()
	if err != nil {
		t.Fatal(err)
	}
	secret, err := w.SetupStart(false, code, "1.1.1.1")
	if err != nil || secret != "" {
		t.Fatalf("secret = %q, err = %v, want none", secret, err)
	}
	cookie, err := w.SetupFinish(false, code, storeTestPassword, "", "1.1.1.1", storeTestUA)
	if err != nil {
		t.Fatal(err)
	}
	return cookie
}

// neededAuthenticator logs in with TOTP on and returns the "authenticator needed" answer.
func neededAuthenticator(t *testing.T, err error) *AuthenticatorNeededError {
	t.Helper()
	var needed *AuthenticatorNeededError
	if !errors.As(err, &needed) {
		t.Fatalf("err = %v, want authenticator needed", err)
	}
	if needed.PendingID == "" || needed.Secret == "" || !strings.HasPrefix(needed.URI, "otpauth://") {
		t.Fatalf("needed = %+v", needed)
	}
	return needed
}

func TestWebAuthStoreTOTPOffSetupAndLoginNeedPasswordOnly(t *testing.T) {
	w, _, dir := newStoreForTest(t)
	finishStoreSetupTOTPOff(t, w)
	if _, err := w.Login(false, storeTestPassword, "", "", "2.2.2.2", storeTestUA); err != nil {
		t.Fatalf("password-only login: %v", err)
	}
	if _, err := w.Login(false, "wrong password here", "", "", "2.2.2.2", storeTestUA); !errors.Is(err, ErrWebAuthRefused) {
		t.Fatalf("wrong password err = %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, webAuthFileName))
	if strings.Contains(string(data), `"totp_secret"`) {
		t.Fatal("file holds a secret that was never made")
	}
}

func TestWebAuthStoreTurningTOTPBackOnKeepsOldSecret(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	_, secret := finishStoreSetup(t, w, clock)
	if _, err := w.Login(false, storeTestPassword, "", "", "2.2.2.2", storeTestUA); err != nil {
		t.Fatalf("password-only login: %v", err)
	}
	if _, err := loginTOTPOn(w, storeTestPassword, nextCode(clock, secret), "3.3.3.3", storeTestUA); err != nil {
		t.Fatalf("old secret refused: %v", err)
	}
}

func TestWebAuthStoreAddAuthenticatorSavesSecretAndSignsIn(t *testing.T) {
	w, clock, dir := newStoreForTest(t)
	finishStoreSetupTOTPOff(t, w)
	_, err := loginTOTPOn(w, storeTestPassword, "", "2.2.2.2", storeTestUA)
	needed := neededAuthenticator(t, err)

	good, _ := TOTPCode(needed.Secret, totpStepAt(clock.Now()))
	bad := "000000"
	if good == bad {
		bad = "111111"
	}
	if _, err := w.AddAuthenticator(needed.PendingID, bad, "2.2.2.2", storeTestUA); !errors.Is(err, ErrWebAuthRefused) {
		t.Fatalf("wrong code err = %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, webAuthFileName))
	if strings.Contains(string(data), needed.Secret) {
		t.Fatal("wrong code saved the secret")
	}

	cookie, err := w.AddAuthenticator(needed.PendingID, good, "2.2.2.2", storeTestUA)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := w.Lookup(cookie, "2.2.2.2"); !ok {
		t.Fatal("cookie not found")
	}
	if _, err := w.AddAuthenticator(needed.PendingID, good, "2.2.2.2", storeTestUA); !errors.Is(err, ErrWebAuthRefused) {
		t.Fatalf("second use err = %v, want refused", err)
	}
	if _, err := loginTOTPOn(w, storeTestPassword, nextCode(clock, needed.Secret), "3.3.3.3", storeTestUA); err != nil {
		t.Fatalf("login with the new secret: %v", err)
	}
}

func TestWebAuthStoreAddAuthenticatorWrongCodesAreCounted(t *testing.T) {
	w, _, _ := newStoreForTest(t)
	finishStoreSetupTOTPOff(t, w)
	_, err := loginTOTPOn(w, storeTestPassword, "", "2.2.2.2", storeTestUA)
	needed := neededAuthenticator(t, err)
	for i := 0; i < authIPFreeFailures; i++ {
		w.AddAuthenticator(needed.PendingID, "000000", "2.2.2.2", storeTestUA)
	}
	_, err = w.AddAuthenticator(needed.PendingID, "000000", "2.2.2.2", storeTestUA)
	var wait *WebAuthWaitError
	if !errors.As(err, &wait) {
		t.Fatalf("err = %v, want a wait error", err)
	}
}

func TestWebAuthStorePendingAuthenticatorExpiresAndDoesNotSurviveRestart(t *testing.T) {
	w, clock, dir := newStoreForTest(t)
	finishStoreSetupTOTPOff(t, w)
	_, err := loginTOTPOn(w, storeTestPassword, "", "2.2.2.2", storeTestUA)
	old := neededAuthenticator(t, err)
	_, err = loginTOTPOn(w, storeTestPassword, "", "2.2.2.2", storeTestUA)
	restarted := neededAuthenticator(t, err)

	if _, err := openStoreForTest(t, dir, clock).AddAuthenticator(restarted.PendingID, "000000", "2.2.2.2", storeTestUA); !errors.Is(err, ErrWebAuthRefused) {
		t.Fatalf("new module err = %v, want refused", err)
	}
	clock.t = clock.t.Add(pendingAuthenticatorTTL)
	code, _ := TOTPCode(old.Secret, totpStepAt(clock.Now()))
	if _, err := w.AddAuthenticator(old.PendingID, code, "2.2.2.2", storeTestUA); !errors.Is(err, ErrWebAuthRefused) {
		t.Fatalf("expired err = %v, want refused", err)
	}
}

func TestWebAuthStoreWrongPasswordGetsNoPendingAuthenticator(t *testing.T) {
	w, _, _ := newStoreForTest(t)
	finishStoreSetupTOTPOff(t, w)
	if _, err := loginTOTPOn(w, "wrong password here", "", "2.2.2.2", storeTestUA); !errors.Is(err, ErrWebAuthRefused) {
		t.Fatalf("err = %v, want refused", err)
	}
}

func TestWebAuthStoreClearAuthenticatorGatesLoginOnSetupCode(t *testing.T) {
	w, clock, dir := newStoreForTest(t)
	first, second, cookie, _ := twoLogins(t, w, clock)
	removed, setupCode, err := w.ClearAuthenticator()
	if err != nil || len(removed) != 2 || removed[0] != first || removed[1] != second || setupCode == "" {
		t.Fatalf("removed %v, code %q, err %v", removed, setupCode, err)
	}
	if _, ok := w.Lookup(cookie, "1.1.1.1"); ok {
		t.Fatal("login survived")
	}
	if _, err := w.Login(true, storeTestPassword, "", "", "9.9.9.9", storeTestUA); !errors.Is(err, ErrSetupCodeRequired) {
		t.Fatalf("password alone err = %v, want ErrSetupCodeRequired", err)
	}
	if _, err := w.Login(true, storeTestPassword, "", "AAAA-AAAA", "9.9.9.9", storeTestUA); !errors.Is(err, ErrWebAuthRefused) {
		t.Fatalf("wrong code err = %v, want refused", err)
	}
	if _, err := w.Login(true, "wrong password here", "", setupCode, "9.9.9.9", storeTestUA); !errors.Is(err, ErrWebAuthRefused) {
		t.Fatalf("wrong password err = %v, want refused", err)
	}
	_, err = w.Login(true, storeTestPassword, "", setupCode, "9.9.9.9", storeTestUA)
	needed := neededAuthenticator(t, err)

	// The old secret is gone, so the flag and the missing secret survive a restart.
	again := openStoreForTest(t, dir, clock)
	if _, err := again.Login(true, storeTestPassword, "", "", "9.9.9.9", storeTestUA); !errors.Is(err, ErrSetupCodeRequired) {
		t.Fatalf("new module err = %v, want ErrSetupCodeRequired", err)
	}

	code, _ := TOTPCode(needed.Secret, totpStepAt(clock.Now()))
	if _, err := w.AddAuthenticator(needed.PendingID, code, "9.9.9.9", storeTestUA); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Login(true, storeTestPassword, nextCode(clock, needed.Secret), "", "9.9.9.9", storeTestUA); err != nil {
		t.Fatalf("login after adding the authenticator: %v", err)
	}
}

func TestWebAuthStoreAddingAuthenticatorConsumesSetupCode(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	twoLogins(t, w, clock)
	_, setupCode, _ := w.ClearAuthenticator()
	_, err := w.Login(true, storeTestPassword, "", setupCode, "9.9.9.9", storeTestUA)
	needed := neededAuthenticator(t, err)
	code, _ := TOTPCode(needed.Secret, totpStepAt(clock.Now()))
	if _, err := w.AddAuthenticator(needed.PendingID, code, "9.9.9.9", storeTestUA); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Login(true, storeTestPassword, nextCode(clock, needed.Secret), "", "9.9.9.9", storeTestUA); err != nil {
		t.Fatalf("login without the Setup code: %v", err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file.AuthenticatorResetPending || w.setupCode != "" {
		t.Fatalf("flag = %v, code = %q, want both cleared", w.file.AuthenticatorResetPending, w.setupCode)
	}
}

func TestWebAuthStoreClearAuthenticatorFlagWaitsWhileTOTPIsOff(t *testing.T) {
	w, _, _ := newStoreForTest(t)
	cookie := finishStoreSetupTOTPOff(t, w)
	if _, _, err := w.ClearAuthenticator(); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.Lookup(cookie, "1.1.1.1"); ok {
		t.Fatal("login survived")
	}
	if _, err := w.Login(false, storeTestPassword, "", "", "2.2.2.2", storeTestUA); err != nil {
		t.Fatalf("password-only login: %v", err)
	}
	if _, err := w.Login(true, storeTestPassword, "", "", "2.2.2.2", storeTestUA); !errors.Is(err, ErrSetupCodeRequired) {
		t.Fatalf("TOTP on again err = %v, want ErrSetupCodeRequired", err)
	}
}

func TestWebAuthStoreSetupStartWrongCodeIsRefusedAndCounted(t *testing.T) {
	w, _, _ := newStoreForTest(t)
	for i := 0; i < authIPFreeFailures; i++ {
		if _, err := w.SetupStart(true, "AAAA-AAAA", "9.9.9.9"); !errors.Is(err, ErrWebAuthRefused) {
			t.Fatalf("attempt %d: err = %v, want refused", i, err)
		}
	}
	_, err := w.SetupStart(true, "AAAA-AAAA", "9.9.9.9")
	var wait *WebAuthWaitError
	if !errors.As(err, &wait) || wait.Seconds <= 0 {
		t.Fatalf("err = %v, want a wait error", err)
	}
}

func TestWebAuthStoreFinishWithWrongTOTPSavesNothing(t *testing.T) {
	w, clock, dir := newStoreForTest(t)
	_, code, _ := w.SetupState()
	secret, err := w.SetupStart(true, code, "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	good, _ := TOTPCode(secret, totpStepAt(clock.Now()))
	bad := "000000"
	if good == bad {
		bad = "111111"
	}
	if _, err := w.SetupFinish(true, code, storeTestPassword, bad, "1.1.1.1", storeTestUA); !errors.Is(err, ErrWebAuthRefused) {
		t.Fatalf("err = %v, want refused", err)
	}
	again := openStoreForTest(t, dir, clock)
	if pending, _, _ := again.SetupState(); !pending {
		t.Fatal("setup should still be pending")
	}
	if _, err := os.Stat(filepath.Join(dir, webAuthFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state file exists: %v", err)
	}
}

func TestWebAuthStoreFinishRejectsShortPassword(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	_, code, _ := w.SetupState()
	secret, _ := w.SetupStart(true, code, "1.1.1.1")
	totp, _ := TOTPCode(secret, totpStepAt(clock.Now()))
	if _, err := w.SetupFinish(true, code, "short", totp, "1.1.1.1", storeTestUA); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("err = %v, want ErrPasswordTooShort", err)
	}
}

func TestWebAuthStoreFinishGivesCookieThatLooksUp(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	cookie, _ := finishStoreSetup(t, w, clock)
	login, ok := w.Lookup(cookie, "2.2.2.2")
	if !ok {
		t.Fatal("cookie not found")
	}
	if login.Name != "Safari on iPhone" || login.LastIP != "2.2.2.2" {
		t.Fatalf("login = %+v", login)
	}
	if pending, _, _ := w.SetupState(); pending {
		t.Fatal("setup should be done")
	}
}

func TestWebAuthStoreFirstDeviceToFinishWins(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	_, code, _ := w.SetupState()
	secret, _ := w.SetupStart(true, code, "1.1.1.1")
	totp, _ := TOTPCode(secret, totpStepAt(clock.Now()))
	if _, err := w.SetupFinish(true, code, storeTestPassword, totp, "1.1.1.1", storeTestUA); err != nil {
		t.Fatal(err)
	}
	if _, err := w.SetupFinish(true, code, storeTestPassword, totp, "3.3.3.3", storeTestUA); !errors.Is(err, ErrSetupNotPending) {
		t.Fatalf("err = %v, want ErrSetupNotPending", err)
	}
}

func TestWebAuthStoreLookupExpiresAfter90IdleDays(t *testing.T) {
	w, clock, dir := newStoreForTest(t)
	cookie, _ := finishStoreSetup(t, w, clock)

	clock.t = clock.t.Add(89 * 24 * time.Hour)
	if _, ok := w.Lookup(cookie, "1.1.1.1"); !ok {
		t.Fatal("day 89 lookup refused")
	}
	clock.t = clock.t.Add(89 * 24 * time.Hour)
	if _, ok := w.Lookup(cookie, "1.1.1.1"); !ok {
		t.Fatal("lookup should have renewed the login")
	}
	clock.t = clock.t.Add(90 * 24 * time.Hour)
	if _, ok := w.Lookup(cookie, "1.1.1.1"); ok {
		t.Fatal("login idle for 90 days should be refused")
	}
	if got := w.List(); len(got) != 0 {
		t.Fatalf("list = %v, want the login dropped", got)
	}
	if got := openStoreForTest(t, dir, clock).List(); len(got) != 0 {
		t.Fatalf("new module list = %v, want the login dropped", got)
	}
}

func TestWebAuthStoreNewModuleAcceptsOldCookie(t *testing.T) {
	w, clock, dir := newStoreForTest(t)
	cookie, _ := finishStoreSetup(t, w, clock)
	again := openStoreForTest(t, dir, clock)
	if _, ok := again.Lookup(cookie, "1.1.1.1"); !ok {
		t.Fatal("cookie refused after restart")
	}
	if pending, code, _ := again.SetupState(); pending || code != "" {
		t.Fatal("setup should stay done after restart")
	}
}

func TestWebAuthStoreNewModuleMakesNewSetupCode(t *testing.T) {
	w, clock, dir := newStoreForTest(t)
	_, first, _ := w.SetupState()
	_, same, _ := w.SetupState()
	if first != same {
		t.Fatal("code changed without expiry")
	}
	_, second, _ := openStoreForTest(t, dir, clock).SetupState()
	if first == second {
		t.Fatal("restart kept the Setup code")
	}
	clock.t = clock.t.Add(setupCodeLifetime)
	_, third, _ := w.SetupState()
	if first == third {
		t.Fatal("expiry kept the Setup code")
	}
}

func TestWebAuthStoreFileHoldsCookieHashOnly(t *testing.T) {
	w, clock, dir := newStoreForTest(t)
	cookie, secret := finishStoreSetup(t, w, clock)
	path := filepath.Join(dir, webAuthFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), cookie) {
		t.Fatal("file holds the cookie value")
	}
	if !strings.Contains(string(data), secret) || strings.Contains(string(data), storeTestPassword) {
		t.Fatal("file should hold the TOTP secret and no plain password")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestWebAuthStoreLoginMakesSecondWebLogin(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	_, secret := finishStoreSetup(t, w, clock)
	clock.t = clock.t.Add(2 * totpPeriod)
	totp, _ := TOTPCode(secret, totpStepAt(clock.Now()))
	cookie, err := loginTOTPOn(w, storeTestPassword, totp, "4.4.4.4", "Mozilla/5.0 Firefox/120.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := w.Lookup(cookie, "4.4.4.4"); !ok {
		t.Fatal("login cookie not found")
	}
	if got := w.List(); len(got) != 2 {
		t.Fatalf("list = %d logins, want 2", len(got))
	}
}

func TestWebAuthStoreLoginGivesOneRefusalKind(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	_, secret := finishStoreSetup(t, w, clock)
	clock.t = clock.t.Add(2 * totpPeriod)
	good, _ := TOTPCode(secret, totpStepAt(clock.Now()))
	cases := []struct{ name, password, totp string }{
		{"wrong password", "wrong password here", good},
		{"wrong code", storeTestPassword, "000000"},
		{"short password", "short", good},
		{"both wrong", "wrong password here", "000000"},
	}
	for i, c := range cases {
		_, err := loginTOTPOn(w, c.password, c.totp, "5.5.5."+string(rune('0'+i)), storeTestUA)
		if !errors.Is(err, ErrWebAuthRefused) {
			t.Errorf("%s: err = %v, want ErrWebAuthRefused", c.name, err)
		}
	}
}

func TestWebAuthStoreLoginUsesLimiter(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	finishStoreSetup(t, w, clock)
	for i := 0; i < authIPFreeFailures; i++ {
		loginTOTPOn(w, "wrong password here", "000000", "6.6.6.6", storeTestUA)
	}
	_, err := loginTOTPOn(w, storeTestPassword, "000000", "6.6.6.6", storeTestUA)
	var wait *WebAuthWaitError
	if !errors.As(err, &wait) {
		t.Fatalf("err = %v, want a wait error", err)
	}
}

func TestWebAuthStoreLoginRefusesReusedTOTPCode(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	_, totpSecret := finishStoreSetup(t, w, clock)
	// Setup used this step; the same code must not log in again.
	code, _ := TOTPCode(totpSecret, totpStepAt(clock.Now()))
	if _, err := loginTOTPOn(w, storeTestPassword, code, "7.7.7.7", storeTestUA); !errors.Is(err, ErrWebAuthRefused) {
		t.Fatalf("err = %v, want refused", err)
	}
}

// nextCode moves the clock one TOTP step and returns a fresh code for secret.
func nextCode(clock *storeClock, secret string) string {
	clock.t = clock.t.Add(totpPeriod)
	code, _ := TOTPCode(secret, totpStepAt(clock.Now()))
	return code
}

// twoLogins finishes setup and adds a second web login. It returns both ids.
func twoLogins(t *testing.T, w *WebAuth, clock *storeClock) (firstID, secondID, firstCookie, secret string) {
	t.Helper()
	firstCookie, secret = finishStoreSetup(t, w, clock)
	if _, err := loginTOTPOn(w, storeTestPassword, nextCode(clock, secret), "4.4.4.4", "Mozilla/5.0 Firefox/120.0"); err != nil {
		t.Fatal(err)
	}
	logins := w.List()
	return logins[0].ID, logins[1].ID, firstCookie, secret
}

func TestWebAuthStoreRenameIsStored(t *testing.T) {
	w, clock, dir := newStoreForTest(t)
	first, _, _, _ := twoLogins(t, w, clock)
	if err := w.RenameLogin(first, "Kitchen tablet"); err != nil {
		t.Fatal(err)
	}
	if got := openStoreForTest(t, dir, clock).List()[0].Name; got != "Kitchen tablet" {
		t.Fatalf("name = %q", got)
	}
	if err := w.RenameLogin(first, strings.Repeat("x", deviceNameMaxLen+1)); !errors.Is(err, ErrWebLoginNameInvalid) {
		t.Fatalf("long name err = %v", err)
	}
	if err := w.RenameLogin("nope", "x"); !errors.Is(err, ErrWebLoginNotFound) {
		t.Fatalf("unknown id err = %v", err)
	}
}

func TestWebAuthStoreRevokeRemovesOneAndReturnsItsID(t *testing.T) {
	w, clock, dir := newStoreForTest(t)
	first, second, _, _ := twoLogins(t, w, clock)
	got, err := w.RevokeLogin(second)
	if err != nil || len(got) != 1 || got[0] != second {
		t.Fatalf("got %v, %v", got, err)
	}
	if list := openStoreForTest(t, dir, clock).List(); len(list) != 1 || list[0].ID != first {
		t.Fatalf("list = %+v", list)
	}
	if _, err := w.RevokeLogin(second); !errors.Is(err, ErrWebLoginNotFound) {
		t.Fatalf("unknown id err = %v", err)
	}
}

func TestWebAuthStoreChangePasswordRefusalsAreCounted(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	_, _, _, secret := twoLogins(t, w, clock)
	good := nextCode(clock, secret)
	for i := 0; i < authIPFreeFailures; i++ {
		if _, err := w.ChangePassword(true, "wrong password here", good, "new password 123", "", "8.8.8.8"); !errors.Is(err, ErrWebAuthRefused) {
			t.Fatalf("attempt %d: err = %v", i, err)
		}
	}
	// The wrong checks also locked the IP out of login.
	_, err := loginTOTPOn(w, storeTestPassword, good, "8.8.8.8", storeTestUA)
	var wait *WebAuthWaitError
	if !errors.As(err, &wait) {
		t.Fatalf("err = %v, want a wait error", err)
	}
}

func TestWebAuthStoreChangePasswordRejectsShortPassword(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	_, _, _, secret := twoLogins(t, w, clock)
	code := nextCode(clock, secret)
	if _, err := w.ChangePassword(true, storeTestPassword, code, "short", "", "8.8.8.8"); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("err = %v", err)
	}
	if _, err := w.SetPassword("short"); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("set err = %v", err)
	}
}

func TestWebAuthStoreChangePasswordKeepsOnlyCurrentLogin(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	first, second, cookie, secret := twoLogins(t, w, clock)
	removed, err := w.ChangePassword(true, storeTestPassword, nextCode(clock, secret), "brand new password", first, "8.8.8.8")
	if err != nil || len(removed) != 1 || removed[0] != second {
		t.Fatalf("removed %v, err %v", removed, err)
	}
	if _, ok := w.Lookup(cookie, "1.1.1.1"); !ok {
		t.Fatal("current login was signed out")
	}
	if _, err := loginTOTPOn(w, "brand new password", nextCode(clock, secret), "9.9.9.9", storeTestUA); err != nil {
		t.Fatalf("new password refused: %v", err)
	}
}

func TestWebAuthStoreSetPasswordSignsOutEveryLogin(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	first, second, cookie, secret := twoLogins(t, w, clock)
	removed, err := w.SetPassword("brand new password")
	if err != nil || len(removed) != 2 || removed[0] != first || removed[1] != second {
		t.Fatalf("removed %v, err %v", removed, err)
	}
	if _, ok := w.Lookup(cookie, "1.1.1.1"); ok {
		t.Fatal("login survived")
	}
	if _, err := loginTOTPOn(w, "brand new password", nextCode(clock, secret), "9.9.9.9", storeTestUA); err != nil {
		t.Fatalf("new password refused: %v", err)
	}
}

func TestWebAuthStoreAbandonedAuthenticatorResetKeepsOldSecret(t *testing.T) {
	w, clock, dir := newStoreForTest(t)
	_, _, _, secret := twoLogins(t, w, clock)
	if _, err := w.AuthenticatorResetBegin(true, storeTestPassword, nextCode(clock, secret), "8.8.8.8"); err != nil {
		t.Fatal(err)
	}
	again := openStoreForTest(t, dir, clock)
	if _, err := loginTOTPOn(again, storeTestPassword, nextCode(clock, secret), "9.9.9.9", storeTestUA); err != nil {
		t.Fatalf("old secret refused: %v", err)
	}
}

func TestWebAuthStoreFinishedAuthenticatorResetReplacesSecret(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	first, second, _, secret := twoLogins(t, w, clock)
	newSecret, err := w.AuthenticatorResetBegin(true, storeTestPassword, nextCode(clock, secret), "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	removed, err := w.AuthenticatorResetFinish(nextCode(clock, newSecret), first, "8.8.8.8")
	if err != nil || len(removed) != 1 || removed[0] != second {
		t.Fatalf("removed %v, err %v", removed, err)
	}
	if _, err := loginTOTPOn(w, storeTestPassword, nextCode(clock, secret), "9.9.9.9", storeTestUA); !errors.Is(err, ErrWebAuthRefused) {
		t.Fatalf("old secret err = %v, want refused", err)
	}
	// The first code of the new secret is not a replay.
	code, _ := TOTPCode(newSecret, totpStepAt(clock.Now()))
	if _, err := loginTOTPOn(w, storeTestPassword, code, "9.9.9.9", storeTestUA); err != nil {
		t.Fatalf("new secret refused: %v", err)
	}
}

func TestWebAuthStoreAuthenticatorResetBeginNeedsGoodChecks(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	_, _, _, secret := twoLogins(t, w, clock)
	if _, err := w.AuthenticatorResetBegin(true, "wrong password here", nextCode(clock, secret), "8.8.8.8"); !errors.Is(err, ErrWebAuthRefused) {
		t.Fatalf("err = %v", err)
	}
	if _, err := w.AuthenticatorResetFinish("123456", "", "8.8.8.8"); !errors.Is(err, ErrWebAuthRefused) {
		t.Fatalf("finish without begin err = %v", err)
	}
}

func TestWebAuthStoreWebResetReturnsToSetupPending(t *testing.T) {
	w, clock, dir := newStoreForTest(t)
	first, second, cookie, _ := twoLogins(t, w, clock)
	_, oldCode, _ := w.SetupState()
	removed, err := w.WebReset()
	if err != nil || len(removed) != 2 || removed[0] != first || removed[1] != second {
		t.Fatalf("removed %v, err %v", removed, err)
	}
	pending, code, _ := w.SetupState()
	if !pending || code == "" {
		t.Fatalf("pending = %v, code = %q", pending, code)
	}
	if code == oldCode {
		t.Fatal("Setup code did not change")
	}
	if _, ok := w.Lookup(cookie, "1.1.1.1"); ok || len(w.List()) != 0 {
		t.Fatal("web logins survived")
	}
	if pending, _, _ := openStoreForTest(t, dir, clock).SetupState(); !pending {
		t.Fatal("reset not saved")
	}
}

func TestWebAuthStoreWebResetClearsRateLimits(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	finishStoreSetup(t, w, clock)
	for i := 0; i < authIPFreeFailures; i++ {
		loginTOTPOn(w, "wrong password here", "000000", "6.6.6.6", storeTestUA)
	}
	if _, err := w.WebReset(); err != nil {
		t.Fatal(err)
	}
	_, code, _ := w.SetupState()
	if _, err := w.SetupStart(true, code, "6.6.6.6"); err != nil {
		t.Fatalf("limit survived reset: %v", err)
	}
}

func TestWebAuthStoreSeededWebLoginShowsInList(t *testing.T) {
	dir := t.TempDir()
	clock := &storeClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	lastUsed := clock.t.Add(-48 * time.Hour)
	cookie, err := SeedWebLogin(dir, storeTestUA, lastUsed)
	if err != nil {
		t.Fatal(err)
	}
	w := openStoreForTest(t, dir, clock)
	got := w.List()
	if len(got) != 1 || !got[0].LastUsed.Equal(lastUsed) || got[0].Name != "Safari on iPhone" {
		t.Fatalf("list = %+v", got)
	}
	if _, ok := w.Lookup(cookie, "1.1.1.1"); !ok {
		t.Fatal("seeded cookie not found")
	}
}
