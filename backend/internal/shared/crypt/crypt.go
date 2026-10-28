// Package crypt holds the few crypto helpers the app needs: AES-256-GCM for
// values we must read back, HMAC for values we only compare, constant-time
// comparison and random digit codes. Nothing here invents a scheme.
package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	keyLen    = 32 // AES-256
	nonceLen  = 12 // GCM standard nonce
	headerLen = 1 + nonceLen
)

var (
	ErrKey     = errors.New("crypt: key must be 32 bytes")
	ErrInvalid = errors.New("crypt: cannot open value") // wrong key, wrong context or tampered
)

// Ring holds the encryption keys by version byte. New values use the current
// version; old versions stay so values written before a rotation can still be read.
type Ring struct {
	current byte
	keys    map[byte]cipher.AEAD
}

// NewRing needs the current version to be present in keys.
func NewRing(current byte, keys map[byte][]byte) (*Ring, error) {
	r := &Ring{current: current, keys: map[byte]cipher.AEAD{}}
	for version, key := range keys {
		if len(key) != keyLen {
			return nil, fmt.Errorf("version %d: %w", version, ErrKey)
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		if r.keys[version], err = cipher.NewGCM(block); err != nil {
			return nil, err
		}
	}
	if _, ok := r.keys[current]; !ok {
		return nil, fmt.Errorf("crypt: current key version %d is not in the ring", current)
	}
	return r, nil
}

// ParseKey reads a base64 key as stored in SSM (`openssl rand -base64 32`).
func ParseKey(b64 string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(key) != keyLen {
		return nil, ErrKey // never include the value in the error
	}
	return key, nil
}

// Seal encrypts plaintext. aad binds the value to its owner (for example the
// order id), so a stored value cannot be copied to another row and still open.
// Output: version byte, 12-byte random nonce, ciphertext with tag.
func (r *Ring) Seal(plaintext, aad []byte) []byte {
	aead := r.keys[r.current]
	out := make([]byte, headerLen, headerLen+len(plaintext)+aead.Overhead())
	out[0] = r.current
	_, _ = rand.Read(out[1:headerLen]) // never fails since Go 1.24
	// #nosec G407 -- the nonce is random: it was just filled from crypto/rand.
	return aead.Seal(out, out[1:headerLen], plaintext, aad)
}

// Open reverses Seal. Every failure returns ErrInvalid so callers cannot tell
// a wrong key from tampering.
func (r *Ring) Open(sealed, aad []byte) ([]byte, error) {
	if len(sealed) < headerLen {
		return nil, ErrInvalid
	}
	aead, ok := r.keys[sealed[0]]
	if !ok {
		return nil, ErrInvalid
	}
	plain, err := aead.Open(nil, sealed[1:headerLen], sealed[headerLen:], aad)
	if err != nil {
		return nil, ErrInvalid
	}
	return plain, nil
}

// HMAC returns HMAC-SHA256 over the parts, each prefixed with its length so
// ("ab","c") and ("a","bc") differ. Used for OTP codes (with the pepper) and IP hashes.
func HMAC(key []byte, parts ...[]byte) []byte {
	mac := hmac.New(sha256.New, key)
	for _, p := range parts {
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(p))) // #nosec G115 -- length of an in-memory slice
		mac.Write(n[:])
		mac.Write(p)
	}
	return mac.Sum(nil)
}

// HashToken is the stored form of a random 32-byte token (refresh and invite
// tokens). A plain SHA-256 is right here: the token has 256 bits of entropy, so
// there is nothing to brute-force and no need for a key.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// Equal compares two secrets in constant time.
func Equal(a, b []byte) bool { return subtle.ConstantTimeCompare(a, b) == 1 }

// RandomDigits returns n uniformly random decimal digits (OTP and delivery codes).
func RandomDigits(n int) string {
	out := make([]byte, 0, n)
	buf := make([]byte, n+8)
	for len(out) < n {
		_, _ = rand.Read(buf)
		for _, b := range buf {
			if b < 250 && len(out) < n { // 250 is a multiple of 10: no modulo bias
				out = append(out, '0'+b%10)
			}
		}
	}
	return string(out)
}
