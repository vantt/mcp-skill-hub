package invite

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"
)

type memStore struct {
	mu      sync.Mutex
	invites map[[32]byte]time.Time
	err     error
}

func newMemStore() *memStore { return &memStore{invites: map[[32]byte]time.Time{}} }

func (m *memStore) Put(h [32]byte, exp time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invites[h] = exp
	return m.err
}

func (m *memStore) Claim(h [32]byte, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return false, m.err
	}
	exp, ok := m.invites[h]
	if !ok || !exp.After(now) {
		return false, nil
	}
	delete(m.invites, h)
	return true, nil
}

func newTestService(store Store, clock *time.Time) *Service {
	rnd := bytes.NewReader(bytes.Repeat([]byte{3, 9, 17, 25, 30, 1}, 8))
	return NewService(store, func() time.Time { return *clock }, rnd)
}

func TestServiceIssueThenRedeemOnce(t *testing.T) {
	clock := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	svc := newTestService(newMemStore(), &clock)
	code, exp, err := svc.Issue()
	if err != nil {
		t.Fatal(err)
	}
	if !exp.Equal(clock.Add(Life)) {
		t.Fatalf("expiry = %v, want now + %v", exp, Life)
	}
	if err := svc.Redeem(code.Format()); err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	if err := svc.Redeem(code.String()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("second redeem = %v, want ErrInvalid", err)
	}
}

func TestServiceRedeemFailuresAreAllErrInvalid(t *testing.T) {
	clock := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	store := newMemStore()
	svc := newTestService(store, &clock)
	code, _, _ := svc.Issue()
	other, _ := NewCode(bytes.NewReader(bytes.Repeat([]byte{5}, 12)))

	if err := svc.Redeem("not a code"); err != ErrInvalid {
		t.Errorf("malformed = %v", err)
	}
	if err := svc.Redeem(other.String()); err != ErrInvalid {
		t.Errorf("unknown = %v", err)
	}
	clock = clock.Add(Life)
	if err := svc.Redeem(code.String()); err != ErrInvalid {
		t.Errorf("expired = %v", err)
	}
	clock = clock.Add(-Life)
	store.err = errors.New("disk on fire")
	if err := svc.Redeem(code.String()); err != ErrInvalid {
		t.Errorf("store failure = %v", err)
	}
}

func TestServiceIssueReportsStoreAndRandomFailures(t *testing.T) {
	clock := time.Now()
	store := newMemStore()
	store.err = errors.New("disk on fire")
	if _, _, err := newTestService(store, &clock).Issue(); err == nil {
		t.Error("store failure must fail Issue")
	}
	if _, _, err := NewService(newMemStore(), nil, bytes.NewReader(nil)).Issue(); err == nil {
		t.Error("random failure must fail Issue")
	}
}

func TestNewServiceDefaults(t *testing.T) {
	svc := NewService(newMemStore(), nil, nil)
	code, exp, err := svc.Issue()
	if err != nil || time.Until(exp) <= 0 || time.Until(exp) > Life {
		t.Fatalf("Issue = %v, %v, %v", code, exp, err)
	}
	if err := svc.Redeem(code.String()); err != nil {
		t.Fatal(err)
	}
}
