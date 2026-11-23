package webpush

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

// RFC 8291 Appendix A example values.
const (
	rfcAuth       = "BTBZMqHH6r4Tts7J_aSIgg"
	rfcUAPrivate  = "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"
	rfcUAPublic   = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	rfcASPrivate  = "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"
	rfcSalt       = "DGv6ra1nlYgDCS1FRnbzlw"
	rfcPlaintext  = "When I grow up, I want to be a watermelon"
	rfcCiphertext = "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27ml" +
		"mlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPT" +
		"pK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
)

func mustB64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := b64.DecodeString(s)
	if err != nil {
		t.Fatalf("decode %q: %v", s, err)
	}
	return b
}

func mustKey(t *testing.T, s string) *ecdh.PrivateKey {
	t.Helper()
	k, err := ecdh.P256().NewPrivateKey(mustB64(t, s))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestEncrypt_MatchesRFC8291AppendixA(t *testing.T) {
	got, err := encrypt([]byte(rfcPlaintext), mustB64(t, rfcUAPublic), mustB64(t, rfcAuth), mustKey(t, rfcASPrivate), mustB64(t, rfcSalt))
	if err != nil {
		t.Fatal(err)
	}
	if want := mustB64(t, rfcCiphertext); !bytes.Equal(got, want) {
		t.Fatalf("body differs from RFC 8291 Appendix A\n got %s\nwant %s", b64.EncodeToString(got), rfcCiphertext)
	}
}

// decrypt is the receiver side of RFC 8291, written from the RFC separately from encrypt.
func decrypt(t *testing.T, body []byte, uaPriv *ecdh.PrivateKey, auth []byte) []byte {
	t.Helper()
	salt := body[:16]
	if rs := binary.BigEndian.Uint32(body[16:20]); rs != 4096 {
		t.Fatalf("record size %d", rs)
	}
	idLen := int(body[20])
	asPub := body[21 : 21+idLen]
	asKey, err := ecdh.P256().NewPublicKey(asPub)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := uaPriv.ECDH(asKey)
	if err != nil {
		t.Fatal(err)
	}
	keyInfo := append(append([]byte("WebPush: info\x00"), uaPriv.PublicKey().Bytes()...), asPub...)
	ikm, err := hkdf.Key(sha256.New, secret, auth, string(keyInfo), 32)
	if err != nil {
		t.Fatal(err)
	}
	cek, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, body[21+idLen:], nil)
	if err != nil {
		t.Fatal(err)
	}
	if plain[len(plain)-1] != 2 {
		t.Fatalf("last record delimiter %d", plain[len(plain)-1])
	}
	return plain[:len(plain)-1]
}

func TestEncrypt_RoundTripWithSubscriptionKeys(t *testing.T) {
	ua, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	as, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := bytes.Repeat([]byte{7}, 16)
	body, err := encrypt([]byte("héllo"), ua.PublicKey().Bytes(), auth, as, bytes.Repeat([]byte{9}, 16))
	if err != nil {
		t.Fatal(err)
	}
	if got := decrypt(t, body, ua, auth); string(got) != "héllo" {
		t.Fatalf("round trip gave %q", got)
	}
}
