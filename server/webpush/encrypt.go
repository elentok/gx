package webpush

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

const (
	recordSize = 4096
	// maxPlaintext is the most payload one record carries once the 86-byte
	// header, the delimiter byte and the 16-byte GCM tag are paid for.
	maxPlaintext = recordSize - 86 - 1 - 16
)

// encrypt builds an RFC 8291 aes128gcm body for one record. asPriv and salt are
// injected so the RFC's Appendix A example can be reproduced.
func encrypt(plaintext, uaPublic, auth []byte, asPriv *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	if len(plaintext) > maxPlaintext {
		return nil, errors.New("payload too large")
	}
	uaKey, err := ecdh.P256().NewPublicKey(uaPublic)
	if err != nil {
		return nil, err
	}
	secret, err := asPriv.ECDH(uaKey)
	if err != nil {
		return nil, err
	}
	asPublic := asPriv.PublicKey().Bytes()
	keyInfo := "WebPush: info\x00" + string(uaPublic) + string(asPublic)
	ikm, err := hkdf.Key(sha256.New, secret, auth, keyInfo, 32)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, 86+len(plaintext)+1+gcm.Overhead())
	out = append(out, salt...)
	out = binary.BigEndian.AppendUint32(out, recordSize)
	out = append(out, byte(len(asPublic)))
	out = append(out, asPublic...)
	record := append(append([]byte{}, plaintext...), 2) // 2 marks the last record
	return gcm.Seal(out, nonce, record, nil), nil
}
