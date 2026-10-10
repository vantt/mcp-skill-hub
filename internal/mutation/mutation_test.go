package mutation

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

func TestGitDirtyDisablesRepositoryFsmonitorAndHooks(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "executed")
	script := filepath.Join(t.TempDir(), "malicious.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch \""+marker+"\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, setting := range [][]string{{"core.fsmonitor", script}, {"core.hooksPath", filepath.Dir(script)}} {
		command := exec.Command("git", "-C", root, "config", "--local", setting[0], setting[1])
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git config: %v: %s", err, output)
		}
	}
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[core]\n\tfsmonitor = "+script+"\n\thooksPath = "+filepath.Dir(script)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	if _, err := gitDirty(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("repository command executed unsafe config: %v", err)
	}
}

func TestCommitWritesReceiptAndIsPinned(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	receipt, err := Commit(root, WriteSet{OperationID: "OP-1", Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-1.yaml", Contents: []byte("id: SRC-1\n")}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.ChangedPaths) != 2 || receipt.CatalogSnapshot == "" {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	if _, err := os.Stat(filepath.Join(root, "sources/catalog/SRC-1.yaml")); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(root, operationPath(receipt.OperationID, time.Now()))
	receiptBytes, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var rawReceipt map[string]any
	if err := yaml.Unmarshal(receiptBytes, &rawReceipt); err != nil {
		t.Fatal(err)
	}
	for i, ch := range rawReceipt["changes"].([]any) {
		m := ch.(map[string]any)
		if _, ok := m["before_content"]; ok {
			t.Fatalf("receipt change[%d] must not have before_content", i)
		}
		if _, ok := m["after_content"]; ok {
			t.Fatalf("receipt change[%d] must not have after_content", i)
		}
		if _, ok := m["content_available"]; ok {
			t.Fatalf("receipt change[%d] must not have content_available", i)
		}
	}
	retry, err := Commit(root, WriteSet{OperationID: "OP-1", Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-1.yaml", Contents: []byte("id: SRC-1\n")}}})
	if err != nil || retry.OperationID != receipt.OperationID {
		t.Fatalf("idempotent retry = %#v, %v", retry, err)
	}
	if _, err := Commit(root, WriteSet{OperationID: "OP-2", Command: "rewrite", Changes: []Change{{Path: "sources/catalog/SRC-1.yaml", BeforeDigest: "sha256:nope", Contents: []byte("id: SRC-1\nname: changed\n")}}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestRollForwardCompletesPreparedTransaction(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	data := []byte("id: SRC-RECOVER\n")
	injected := errors.New("stop after preparation")
	_, err := CommitWithOptions(root, WriteSet{OperationID: "OP-RECOVER", Command: "create_source", Changes: []Change{{Path: "sources/catalog/SRC-RECOVER.yaml", Contents: data}}}, Options{Fault: func(point FaultPoint) error {
		if point == FaultManifestPhaseUpdate {
			return injected
		}
		return nil
	}})
	if !errors.Is(err, injected) {
		t.Fatalf("commit error = %v", err)
	}
	if err := RollForward(root); err != nil {
		t.Fatal(err)
	}
	if got, err := digestAt(root, "sources/catalog/SRC-RECOVER.yaml"); err != nil || got != digest(data) {
		t.Fatalf("recovery result = %s, %v", got, err)
	}
}

func TestRollForwardRefusesExternalChange(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	path := "sources/catalog/SRC-CONFLICT.yaml"
	if err := os.WriteFile(filepath.Join(root, path), []byte("id: SRC-CONFLICT\nname: external\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	txn := filepath.Join(root, ".skillhub", "transactions", "OP-CONFLICT")
	after := []byte("id: SRC-CONFLICT\nname: desired\n")
	item := change{Path: path, BeforeDigest: digest([]byte("id: SRC-CONFLICT\nname: original\n")), AfterDigest: digest(after), Staged: "after/" + path}
	if err := writeFileSync(txn, item.Staged, after); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(txn, manifest{TransactionVersion: 1, OperationID: "OP-CONFLICT", Command: "rewrite", Phase: "prepared", Files: []change{item}}); err != nil {
		t.Fatal(err)
	}
	if err := RollForward(root); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("expected recovery conflict, got %v", err)
	}
}

func TestRollForwardValidatesEveryStageBeforeApplying(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	txn := filepath.Join(root, ".skillhub", "transactions", "OP-PREFLIGHT")
	firstData := []byte("id: SRC-FIRST\n")
	first := change{Path: "sources/catalog/SRC-FIRST.yaml", AfterDigest: digest(firstData), Staged: "after/sources/catalog/SRC-FIRST.yaml"}
	if err := writeFileSync(txn, first.Staged, firstData); err != nil {
		t.Fatal(err)
	}
	second := change{Path: "sources/catalog/SRC-SECOND.yaml", AfterDigest: digest([]byte("id: SRC-SECOND\n")), Staged: "after/sources/catalog/SRC-SECOND.yaml"}
	m := manifest{TransactionVersion: 1, OperationID: "OP-PREFLIGHT", Command: "create_sources", Phase: "prepared", Files: []change{first, second}}
	appendTestReceipt(t, txn, &m)
	if err := writeManifest(txn, m); err != nil {
		t.Fatal(err)
	}
	if err := RollForward(root); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("expected invalid staged image to require recovery, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(first.Path))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("first path was applied before the later stage was validated: %v", err)
	}
}

func TestCommitRejectsInvalidVirtualTreeWithoutWriting(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sources/catalog", "original.yaml"), []byte("id: SRC-DUP\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Commit(root, WriteSet{OperationID: "OP-BAD", Command: "create_source", Changes: []Change{{Path: "sources/catalog/duplicate.yaml", Contents: []byte("id: SRC-DUP\n")}}})
	if err == nil {
		t.Fatal("invalid virtual tree was accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "sources/catalog", "duplicate.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid mutation wrote canonical path: %v", err)
	}
	if ids, err := Pending(root); err != nil || len(ids) != 0 {
		t.Fatalf("invalid mutation left a recovery journal: %#v, %v", ids, err)
	}
}

func TestCommitDeletesPinnedPath(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	path := "sources/catalog/SRC-DELETE.yaml"
	contents := []byte("id: SRC-DELETE\n")
	if err := os.WriteFile(filepath.Join(root, path), contents, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Commit(root, WriteSet{OperationID: "OP-DELETE", Command: "delete_source", Changes: []Change{{Path: path, BeforeDigest: digest(contents), Delete: true}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted path remains: %v", err)
	}
}

func TestCommitRejectsUnsafeOperationID(t *testing.T) {
	t.Parallel()
	for _, id := range []string{".", "..", "../escape", "OP\nINJECT"} {
		root := filepath.Join(t.TempDir(), "workspace")
		if _, err := workspace.Apply(root); err != nil {
			t.Fatal(err)
		}
		if _, err := Commit(root, WriteSet{OperationID: id, Command: "test", Changes: []Change{{Path: "sources/catalog/a.yaml", Contents: []byte("id: SRC-UNSAFE\n")}}}); err == nil {
			t.Errorf("unsafe ID accepted: %q", id)
		}
		if ids, err := Pending(root); err != nil || len(ids) != 0 {
			t.Errorf("unsafe ID %q left a journal: %#v, %v", id, ids, err)
		}
	}
}

func appendTestReceipt(t *testing.T, txn string, m *manifest) {
	t.Helper()
	occurredAt := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	path := operationPath(m.OperationID, occurredAt)
	contents, err := receiptYAML(filepath.Clean(filepath.Join(txn, "..", "..", "..")), WriteSet{OperationID: m.OperationID, Command: m.Command}, nil, "sha256:test", occurredAt)
	if err != nil {
		t.Fatal(err)
	}
	entry := change{Path: path, AfterDigest: digest(contents), Staged: "after/" + path, Receipt: true}
	if err := writeFileSync(txn, entry.Staged, contents); err != nil {
		t.Fatal(err)
	}
	m.Files = append(m.Files, entry)
}

func TestManifestValidationTable(t *testing.T) {
	t.Parallel()
	domain := change{Path: "sources/catalog/SRC-MANIFEST.yaml", AfterDigest: digest([]byte("id: SRC-MANIFEST\n")), Staged: "after/sources/catalog/SRC-MANIFEST.yaml"}
	receipt := change{Path: "history/operations/2026/09/OP-MANIFEST.yaml", AfterDigest: digest([]byte("receipt")), Staged: "after/history/operations/2026/09/OP-MANIFEST.yaml", Receipt: true}
	tests := []struct {
		name    string
		phase   string
		files   []change
		wantErr bool
	}{
		{name: "staging may be incomplete", phase: "staging", files: nil},
		{name: "prepared empty", phase: "prepared", files: nil, wantErr: true},
		{name: "prepared domain only", phase: "prepared", files: []change{domain}, wantErr: true},
		{name: "prepared receipt only", phase: "prepared", files: []change{receipt}, wantErr: true},
		{name: "receipt not final", phase: "prepared", files: []change{receipt, domain}, wantErr: true},
		{name: "wrong receipt operation", phase: "prepared", files: []change{domain, {Path: "history/operations/2026/09/OP-OTHER.yaml", AfterDigest: digest([]byte("receipt")), Staged: "after/history/operations/2026/09/OP-OTHER.yaml", Receipt: true}}, wantErr: true},
		{name: "valid prepared", phase: "prepared", files: []change{domain, receipt}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateManifest(manifest{TransactionVersion: 1, OperationID: "OP-MANIFEST", Command: "test", Phase: test.phase, Files: test.files}, "OP-MANIFEST")
			if (err != nil) != test.wantErr {
				t.Fatalf("validateManifest error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestRollForwardRefusesMalformedOrTraversalManifestWithoutTouchingSentinel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		manifest []byte
		stage    []byte
	}{
		{
			name:     "malformed JSON",
			manifest: []byte(`{"operation_id":`),
		},
		{
			name: "path traversal",
			manifest: []byte(`{
  "transaction_version": 1,
  "operation_id": "OP-MALICIOUS",
  "command": "rewrite",
  "phase": "prepared",
  "files": [{
    "path": "../sentinel.txt",
    "before": "sha256:70e85898d13a5318b2a0c59dad361eb2d9cd5be94208b5b16a3e1c21cc31c4cb",
    "after": "sha256:8a4a7147af1d34a9f50516b71cf56fe0f7a9cf6784ca816045f53dd1668522b3",
    "staged": "after/payload.txt"
  }]
}`),
			stage: []byte("attacker replacement\n"),
		},
		{
			name: "staged path traversal",
			manifest: []byte(`{
  "transaction_version": 1,
  "operation_id": "OP-MALICIOUS",
  "command": "rewrite",
  "phase": "prepared",
  "files": [{
    "path": "sources/catalog/SRC-MALICIOUS.yaml",
    "before": "",
    "after": "sha256:70e85898d13a5318b2a0c59dad361eb2d9cd5be94208b5b16a3e1c21cc31c4cb",
    "staged": "../../../../sentinel.txt"
  }]
}`),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "workspace")
			if _, err := workspace.Apply(root); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(parent, "sentinel.txt")
			original := []byte("do not touch\n")
			if err := os.WriteFile(sentinel, original, 0o644); err != nil {
				t.Fatal(err)
			}
			txn := filepath.Join(root, ".skillhub", "transactions", "OP-MALICIOUS")
			if err := os.MkdirAll(filepath.Join(txn, "after"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(txn, "manifest.json"), test.manifest, 0o644); err != nil {
				t.Fatal(err)
			}
			if test.stage != nil {
				if err := os.WriteFile(filepath.Join(txn, "after", "payload.txt"), test.stage, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := RollForward(root); err == nil {
				t.Fatal("malicious transaction was accepted")
			}
			got, err := os.ReadFile(sentinel)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(original) {
				t.Fatalf("out-of-root sentinel changed: got %q, want %q", got, original)
			}
		})
	}
}
func TestMutationCancellationWaitingForLockWritesNothing(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}

	// Cancelled context before acquisition
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	set := WriteSet{
		OperationID: "OP-CANCEL-LOCK",
		Command:     "create_source",
		Changes:     []Change{{Path: "sources/catalog/SRC-CANCEL.yaml", Contents: []byte("id: SRC-CANCEL\n")}},
	}
	_, err := CommitWithOptions(root, set, Options{Context: ctx})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Verify nothing written
	if _, err := os.Stat(filepath.Join(root, "sources", "catalog", "SRC-CANCEL.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file should not exist after cancellation: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, ".skillhub", "transactions"))
	if len(entries) != 0 {
		t.Fatalf("no transactions should remain after cancellation before lock, got %d", len(entries))
	}
}

func TestMutationCancellationBeforeCanonicalDisplacementWritesNothing(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	set := WriteSet{
		OperationID: "OP-CANCEL-BEFORE-DISPLACE",
		Command:     "create_source",
		Changes:     []Change{{Path: "sources/catalog/SRC-PRE-DISPLACE.yaml", Contents: []byte("id: SRC-PRE-DISPLACE\n")}},
	}

	// Cancel context when manifest is prepared, right before displacement starts
	options := Options{
		Context: ctx,
		Fault: func(point FaultPoint) error {
			if point == FaultManifestPhaseUpdate {
				cancel()
			}
			return nil
		},
	}

	_, err := CommitWithOptions(root, set, options)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Verify nothing was written to canonical layout
	if _, err := os.Stat(filepath.Join(root, "sources", "catalog", "SRC-PRE-DISPLACE.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonical file must not exist when cancelled before displacement: %v", err)
	}

	// Verify transaction directory was removed
	txnDir := filepath.Join(root, ".skillhub", "transactions", "OP-CANCEL-BEFORE-DISPLACE")
	if _, err := os.Stat(txnDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("transaction directory must be removed on pre-displacement cancellation: %v", err)
	}
}

func TestMutationCancellationAfterDisplacementReachesDurableRecovery(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	set := WriteSet{
		OperationID: "OP-CANCEL-POST-DISPLACE",
		Command:     "create_source",
		Changes:     []Change{{Path: "sources/catalog/SRC-POST-DISPLACE.yaml", Contents: []byte("id: SRC-POST-DISPLACE\n")}},
	}

	// Cancel context during first canonical replace (after displacement has started)
	options := Options{
		Context: ctx,
		Fault: func(point FaultPoint) error {
			if point == FaultFirstCanonicalReplace {
				cancel()
			}
			return nil
		},
	}

	_, err := CommitWithOptions(root, set, options)
	if err == nil {
		t.Fatal("expected error on cancelled mutation")
	}
	if !strings.Contains(err.Error(), "remains recoverable") {
		t.Fatalf("expected outcome indicating transaction remains recoverable, got: %v", err)
	}

	// Verify file was applied to canonical layout
	if _, err := os.Stat(filepath.Join(root, "sources", "catalog", "SRC-POST-DISPLACE.yaml")); err != nil {
		t.Fatalf("canonical file should be applied when cancellation happens post-displacement: %v", err)
	}

	// Verify transaction is in recoverable state and RollForward cleans it up
	recoveries, err := InspectRecoveryWhileLocked(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(recoveries) != 1 || recoveries[0].OperationID != "OP-CANCEL-POST-DISPLACE" {
		t.Fatalf("expected 1 recoverable transaction, got %#v", recoveries)
	}

	if err := RollForward(root); err != nil {
		t.Fatalf("RollForward failed: %v", err)
	}
}
