// Package invite issues and redeems single-use, short-lived invite codes. A
// code lets one machine fetch the shared hub token once; only the SHA-256 of a
// code is ever stored, and a code is never logged or placed in an error.
package invite

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Life is how long an issued invite can be redeemed.
const Life = 10 * time.Minute

const (
	// alphabet is Crockford base32: digits and letters without I, L, O and U.
	alphabet   = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	codeLength = 12
	groupSize  = 4
)

// ErrInvalid is the only redeem failure: unknown, expired, already used and
// malformed codes are deliberately indistinguishable.
var ErrInvalid = errors.New("invite not valid")

// Code is a canonical invite code: exactly codeLength alphabet characters.
// Build one with NewCode or ParseCode; the zero value is not a valid code.
type Code string

// NewCode draws a code from r (crypto/rand.Reader in production). The
// alphabet has 32 symbols, so the low five bits of each byte are uniform.
func NewCode(r io.Reader) (Code, error) {
	raw := make([]byte, codeLength)
	if _, err := io.ReadFull(r, raw); err != nil {
		return "", fmt.Errorf("read random bytes for an invite: %w", err)
	}
	out := make([]byte, codeLength)
	for i, b := range raw {
		out[i] = alphabet[b&31]
	}
	return Code(out), nil
}

// ParseCode normalises what a person typed or pasted: case is ignored, dashes
// and spaces are dropped, O reads as 0 and I or L as 1. Anything that is not
// then exactly codeLength alphabet characters is rejected with ErrInvalid.
func ParseCode(s string) (Code, error) {
	out := make([]byte, 0, codeLength)
	for _, r := range strings.ToUpper(s) {
		switch r {
		case '-', ' ':
			continue
		case 'O':
			r = '0'
		case 'I', 'L':
			r = '1'
		}
		if r > 0x7f || strings.IndexByte(alphabet, byte(r)) < 0 || len(out) == codeLength {
			return "", ErrInvalid
		}
		out = append(out, byte(r))
	}
	if len(out) != codeLength {
		return "", ErrInvalid
	}
	return Code(out), nil
}

// Format groups the code for display, for example K7QF-9M2X-4TBD.
func (c Code) Format() string {
	var b strings.Builder
	for i := 0; i < len(c); i += groupSize {
		if i > 0 {
			b.WriteByte('-')
		}
		b.WriteString(string(c[i:min(i+groupSize, len(c))]))
	}
	return b.String()
}

// String is the grouped form, so a Code printed on purpose is easy to read.
func (c Code) String() string { return c.Format() }

// Hash is the SHA-256 of the canonical characters; stores key on it so the
// code itself never reaches the disk.
func (c Code) Hash() [32]byte { return sha256.Sum256([]byte(c)) }
