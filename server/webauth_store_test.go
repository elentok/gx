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
	secret, err = w.SetupStart(code, "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	totp, _ := TOTPCode(secret, totpStepAt(clock.Now()))
	cookie, err = w.SetupFinish(code, storeTestPassword, totp, "1.1.1.1", storeTestUA)
	if err != nil {
		t.Fatal(err)
	}
	return cookie, secret
}

func TestWebAuthStoreSetupStartWrongCodeIsRefusedAndCounted(t *testing.T) {
	w, _, _ := newStoreForTest(t)
	for i := 0; i < authIPFreeFailures; i++ {
		if _, err := w.SetupStart("AAAA-AAAA", "9.9.9.9"); !errors.Is(err, ErrWebAuthRefused) {
			t.Fatalf("attempt %d: err = %v, want refused", i, err)
		}
	}
	_, err := w.SetupStart("AAAA-AAAA", "9.9.9.9")
	var wait *WebAuthWaitError
	if !errors.As(err, &wait) || wait.Seconds <= 0 {
		t.Fatalf("err = %v, want a wait error", err)
	}
}

func TestWebAuthStoreFinishWithWrongTOTPSavesNothing(t *testing.T) {
	w, clock, dir := newStoreForTest(t)
	_, code, _ := w.SetupState()
	secret, err := w.SetupStart(code, "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	good, _ := TOTPCode(secret, totpStepAt(clock.Now()))
	bad := "000000"
	if good == bad {
		bad = "111111"
	}
	if _, err := w.SetupFinish(code, storeTestPassword, bad, "1.1.1.1", storeTestUA); !errors.Is(err, ErrWebAuthRefused) {
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
	secret, _ := w.SetupStart(code, "1.1.1.1")
	totp, _ := TOTPCode(secret, totpStepAt(clock.Now()))
	if _, err := w.SetupFinish(code, "short", totp, "1.1.1.1", storeTestUA); !errors.Is(err, ErrPasswordTooShort) {
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
	secret, _ := w.SetupStart(code, "1.1.1.1")
	totp, _ := TOTPCode(secret, totpStepAt(clock.Now()))
	if _, err := w.SetupFinish(code, storeTestPassword, totp, "1.1.1.1", storeTestUA); err != nil {
		t.Fatal(err)
	}
	if _, err := w.SetupFinish(code, storeTestPassword, totp, "3.3.3.3", storeTestUA); !errors.Is(err, ErrSetupNotPending) {
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
	cookie, err := w.Login(storeTestPassword, totp, "4.4.4.4", "Mozilla/5.0 Firefox/120.0")
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
		_, err := w.Login(c.password, c.totp, "5.5.5."+string(rune('0'+i)), storeTestUA)
		if !errors.Is(err, ErrWebAuthRefused) {
			t.Errorf("%s: err = %v, want ErrWebAuthRefused", c.name, err)
		}
	}
}

func TestWebAuthStoreLoginUsesLimiter(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	finishStoreSetup(t, w, clock)
	for i := 0; i < authIPFreeFailures; i++ {
		w.Login("wrong password here", "000000", "6.6.6.6", storeTestUA)
	}
	_, err := w.Login(storeTestPassword, "000000", "6.6.6.6", storeTestUA)
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
	if _, err := w.Login(storeTestPassword, code, "7.7.7.7", storeTestUA); !errors.Is(err, ErrWebAuthRefused) {
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
	if _, err := w.Login(storeTestPassword, nextCode(clock, secret), "4.4.4.4", "Mozilla/5.0 Firefox/120.0"); err != nil {
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
		if _, err := w.ChangePassword("wrong password here", good, "new password 123", "", "8.8.8.8"); !errors.Is(err, ErrWebAuthRefused) {
			t.Fatalf("attempt %d: err = %v", i, err)
		}
	}
	// The wrong checks also locked the IP out of login.
	_, err := w.Login(storeTestPassword, good, "8.8.8.8", storeTestUA)
	var wait *WebAuthWaitError
	if !errors.As(err, &wait) {
		t.Fatalf("err = %v, want a wait error", err)
	}
}

func TestWebAuthStoreChangePasswordRejectsShortPassword(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	_, _, _, secret := twoLogins(t, w, clock)
	code := nextCode(clock, secret)
	if _, err := w.ChangePassword(storeTestPassword, code, "short", "", "8.8.8.8"); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("err = %v", err)
	}
	if _, err := w.SetPassword("short"); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("set err = %v", err)
	}
}

func TestWebAuthStoreChangePasswordKeepsOnlyCurrentLogin(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	first, second, cookie, secret := twoLogins(t, w, clock)
	removed, err := w.ChangePassword(storeTestPassword, nextCode(clock, secret), "brand new password", first, "8.8.8.8")
	if err != nil || len(removed) != 1 || removed[0] != second {
		t.Fatalf("removed %v, err %v", removed, err)
	}
	if _, ok := w.Lookup(cookie, "1.1.1.1"); !ok {
		t.Fatal("current login was signed out")
	}
	if _, err := w.Login("brand new password", nextCode(clock, secret), "9.9.9.9", storeTestUA); err != nil {
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
	if _, err := w.Login("brand new password", nextCode(clock, secret), "9.9.9.9", storeTestUA); err != nil {
		t.Fatalf("new password refused: %v", err)
	}
}

func TestWebAuthStoreAbandonedAuthenticatorResetKeepsOldSecret(t *testing.T) {
	w, clock, dir := newStoreForTest(t)
	_, _, _, secret := twoLogins(t, w, clock)
	if _, err := w.AuthenticatorResetBegin(storeTestPassword, nextCode(clock, secret), "8.8.8.8"); err != nil {
		t.Fatal(err)
	}
	again := openStoreForTest(t, dir, clock)
	if _, err := again.Login(storeTestPassword, nextCode(clock, secret), "9.9.9.9", storeTestUA); err != nil {
		t.Fatalf("old secret refused: %v", err)
	}
}

func TestWebAuthStoreFinishedAuthenticatorResetReplacesSecret(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	first, second, _, secret := twoLogins(t, w, clock)
	newSecret, err := w.AuthenticatorResetBegin(storeTestPassword, nextCode(clock, secret), "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	removed, err := w.AuthenticatorResetFinish(nextCode(clock, newSecret), first, "8.8.8.8")
	if err != nil || len(removed) != 1 || removed[0] != second {
		t.Fatalf("removed %v, err %v", removed, err)
	}
	if _, err := w.Login(storeTestPassword, nextCode(clock, secret), "9.9.9.9", storeTestUA); !errors.Is(err, ErrWebAuthRefused) {
		t.Fatalf("old secret err = %v, want refused", err)
	}
	// The first code of the new secret is not a replay.
	code, _ := TOTPCode(newSecret, totpStepAt(clock.Now()))
	if _, err := w.Login(storeTestPassword, code, "9.9.9.9", storeTestUA); err != nil {
		t.Fatalf("new secret refused: %v", err)
	}
}

func TestWebAuthStoreAuthenticatorResetBeginNeedsGoodChecks(t *testing.T) {
	w, clock, _ := newStoreForTest(t)
	_, _, _, secret := twoLogins(t, w, clock)
	if _, err := w.AuthenticatorResetBegin("wrong password here", nextCode(clock, secret), "8.8.8.8"); !errors.Is(err, ErrWebAuthRefused) {
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
		w.Login("wrong password here", "000000", "6.6.6.6", storeTestUA)
	}
	if _, err := w.WebReset(); err != nil {
		t.Fatal(err)
	}
	_, code, _ := w.SetupState()
	if _, err := w.SetupStart(code, "6.6.6.6"); err != nil {
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
