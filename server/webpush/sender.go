package webpush

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	reasonUnreachable = "gx can't reach the push service. Check the server's outbound HTTPS"
	reasonNoContact   = "gx has no valid contact email for push. Set one in the config"
	maxRetryAfter     = 30 * time.Second
	bodyChars         = 200
	bodyCharsSmall    = 100
)

var retryWaits = []time.Duration{time.Second, 5 * time.Second}

// Subscription is a browser's push subscription, base64url as the browser gives it.
type Subscription struct{ Endpoint, P256dh, Auth string }

// Message is what the service worker shows. Path is a route path, nothing more.
type Message struct {
	Title, Body, Path, Tag string
	Badge                  int
}

// Options are the per-send choices the caller makes.
type Options struct {
	Urgency string // "high" or "normal"
	TTL     time.Duration
	Test    bool
}

// Outcome reports one Send. Reason is set on a failure and nowhere else.
type Outcome struct {
	Delivered bool
	Gone      bool // 404 or 410: the caller deletes the subscription
	Dropped   bool // the quiet check stopped it; not a failure
	Reason    string
}

// Sender sends one push at a time and follows gx's delivery rules.
type Sender struct {
	Client  *http.Client
	Keys    *Keys
	Contact func() string
	Quiet   func() bool
	Sleep   func(ctx context.Context, d time.Duration) error
	Rand    io.Reader // nil means crypto/rand; tests fix the key and salt through it
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func failed(reason string) Outcome { return Outcome{Reason: reason} }

func statusReason(code int) string { return fmt.Sprintf("The push service answered %d.", code) }

// Send delivers msg to sub. It never returns an error: every failure is in the Outcome.
func (s *Sender) Send(ctx context.Context, sub Subscription, msg Message, opts Options) Outcome {
	contact := s.Contact()
	if !ValidContact(contact) {
		return failed(reasonNoContact)
	}
	endpoint, err := url.Parse(sub.Endpoint)
	if err != nil || endpoint.Host == "" {
		return failed("The push subscription has no usable endpoint.")
	}
	auth, err := s.Keys.Sign(endpoint.Scheme+"://"+endpoint.Host, contact, time.Now())
	if err != nil {
		return failed("gx could not sign the push.")
	}
	body, err := s.encryptMessage(sub, msg)
	if err != nil {
		return failed("gx could not encrypt the push.")
	}
	ttl := opts.TTL
	if ttl == 0 {
		ttl = time.Hour
		if opts.Test {
			ttl = time.Minute
		}
	}
	topic := sha256.Sum256([]byte(msg.Tag))
	hdr := http.Header{
		"Content-Encoding": {"aes128gcm"},
		"Content-Type":     {"application/octet-stream"},
		"Authorization":    {auth},
		"Ttl":              {strconv.Itoa(int(ttl.Seconds()))},
		"Urgency":          {opts.Urgency},
		"Topic":            {b64.EncodeToString(topic[:])[:32]},
	}

	tries := len(retryWaits) + 1
	if opts.Test {
		tries = 1
	}
	var reason string
	for i := 0; i < tries; i++ {
		if !opts.Test && s.Quiet != nil && s.Quiet() {
			return Outcome{Dropped: true}
		}
		code, retryAfter, err := s.post(ctx, sub.Endpoint, hdr, body)
		switch {
		case err != nil:
			reason = reasonUnreachable
		case code == http.StatusCreated:
			return Outcome{Delivered: true}
		case code == http.StatusNotFound || code == http.StatusGone:
			return Outcome{Gone: true}
		case code >= 500 || code == http.StatusTooManyRequests:
			reason = statusReason(code)
		default:
			return failed(statusReason(code))
		}
		if ctx.Err() != nil {
			return failed(reason)
		}
		if i < tries-1 {
			wait := retryWaits[i]
			if retryAfter > 0 {
				wait = retryAfter
			}
			sleep := s.Sleep
			if sleep == nil {
				sleep = sleepCtx
			}
			if err := sleep(ctx, wait); err != nil {
				return failed(reason)
			}
		}
	}
	return failed(reason)
}

// post returns the status and a usable Retry-After (0 when absent, over the cap or unparsable).
// An error from the client is returned bare: it holds the endpoint URL and never reaches a Reason.
func (s *Sender) post(ctx context.Context, endpoint string, hdr http.Header, body []byte) (int, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, 0, err
	}
	req.Header = hdr.Clone()
	resp, err := s.Client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	var after time.Duration
	if n, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && n > 0 && time.Duration(n)*time.Second <= maxRetryAfter {
		after = time.Duration(n) * time.Second
	}
	return resp.StatusCode, after, nil
}

func (s *Sender) encryptMessage(sub Subscription, msg Message) ([]byte, error) {
	uaPublic, err := b64.DecodeString(sub.P256dh)
	if err != nil {
		return nil, err
	}
	auth, err := b64.DecodeString(sub.Auth)
	if err != nil {
		return nil, err
	}
	plain, err := payload(msg, bodyChars)
	if err != nil {
		return nil, err
	}
	if len(plain) > maxPlaintext {
		if plain, err = payload(msg, bodyCharsSmall); err != nil {
			return nil, err
		}
	}
	rnd := s.Rand
	if rnd == nil {
		rnd = rand.Reader
	}
	asPriv, err := ecdh.P256().GenerateKey(rnd)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rnd, salt); err != nil {
		return nil, err
	}
	out, err := encrypt(plain, uaPublic, auth, asPriv, salt)
	if err != nil {
		return nil, errors.New("encrypt: " + err.Error())
	}
	return out, nil
}

func payload(msg Message, bodyLimit int) ([]byte, error) {
	if r := []rune(msg.Body); len(r) > bodyLimit {
		msg.Body = string(r[:bodyLimit])
	}
	return json.Marshal(struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		Path  string `json:"path"`
		Badge int    `json:"badge"`
		Tag   string `json:"tag"`
	}{msg.Title, msg.Body, msg.Path, msg.Badge, msg.Tag})
}
