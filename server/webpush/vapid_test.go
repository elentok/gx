package webpush

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func vapidPath(dir string) string { return filepath.Join(dir, "web-push", "vapid.json") }

type loadResult struct {
	keys *Keys
	hook []string
	logs string
}

func load(t *testing.T, dir string) loadResult {
	t.Helper()
	var buf bytes.Buffer
	var r loadResult
	log := slog.New(slog.NewTextHandler(&buf, nil))
	k, err := LoadOrCreate(dir, log, func(p string) {
		// vapid.json must already hold the new pair when the hook runs.
		var f struct{ Public string }
		raw, err := os.ReadFile(vapidPath(dir))
		if err != nil {
			t.Errorf("vapid.json not written before hook: %v", err)
		} else if err := json.Unmarshal(raw, &f); err != nil || f.Public != p {
			t.Errorf("vapid.json public = %q, hook got %q (err %v)", f.Public, p, err)
		}
		r.hook = append(r.hook, p)
	})
	if err != nil {
		t.Fatal(err)
	}
	r.keys, r.logs = k, buf.String()
	return r
}

func TestVAPID_FirstUseCreatesFileAndReloads(t *testing.T) {
	dir := t.TempDir()
	first := load(t, dir)
	info, err := os.Stat(vapidPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
	dirInfo, _ := os.Stat(filepath.Dir(vapidPath(dir)))
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v, want 0700", dirInfo.Mode().Perm())
	}
	if len(first.hook) != 1 || first.hook[0] != first.keys.Public() {
		t.Fatalf("first use hook = %v", first.hook)
	}
	if first.logs != "" {
		t.Fatalf("first use logged %q", first.logs)
	}
	pub, err := base64.RawURLEncoding.DecodeString(first.keys.Public())
	if err != nil || len(pub) != 65 || pub[0] != 4 {
		t.Fatalf("public key not a 65-byte uncompressed point: %v len %d", err, len(pub))
	}
	again := load(t, dir)
	if again.keys.Public() != first.keys.Public() || len(again.hook) != 0 {
		t.Fatalf("reload changed the pair or fired the hook: %v", again.hook)
	}
}

func TestVAPID_BadFileMakesNewPair(t *testing.T) {
	stored := func() (priv, pub string) {
		d := t.TempDir()
		load(t, d)
		raw, _ := os.ReadFile(vapidPath(d))
		var f map[string]string
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatal(err)
		}
		return f["private"], f["public"]
	}
	mismatch := func(dir string) {
		priv, _ := stored()
		_, pub := stored()
		raw, _ := json.Marshal(map[string]string{"private": priv, "public": pub})
		write(t, dir, string(raw))
	}
	cases := map[string]func(dir string){
		"empty":     func(dir string) { write(t, dir, "") },
		"malformed": func(dir string) { write(t, dir, "{nope") },
		"mismatch":  mismatch,
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			setup(dir)
			old, _ := os.ReadFile(vapidPath(dir))
			r := load(t, dir)
			if len(r.hook) != 1 || r.hook[0] != r.keys.Public() {
				t.Fatalf("hook = %v", r.hook)
			}
			if n := strings.Count(strings.TrimSpace(r.logs), "\n") + 1; r.logs == "" || n != 1 {
				t.Fatalf("want one log line, got %q", r.logs)
			}
			now, _ := os.ReadFile(vapidPath(dir))
			if bytes.Equal(old, now) {
				t.Fatal("file not replaced")
			}
			if r.keys.Public() == "" || strings.Contains(r.logs, r.keys.Public()) {
				t.Fatalf("log leaks key material: %q", r.logs)
			}
		})
	}
}

func write(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(vapidPath(dir)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vapidPath(dir), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestVAPID_SignHeaderVerifies(t *testing.T) {
	k := load(t, t.TempDir()).keys
	now := time.Unix(1_700_000_000, 0)
	header, err := k.Sign("https://push.example.com", "me@example.com", now)
	if err != nil {
		t.Fatal(err)
	}
	rest, ok := strings.CutPrefix(header, "vapid t=")
	if !ok {
		t.Fatalf("header = %q", header)
	}
	jwt, key, ok := strings.Cut(rest, ", k=")
	if !ok || key != k.Public() {
		t.Fatalf("header = %q", header)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt = %q", jwt)
	}
	pubRaw, _ := base64.RawURLEncoding.DecodeString(k.Public())
	pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(pubRaw[1:33]), Y: new(big.Int).SetBytes(pubRaw[33:])}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if len(sig) != 64 || !ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("signature does not verify")
	}
	var claims struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Aud != "https://push.example.com" || claims.Sub != "mailto:me@example.com" {
		t.Fatalf("claims = %+v", claims)
	}
	if d := time.Unix(claims.Exp, 0).Sub(now); d <= 0 || d > 12*time.Hour {
		t.Fatalf("exp is %v after now", d)
	}
}

func TestValidContact(t *testing.T) {
	for s, want := range map[string]bool{
		"me@example.com":               true,
		"":                             false,
		"Me <me@example.com>":          false,
		"a@example.com, b@example.com": false,
		" me@example.com":              false,
		"me@example.com ":              false,
		"mailto:me@example.com":        false,
		"nope":                         false,
	} {
		if got := ValidContact(s); got != want {
			t.Errorf("ValidContact(%q) = %v, want %v", s, got, want)
		}
	}
}
