package invite

import (
	"crypto/rand"
	"io"
	"time"
)

// Store keeps pending invites by code hash.
type Store interface {
	// Put records an invite that is valid until expires. It may also drop
	// invites that have already expired.
	Put(hash [32]byte, expires time.Time) error
	// Claim atomically takes the invite if it exists and has not expired at
	// now, reporting true for exactly one caller. A missing, expired or
	// already claimed invite is (false, nil).
	Claim(hash [32]byte, now time.Time) (bool, error)
}

// Service issues and redeems invites. It is safe for concurrent use when its
// Store is.
type Service struct {
	store Store
	now   func() time.Time
	rand  io.Reader
}

// NewService returns a Service over store. A nil now defaults to time.Now and
// a nil rnd to crypto/rand.Reader; tests pass a fixed clock and reader.
func NewService(store Store, now func() time.Time, rnd io.Reader) *Service {
	if now == nil {
		now = time.Now
	}
	if rnd == nil {
		rnd = rand.Reader
	}
	return &Service{store: store, now: now, rand: rnd}
}

// Issue creates a code valid for Life and returns it with its expiry.
func (s *Service) Issue() (Code, time.Time, error) {
	code, err := NewCode(s.rand)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := s.now().Add(Life)
	if err := s.store.Put(code.Hash(), expires); err != nil {
		return "", time.Time{}, err
	}
	return code, expires, nil
}

// Redeem consumes the invite a person typed. Every failure, including a store
// error, returns ErrInvalid so callers cannot tell the reasons apart.
func (s *Service) Redeem(code string) error {
	c, err := ParseCode(code)
	if err != nil {
		return ErrInvalid
	}
	ok, err := s.store.Claim(c.Hash(), s.now())
	if err != nil || !ok {
		return ErrInvalid
	}
	return nil
}
