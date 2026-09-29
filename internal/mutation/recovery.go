package mutation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PathState is the digest-derived state of a transaction path.
type PathState string

const (
	PathBefore  PathState = "before"
	PathAfter   PathState = "after"
	PathUnknown PathState = "unknown"
)

// RecoveryAction is the safe action doctor can take.
type RecoveryAction string

const (
	RecoveryRollForward RecoveryAction = "roll_forward"
	RecoveryAbort       RecoveryAction = "abort_staging"
	RecoveryConflict    RecoveryAction = "manual_conflict"
)

// RecoveryPath describes one canonical path without exposing staged contents.
type RecoveryPath struct {
	Path  string
	State PathState
}

// Recovery describes a pending transaction and its deterministic action.
type Recovery struct {
	OperationID string
	Phase       string
	Action      RecoveryAction
	Paths       []RecoveryPath
	Detail      string
}

type recoveryPlan struct {
	Recovery
	txn      string
	manifest manifest
	err      error
}

// Pending reports transaction directory names without modifying the workspace.
func Pending(root string) ([]string, error) {
	if exists, err := transactionDirectoryExists(root); err != nil {
		return nil, err
	} else if !exists {
		return []string{}, nil
	}
	lock, err := AcquireSharedLock(context.Background(), root, DefaultLockTimeout)
	if err != nil {
		return nil, err
	}
	defer lock.Unlock()
	return pendingUnlocked(root)
}

func transactionDirectoryExists(root string) (bool, error) {
	info, err := os.Lstat(filepath.Join(root, ".skillhub", "transactions"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return false, fmt.Errorf("invalid transaction directory")
	}
	return true, nil
}

func pendingUnlocked(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, ".skillhub", "transactions"))
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() || !validOpaqueID(entry.Name()) {
			return nil, fmt.Errorf("invalid transaction entry: %s", entry.Name())
		}
		ids = append(ids, entry.Name())
	}
	sort.Strings(ids)
	return ids, nil
}

// InspectRecovery returns the same read-only classification doctor --fix uses.
func InspectRecovery(root string) ([]Recovery, error) {
	if exists, err := transactionDirectoryExists(root); err != nil {
		return nil, err
	} else if !exists {
		return []Recovery{}, nil
	}
	lock, err := AcquireSharedLock(context.Background(), root, DefaultLockTimeout)
	if err != nil {
		return nil, err
	}
	defer lock.Unlock()
	return InspectRecoveryWhileLocked(root)
}

// InspectRecoveryWhileLocked classifies recovery while the caller holds a
// shared or exclusive workspace lock. It avoids recursively acquiring flock,
// which is not portable across operating systems.
func InspectRecoveryWhileLocked(root string) ([]Recovery, error) {
	plans, err := preflightRecovery(root, false)
	if err != nil {
		return nil, err
	}
	result := make([]Recovery, 0, len(plans))
	for _, plan := range plans {
		result = append(result, plan.Recovery)
	}
	return result, nil
}

// RollForward completes every approved prepared transaction. Transactions that
// failed before durable preparation are safely aborted only while every path is
// still at its before image. No write occurs until every transaction and staged
// image has passed preflight.
func RollForward(root string) error {
	return RollForwardWithOptions(root, Options{})
}

// RollForwardWithOptions completes canonical recovery and, when configured,
// republishes the exact recovered snapshot before removing the WAL.
func RollForwardWithOptions(root string, options Options) error {
	lock, err := AcquireExclusiveLock(context.Background(), root, DefaultLockTimeout)
	if err != nil {
		return err
	}
	defer lock.Unlock()
	plans, err := preflightRecovery(root, true)
	if err != nil {
		return err
	}
	for _, plan := range plans {
		if plan.Action == RecoveryAbort {
			if err := removeTransaction(plan.txn); err != nil {
				return err
			}
			continue
		}
		for _, item := range plan.manifest.Files {
			if err := applyStagedChecked(root, plan.txn, item, Options{}); err != nil {
				return err
			}
		}
		if err := validateApplied(root, plan.manifest); err != nil {
			return fmt.Errorf("recovered transaction %s is invalid: %w", plan.OperationID, err)
		}
		if plan.manifest.Phase != "catalog_published" {
			plan.manifest.Phase = "canonical_applied"
			if err := writeManifest(plan.txn, plan.manifest); err != nil {
				return err
			}
			if options.PostCanonical != nil {
				published, err := options.PostCanonical(plan.manifest.ExpectedResultCatalogSnapshot)
				if err != nil {
					return fmt.Errorf("recover publication for transaction %s: %w", plan.OperationID, err)
				}
				if published.CatalogSnapshot != plan.manifest.ExpectedResultCatalogSnapshot || published.Generation == "" {
					return fmt.Errorf("%w: recovered publication does not match transaction %s", ErrRecoveryRequired, plan.OperationID)
				}
				plan.manifest.PublishedCatalogSnapshot = published.CatalogSnapshot
				plan.manifest.PublishedGeneration = published.Generation
				plan.manifest.Phase = "catalog_published"
				if err := writeManifest(plan.txn, plan.manifest); err != nil {
					return err
				}
			}
		}
		if err := removeTransaction(plan.txn); err != nil {
			return err
		}
	}
	return nil
}

