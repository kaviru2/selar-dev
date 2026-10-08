// Package linktoken issues and checks the bearer tokens embedded in links
// that work without a session: the calendar feed URL and email unsubscribe
// links. A token binds a purpose, a user id and a per-user version number with
// an HMAC; nothing is stored. Revoking every outstanding link of one purpose
// for a user is a matter of incrementing that user's version.
package linktoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"

	"github.com/google/uuid"
)

// Purpose separates token families: a calendar token can never be used as an
// unsubscribe token and vice versa.
type Purpose string

const (
	Calendar    Purpose = "calendar-feed"
	Unsubscribe Purpose = "email-unsubscribe"
)

// ErrInvalid means the token is malformed or its signature is wrong.
var ErrInvalid = errors.New("invalid link token")

const sigLen = 16 // 128-bit truncated HMAC-SHA256

// Signer signs and verifies tokens with keys derived from one secret.
type Signer struct{ secret []byte }

// New returns a signer. secret must be the server's private signing secret
// (JWT_SECRET or LINK_TOKEN_SECRET); an empty secret yields a signer that
// rejects everything.
func New(secret string) *Signer { return &Signer{secret: []byte(secret)} }

func (s *Signer) key(p Purpose) []byte {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte("selar-link-token-v1/" + string(p)))
	return m.Sum(nil)
}

func (s *Signer) mac(p Purpose, body []byte) []byte {
	m := hmac.New(sha256.New, s.key(p))
	m.Write(body)
	return m.Sum(nil)[:sigLen]
}

// Issue returns a URL-safe token for (purpose, userID, version).
func (s *Signer) Issue(p Purpose, userID string, version int) (string, error) {
	if len(s.secret) == 0 {
		return "", errors.New("link token secret not configured")
	}
	id, err := uuid.Parse(userID)
	if err != nil {
		return "", err
	}
	if version < 0 || version > 1<<31-1 {
		return "", errors.New("version out of range")
	}
	body := make([]byte, 20, 20+sigLen)
	copy(body, id[:])
	binary.BigEndian.PutUint32(body[16:], uint32(version))
	return base64.RawURLEncoding.EncodeToString(append(body, s.mac(p, body)...)), nil
}

// Verify checks the signature and returns the user id and version. The
// caller must still compare the version with the user's current one.
func (s *Signer) Verify(p Purpose, token string) (string, int, error) {
	if len(s.secret) == 0 || len(token) > 64 {
		return "", 0, ErrInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 20+sigLen {
		return "", 0, ErrInvalid
	}
	body, sig := raw[:20], raw[20:]
	if !hmac.Equal(sig, s.mac(p, body)) {
		return "", 0, ErrInvalid
	}
	var id uuid.UUID
	copy(id[:], body[:16])
	return id.String(), int(binary.BigEndian.Uint32(body[16:])), nil
}
