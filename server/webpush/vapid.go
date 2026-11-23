// Package webpush holds the server's Web Push identity: the VAPID key pair and
// the contact claim every push send carries.
package webpush

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/config/registry"
	"github.com/elentok/gx/fsutil"
)

const (
	dirName     = "web-push"
	fileName    = "vapid.json"
	maxJWTLife  = 12 * time.Hour
	jwtHeaderB4 = `{"typ":"JWT","alg":"ES256"}`
)

var b64 = base64.RawURLEncoding

// ValidContact reports whether s is exactly one plain email address: no display
// name, list, whitespace or mailto: prefix.
func ValidContact(s string) bool { return registry.ValidContactEmail(s) }

type vapidFile struct {
	Private string `json:"private"`
	Public  string `json:"public"`
}

// Keys is the VAPID key pair. It is immutable after load.
type Keys struct {
	priv   *ecdsa.PrivateKey
	public string
}

// Public returns the VAPID public key string: the 65-byte uncompressed point
// in unpadded base64url, the browser's applicationServerKey.
func (k *Keys) Public() string { return k.public }

// LoadOrCreate loads <stateDir>/web-push/vapid.json, or makes a new pair when
// the file is missing or unusable. A lost or bad file logs one line.
func LoadOrCreate(stateDir string, log *slog.Logger) (*Keys, error) {
	path := filepath.Join(stateDir, dirName, fileName)
	raw, readErr := os.ReadFile(path)
	if readErr == nil {
		if k, err := parse(raw); err == nil {
			return k, nil
		}
		log.Warn("web push key file unusable, made a new key pair", "file", path)
	} else if !errors.Is(readErr, os.ErrNotExist) {
		log.Warn("web push key file unreadable, made a new key pair", "file", path)
	}
	return create(path)
}

func create(path string) (*Keys, error) {
	ek, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(vapidFile{Private: b64.EncodeToString(ek.Bytes()), Public: b64.EncodeToString(ek.PublicKey().Bytes())})
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := fsutil.WriteFilePrivateAtomic(path, out); err != nil {
		return nil, err
	}
	return parse(out)
}

func parse(raw []byte) (*Keys, error) {
	var f vapidFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	d, err := b64.DecodeString(f.Private)
	if err != nil {
		return nil, err
	}
	priv, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), d)
	if err != nil {
		return nil, err
	}
	pub, err := priv.PublicKey.Bytes()
	if err != nil {
		return nil, err
	}
	if b64.EncodeToString(pub) != f.Public {
		return nil, errors.New("public key does not match the private key")
	}
	return &Keys{priv: priv, public: f.Public}, nil
}

// Sign builds the Authorization header for a push to the service at audience
// (its origin). contact is the plain email for the one mailto: sub claim.
func (k *Keys) Sign(audience, contact string, now time.Time) (string, error) {
	if !ValidContact(contact) {
		return "", errors.New("no valid contact email")
	}
	claims, err := json.Marshal(map[string]any{"aud": audience, "exp": now.Add(maxJWTLife).Unix(), "sub": "mailto:" + contact})
	if err != nil {
		return "", err
	}
	signing := b64.EncodeToString([]byte(jwtHeaderB4)) + "." + b64.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, k.priv, digest[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return fmt.Sprintf("vapid t=%s.%s, k=%s", signing, b64.EncodeToString(sig), k.public), nil
}
