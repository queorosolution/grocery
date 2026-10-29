package crypt

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func key(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func ring(t *testing.T, current byte, versions ...byte) *Ring {
	t.Helper()
	keys := map[byte][]byte{}
	for _, v := range versions {
		keys[v] = key(v)
	}
	r, err := NewRing(current, keys)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSealOpenRoundTrip(t *testing.T) {
	r := ring(t, 1, 1)
	sealed := r.Seal([]byte("4821"), []byte("order-1"))
	if bytes.Contains(sealed, []byte("4821")) {
		t.Fatal("plaintext visible in output")
	}
	got, err := r.Open(sealed, []byte("order-1"))
	if err != nil || string(got) != "4821" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestSealUsesFreshNonce(t *testing.T) {
	r := ring(t, 1, 1)
	if bytes.Equal(r.Seal([]byte("x"), nil), r.Seal([]byte("x"), nil)) {
		t.Fatal("same input produced the same output")
	}
}

func TestOpenRejectsWrongContextAndTampering(t *testing.T) {
	r := ring(t, 1, 1)
	sealed := r.Seal([]byte("4821"), []byte("order-1"))
	if _, err := r.Open(sealed, []byte("order-2")); !errors.Is(err, ErrInvalid) {
		t.Error("value opened for a different owner")
	}
	flipped := append([]byte(nil), sealed...)
	flipped[len(flipped)-1] ^= 1
	if _, err := r.Open(flipped, []byte("order-1")); !errors.Is(err, ErrInvalid) {
		t.Error("tampered value opened")
	}
	if _, err := r.Open([]byte{1, 2, 3}, nil); !errors.Is(err, ErrInvalid) {
		t.Error("short input must be rejected")
	}
}

func TestKeyRotation(t *testing.T) {
	old := ring(t, 1, 1).Seal([]byte("secret"), nil)
	rotated := ring(t, 2, 1, 2)
	if got, err := rotated.Open(old, nil); err != nil || string(got) != "secret" {
		t.Fatalf("old value must open after rotation: %q, %v", got, err)
	}
	if v := rotated.Seal([]byte("new"), nil)[0]; v != 2 {
		t.Fatalf("new values must use version 2, got %d", v)
	}
	if _, err := ring(t, 2, 2).Open(old, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("a ring without version 1 must not open version 1 values")
	}
}

func TestNewRingValidatesKeys(t *testing.T) {
	if _, err := NewRing(1, map[byte][]byte{1: []byte("short")}); !errors.Is(err, ErrKey) {
		t.Error("short key accepted")
	}
	if _, err := NewRing(2, map[byte][]byte{1: key(1)}); err == nil {
		t.Error("missing current version accepted")
	}
}

func TestParseKey(t *testing.T) {
	if _, err := ParseKey("c2hvcnQ="); !errors.Is(err, ErrKey) {
		t.Error("short key accepted")
	}
	if _, err := ParseKey("not base64!"); !errors.Is(err, ErrKey) {
		t.Error("bad base64 accepted")
	}
	good := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	if k, err := ParseKey(good); err != nil || len(k) != 32 {
		t.Errorf("good key rejected: %v", err)
	}
	if _, err := ParseKey("bad key value"); err != nil && strings.Contains(err.Error(), "bad key value") {
		t.Error("error leaks the key")
	}
}

func TestHMACAndEqual(t *testing.T) {
	k := key(7)
	a := HMAC(k, []byte("123456"), []byte("+94771234567"))
	if !Equal(a, HMAC(k, []byte("123456"), []byte("+94771234567"))) {
		t.Fatal("same input must give the same MAC")
	}
	if Equal(a, HMAC(k, []byte("123457"), []byte("+94771234567"))) || Equal(a, HMAC(key(8), []byte("123456"), []byte("+94771234567"))) {
		t.Fatal("different input or key must differ")
	}
	if Equal(HMAC(k, []byte("ab"), []byte("c")), HMAC(k, []byte("a"), []byte("bc"))) {
		t.Fatal("part boundaries must matter")
	}
	if Equal([]byte("a"), []byte("ab")) {
		t.Fatal("different lengths must not be equal")
	}
}

func TestRandomDigits(t *testing.T) {
	counts := [10]int{}
	for i := 0; i < 2000; i++ {
		s := RandomDigits(6)
		if len(s) != 6 {
			t.Fatalf("len %d", len(s))
		}
		for _, c := range s {
			if c < '0' || c > '9' {
				t.Fatalf("not a digit: %q", s)
			}
			counts[c-'0']++
		}
	}
	for d, n := range counts { // 12000 digits: each expected near 1200
		if n < 900 || n > 1500 {
			t.Errorf("digit %d appeared %d times, looks biased", d, n)
		}
	}
}

func TestHashToken(t *testing.T) {
	a, b := HashToken("x"), HashToken("x")
	if !Equal(a, b) || len(a) != 32 || Equal(a, HashToken("y")) {
		t.Fatal("hash must be deterministic, 32 bytes, and differ per token")
	}
}
