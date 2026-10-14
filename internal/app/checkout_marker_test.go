package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckoutMarkerRoundTripAndVersionGuard(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, found, err := ReadCheckoutMarker(dir); found || err != nil {
		t.Fatalf("empty folder: found=%v err=%v", found, err)
	}
	want := CheckoutMarker{SkillID: "pdf", Hub: "/hub", BaseVersion: "sha256:aa", BaseCommit: "abc", CheckedOutAt: "2026-10-14T10:00:00Z"}
	if err := WriteCheckoutMarker(dir, want); err != nil {
		t.Fatal(err)
	}
	got, found, err := ReadCheckoutMarker(dir)
	if err != nil || !found {
		t.Fatalf("read: found=%v err=%v", found, err)
	}
	want.SchemaVersion = 1
	if got != want {
		t.Fatalf("marker = %+v, want %+v", got, want)
	}
	if err := os.WriteFile(filepath.Join(dir, CheckoutMarkerName), []byte(`{"schema_version":2,"skill_id":"pdf"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadCheckoutMarker(dir); err == nil || !strings.Contains(err.Error(), "schema_version 2") {
		t.Fatalf("unknown schema version must be rejected, got %v", err)
	}
}
