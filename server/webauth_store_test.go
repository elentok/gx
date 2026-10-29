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
