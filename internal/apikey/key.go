// Package apikey generates, hashes, stores, and verifies client API keys.
//
// A key looks like gwp_<id>_<secret>. The id is public and used for lookup;
// the secret is 256 random bits. Only an HMAC-SHA-256 of the full key,
// keyed with a server secret (the pepper), is ever stored.
package apikey

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	Prefix        = "gwp_"
	IDLength      = 8
	SecretBytes   = 32
	SecretLength  = 43 // base64url of 32 bytes, unpadded
	AlgorithmHMAC = "hmac-sha256"

	StatusActive  = "active"
	StatusRevoked = "revoked"
)

var (
	idPattern   = regexp.MustCompile(`^[a-z0-9]{8}$`)
	namePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	b64Pattern  = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

	ErrMalformedKey = errors.New("malformed API key")
)

const idAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// Entry is one stored key. It never holds the plaintext key.
type Entry struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Algorithm string     `json:"algorithm"`
	Hash      string     `json:"hash"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// Usable reports whether the entry is active and not expired at now.
func (e Entry) Usable(now time.Time) bool {
	if e.Status != StatusActive {
		return false
	}
	return e.ExpiresAt == nil || now.Before(*e.ExpiresAt)
}

// Validate checks the fields of an entry.
func (e Entry) Validate() error {
	if !idPattern.MatchString(e.ID) {
		return fmt.Errorf("key id %q must be 8 lowercase letters or digits", e.ID)
	}
	if !namePattern.MatchString(e.Name) {
		return fmt.Errorf("key %s: name %q may only contain letters, digits, '-' and '_'", e.ID, e.Name)
	}
	if e.Algorithm != AlgorithmHMAC {
		return fmt.Errorf("key %s: unsupported algorithm %q", e.ID, e.Algorithm)
	}
	if e.Status != StatusActive && e.Status != StatusRevoked {
		return fmt.Errorf("key %s: status must be %q or %q", e.ID, StatusActive, StatusRevoked)
	}
	raw, err := base64.RawURLEncoding.DecodeString(e.Hash)
	if err != nil || len(raw) != sha256.Size || !b64Pattern.MatchString(e.Hash) {
		return fmt.Errorf("key %s: hash must be unpadded base64url of 32 bytes", e.ID)
	}
	if e.CreatedAt.IsZero() {
		return fmt.Errorf("key %s: created_at is required", e.ID)
	}
	return nil
}

// ValidName reports whether name is an allowed key name.
func ValidName(name string) bool { return namePattern.MatchString(name) }

// NewID returns a random 8-character key id.
func NewID() (string, error) {
	buf := make([]byte, IDLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, IDLength)
	for i, b := range buf {
		out[i] = idAlphabet[int(b)%len(idAlphabet)]
	}
	return string(out), nil
}

// NewSecret returns 256 random bits as unpadded base64url.
func NewSecret() (string, error) {
	buf := make([]byte, SecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// NewPepper returns a new random server secret.
func NewPepper() (string, error) { return NewSecret() }

// Format builds the full key from its id and secret.
func Format(id, secret string) string { return Prefix + id + "_" + secret }

// Parse splits a full key into its id. It does not verify the secret.
func Parse(key string) (id string, err error) {
	if len(key) != len(Prefix)+IDLength+1+SecretLength || !strings.HasPrefix(key, Prefix) {
		return "", ErrMalformedKey
	}
	id = key[len(Prefix) : len(Prefix)+IDLength]
	if key[len(Prefix)+IDLength] != '_' || !idPattern.MatchString(id) {
		return "", ErrMalformedKey
	}
	if !b64Pattern.MatchString(key[len(Prefix)+IDLength+1:]) {
		return "", ErrMalformedKey
	}
	return id, nil
}

// Hash returns the keyed hash of value: unpadded base64url HMAC-SHA-256.
func Hash(pepper, value string) string {
	m := hmac.New(sha256.New, []byte(pepper))
	m.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// PepperFingerprint is a short, non-reversible label for a pepper. The
// server and the apikey command print it, so a pepper mismatch between
// the machine that made the hashes and the server is easy to spot.
func PepperFingerprint(pepper string) string {
	sum := sha256.Sum256([]byte("gwp-pepper-fingerprint:" + pepper))
	return hex.EncodeToString(sum[:4])
}

// HashEqual compares the keyed hash of value with stored in constant time.
func HashEqual(pepper, value, stored string) bool {
	want, err := base64.RawURLEncoding.DecodeString(stored)
	if err != nil {
		want = make([]byte, sha256.Size)
	}
	m := hmac.New(sha256.New, []byte(pepper))
	m.Write([]byte(value))
	return hmac.Equal(m.Sum(nil), want) && err == nil
}

// Generated is a newly created key: the entry to store and the plaintext
// to hand over once.
type Generated struct {
	Entry Entry
	Key   string
}

// Generate creates a new key for name. expiresIn of 0 means it never expires.
func Generate(pepper, name string, expiresIn time.Duration, now time.Time) (Generated, error) {
	if !ValidName(name) {
		return Generated{}, fmt.Errorf("name %q may only contain letters, digits, '-' and '_'", name)
	}
	id, err := NewID()
	if err != nil {
		return Generated{}, err
	}
	secret, err := NewSecret()
	if err != nil {
		return Generated{}, err
	}
	key := Format(id, secret)
	e := Entry{
		ID:        id,
		Name:      name,
		Algorithm: AlgorithmHMAC,
		Hash:      Hash(pepper, key),
		Status:    StatusActive,
		CreatedAt: now.UTC().Truncate(time.Second),
	}
	if expiresIn > 0 {
		t := e.CreatedAt.Add(expiresIn)
		e.ExpiresAt = &t
	}
	return Generated{Entry: e, Key: key}, nil
}