// RollBack explicitly restores all pending transactions. Mixed before/after
// progress is allowed only when all paths are recognized and every required
// before-image is present and digest-valid. Preflight covers all transactions
// before the first restoration write.
func RollBack(root string) error {
	lock, err := AcquireExclusiveLock(context.Background(), root, DefaultLockTimeout)
	if err != nil {
		return err
	}
	defer lock.Unlock()
	plans, err := preflightRollback(root)
	if err != nil {
		return err
	}
	for planIndex := len(plans) - 1; planIndex >= 0; planIndex-- {
		plan := plans[planIndex]
		if plan.Action == RecoveryAbort {
			if err := removeTransaction(plan.txn); err != nil {
				return err
			}
			continue
		}
		for index := len(plan.manifest.Files) - 1; index >= 0; index-- {
			item := plan.manifest.Files[index]
			actual, err := digestAt(root, item.Path)
			if err != nil {
				return err
			}
			if actual == item.BeforeDigest {
				continue
			}
			if actual != item.AfterDigest {
				if !(actual == "" && item.Displaced != "" && item.BeforeDigest != "") {
					return fmt.Errorf("%w: path %s changed during rollback", ErrRecoveryRequired, item.Path)
				}
				displacedDigest, displacedErr := digestAt(root, item.Displaced)
				if displacedErr != nil || displacedDigest != item.BeforeDigest {
					return fmt.Errorf("%w: displaced target for %s is invalid", ErrRecoveryRequired, item.Path)
				}
			}
			if err := rollbackCanonicalChecked(root, plan.txn, item); err != nil {
				return err
			}
		}
		if err := removeTransaction(plan.txn); err != nil {
			return err
		}
	}
	return nil
}

func preflightRecovery(root string, strict bool) ([]recoveryPlan, error) {
	ids, err := pendingUnlocked(root)
	if err != nil {
		return nil, err
	}
	plans := make([]recoveryPlan, 0, len(ids))
	seenPaths := make(map[string]string)
	for _, id := range ids {
		txn := filepath.Join(root, ".skillhub", "transactions", id)
		m, err := readManifest(txn, id)
		if err != nil {
			if safeEmptyTransaction(txn) {
				plans = append(plans, recoveryPlan{Recovery: Recovery{OperationID: id, Phase: "incomplete", Action: RecoveryAbort, Detail: "No durable manifest or staged files exist; the empty journal can be removed."}, txn: txn})
				continue
			}
			plan := recoveryPlan{Recovery: Recovery{OperationID: id, Phase: "invalid", Action: RecoveryConflict, Detail: err.Error()}, txn: txn, err: err}
			if strict {
				return nil, err
			}
			plans = append(plans, plan)
			continue
		}
		plan := recoveryPlan{Recovery: Recovery{OperationID: id, Phase: m.Phase, Action: RecoveryRollForward}, txn: txn, manifest: m}
		allBefore := true
		for _, item := range m.Files {
			if prior, duplicate := seenPaths[item.Path]; duplicate {
				return nil, fmt.Errorf("%w: transactions %s and %s overlap path %s", ErrRecoveryRequired, prior, id, item.Path)
			}
			seenPaths[item.Path] = id
			actual, digestErr := digestAt(root, item.Path)
			if digestErr != nil {
				return nil, digestErr
			}
			state := classifyDigest(actual, item.BeforeDigest, item.AfterDigest)
			if state == PathUnknown && actual == "" && item.Displaced != "" {
				displacedDigest, displacedErr := digestAt(root, item.Displaced)
				if displacedErr != nil {
					return nil, displacedErr
				}
				if displacedDigest == item.BeforeDigest {
					// The canonical name was safely displaced but publish had not
					// completed. Roll-forward can continue without overwriting.
					state = PathBefore
				}
			}
			plan.Paths = append(plan.Paths, RecoveryPath{Path: item.Path, State: state})
			if state != PathBefore {
				allBefore = false
			}
			if state == PathUnknown {
				plan.Action = RecoveryConflict
				plan.Detail = "A canonical path matches neither the before nor after digest."
			}
		}
		if m.Phase == "staging" {
			if allBefore {
				plan.Action = RecoveryAbort
				plan.Detail = "Preparation did not complete and no canonical path changed."
			} else {
				plan.Action = RecoveryConflict
				plan.Detail = "A staging journal has unexpected canonical changes."
			}
		} else if plan.Action != RecoveryConflict {
			if err := validateStagedImages(txn, m, false); err != nil {
				plan.Action, plan.Detail, plan.err = RecoveryConflict, err.Error(), err
			}
		}
		if plan.Action == RecoveryConflict && strict {
			if plan.err != nil {
				return nil, plan.err
			}
			return nil, fmt.Errorf("%w: transaction %s: %s", ErrRecoveryRequired, id, plan.Detail)
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func preflightRollback(root string) ([]recoveryPlan, error) {
	plans, err := preflightRecovery(root, false)
	if err != nil {
		return nil, err
	}
	for index := range plans {
		plan := &plans[index]
		if plan.Action == RecoveryConflict {
			return nil, fmt.Errorf("%w: transaction %s: %s", ErrRecoveryRequired, plan.OperationID, plan.Detail)
		}
		if plan.Action == RecoveryAbort {
			continue
		}
		if err := validateStagedImages(plan.txn, plan.manifest, true); err != nil {
			return nil, err
		}
	}
	return plans, nil
}

func classifyDigest(actual, before, after string) PathState {
	if actual == before {
		return PathBefore
	}
	if actual == after {
		return PathAfter
	}
	return PathUnknown
}

func readManifest(txn, transactionID string) (manifest, error) {
	var m manifest
	data, err := readStagedFile(txn, "manifest.json")
	if err != nil {
		return m, fmt.Errorf("%w: read transaction manifest %s: %v", ErrRecoveryRequired, transactionID, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return m, fmt.Errorf("%w: parse transaction manifest %s: %v", ErrRecoveryRequired, transactionID, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("trailing JSON value")
		}
		return m, fmt.Errorf("%w: transaction manifest %s has trailing data: %v", ErrRecoveryRequired, transactionID, err)
	}
	if err := validateManifest(m, transactionID); err != nil {
		return m, fmt.Errorf("%w: invalid transaction manifest %s: %v", ErrRecoveryRequired, transactionID, err)
	}
	return m, nil
}

func validateManifest(m manifest, transactionID string) error {
	if m.TransactionVersion != 1 {
		return fmt.Errorf("unsupported transaction version %d", m.TransactionVersion)
	}
	if !validOpaqueID(m.OperationID) || m.OperationID != transactionID {
		return errors.New("operation ID is invalid or does not match the transaction directory")
	}
	if m.Command == "" {
		return errors.New("command is required")
	}
	switch m.Phase {
	case "staging", "prepared", "canonical_applied", "catalog_published", "finalized":
	default:
		return fmt.Errorf("invalid transaction phase %q", m.Phase)
	}
	if len(m.Files) > maxChanges+1 {
		return errors.New("transaction has too many files")
	}
	durablePrepared := m.Phase != "staging"
	if m.Phase == "catalog_published" {
		if !validOptionalDigest(m.PublishedCatalogSnapshot) || m.PublishedCatalogSnapshot == "" || m.PublishedCatalogSnapshot != m.ExpectedResultCatalogSnapshot || !validOpaqueID(m.PublishedGeneration) {
			return errors.New("catalog publication identity is invalid")
		}
	} else if m.PublishedCatalogSnapshot != "" || m.PublishedGeneration != "" {
		return errors.New("catalog publication identity appears before publication phase")
	}
	if durablePrepared && len(m.Files) < 2 {
		return errors.New("prepared transaction requires at least one domain entry and one receipt")
	}
	seen := make(map[string]struct{}, len(m.Files))
	receiptCount := 0
	for index, item := range m.Files {
		if !recoveryPath(item.Path, m.OperationID) {
			return fmt.Errorf("invalid canonical path %q", item.Path)
		}
		if _, duplicate := seen[item.Path]; duplicate {
			return fmt.Errorf("duplicate canonical path %q", item.Path)
		}
		seen[item.Path] = struct{}{}
		if !validOptionalDigest(item.BeforeDigest) || !validOptionalDigest(item.AfterDigest) {
			return fmt.Errorf("invalid digest for %q", item.Path)
		}
		expectedAfter := "after/" + item.Path
		if item.Delete {
			if item.AfterDigest != "" || item.Staged != "" {
				return fmt.Errorf("delete entry %q has an after-image", item.Path)
			}
		} else if item.AfterDigest == "" || item.Staged != expectedAfter {
			return fmt.Errorf("invalid after-image path for %q", item.Path)
		}
		if item.BeforeStaged != "" && (item.BeforeDigest == "" || item.BeforeStaged != "before/"+item.Path) {
			return fmt.Errorf("invalid before-image path for %q", item.Path)
		}
		expectedDisplaced := ".skillhub/transactions/" + m.OperationID + "/displaced/" + item.Path
		if item.Displaced != "" && (item.BeforeDigest == "" || item.Displaced != expectedDisplaced) {
			return fmt.Errorf("invalid displaced-target path for %q", item.Path)
		}
		if durablePrepared && item.BeforeDigest != "" && item.Displaced != expectedDisplaced {
			return fmt.Errorf("prepared transaction is missing displaced-target path for %q", item.Path)
		}
		if item.Receipt {
			receiptCount++
			if index != len(m.Files)-1 || item.Path != receiptPathForOperation(item.Path, m.OperationID) {
				return errors.New("operation receipt must be the final transaction entry and match the operation ID")
			}
		}
	}
	if durablePrepared && receiptCount != 1 {
		return errors.New("prepared transaction requires exactly one final operation receipt")
	}
	return nil
}

func receiptPathForOperation(path, operationID string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 5 && parts[0] == "history" && parts[1] == "operations" && len(parts[2]) == 4 && len(parts[3]) == 2 && allDigits(parts[2]) && allDigits(parts[3]) {
		return strings.Join(parts[:4], "/") + "/" + operationID + ".yaml"
	}
	return ""
}

func recoveryPath(path, operationID string) bool {
	if canonicalPath(path) {
		return true
	}
	parts := strings.Split(path, "/")
	return len(parts) == 5 && parts[0] == "history" && parts[1] == "operations" && len(parts[2]) == 4 && len(parts[3]) == 2 && parts[4] == operationID+".yaml" && allDigits(parts[2]) && allDigits(parts[3])
}

func allDigits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return value != ""
}

func validateStagedImages(txn string, m manifest, rollback bool) error {
	for _, item := range m.Files {
		if !item.Delete {
			contents, err := readStagedFile(txn, item.Staged)
			if err != nil {
				return fmt.Errorf("%w: read after-image for %s: %v", ErrRecoveryRequired, item.Path, err)
			}
			if len(contents) > maxChangeBytes || digest(contents) != item.AfterDigest {
				return fmt.Errorf("%w: staged digest mismatch for %s", ErrRecoveryRequired, item.Path)
			}
		}
		if rollback && item.BeforeDigest != "" {
			if item.BeforeStaged == "" {
				return fmt.Errorf("%w: missing before-image for %s", ErrRecoveryRequired, item.Path)
			}
			contents, err := readStagedFile(txn, item.BeforeStaged)
			if err != nil {
				return fmt.Errorf("%w: read before-image for %s: %v", ErrRecoveryRequired, item.Path, err)
			}
			if len(contents) > maxChangeBytes || digest(contents) != item.BeforeDigest {
				return fmt.Errorf("%w: before-image digest mismatch for %s", ErrRecoveryRequired, item.Path)
			}
		}
	}
	return nil
}

func safeEmptyTransaction(txn string) bool {
	empty := true
	err := filepath.WalkDir(txn, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == txn {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() {
			empty = false
			return filepath.SkipAll
		}
		return nil
	})
	return err == nil && empty
}
