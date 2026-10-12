package hostintegration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/vantt/mcp-skill-hub/internal/userconfig"
)

// permissionRecordFile holds, per user, which managed Claude permission rules
// connect itself added to a settings file. `connect --remove` takes out only
// those, so a rule the user wrote before connecting stays. The record lives
// next to the user configuration, never inside Claude's settings file.
const permissionRecordFile = "permission-records.json"

const maxPermissionRecordBytes = 1 << 20

// permissionRecordMutex serializes the read-modify-write of the record file
// within this process; permissionRecordLockFile does the same between
// processes, so two connections made at once do not lose each other's entries.
var permissionRecordMutex sync.Mutex

const (
	permissionRecordLockFile    = "permission-records.lock"
	permissionRecordLockTimeout = 10 * time.Second
	permissionRecordLockRetry   = 20 * time.Millisecond
)

// lockPermissionRecords takes the process mutex and the lock file next to the
// record and returns the function that releases both.
func lockPermissionRecords() (func(), error) {
	permissionRecordMutex.Lock()
	dir, err := userconfig.Dir()
	if err != nil {
		permissionRecordMutex.Unlock()
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		permissionRecordMutex.Unlock()
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	lock := flock.New(filepath.Join(dir, permissionRecordLockFile))
	ctx, cancel := context.WithTimeout(context.Background(), permissionRecordLockTimeout)
	defer cancel()
	locked, err := lock.TryLockContext(ctx, permissionRecordLockRetry)
	if err != nil || !locked {
		permissionRecordMutex.Unlock()
		if err == nil {
			err = context.DeadlineExceeded
		}
		return nil, fmt.Errorf("wait for another skillhub process to finish updating %s: %w", permissionRecordFile, err)
	}
	return func() {
		_ = lock.Unlock()
		permissionRecordMutex.Unlock()
	}, nil
}

// permissionRecords maps "<scope>:<settings file>" to the rules connect added,
// by permission key.
type permissionRecords map[string]map[string][]string

func permissionRecordKey(scope Scope, path string) string {
	return string(scope) + ":" + filepath.Clean(path)
}

func loadPermissionRecords() (permissionRecords, string, error) {
	dir, err := userconfig.Dir()
	if err != nil {
		return nil, "", err
	}
	path := filepath.Join(dir, permissionRecordFile)
	data, err := readPermissionRecordFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return permissionRecords{}, path, nil
	}
	if err != nil {
		return nil, path, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) > maxPermissionRecordBytes {
		return nil, path, fmt.Errorf("%s is larger than %d bytes; remove it to forget which permission rules connect added", path, maxPermissionRecordBytes)
	}
	records := permissionRecords{}
	if len(bytes.TrimSpace(data)) == 0 {
		return records, path, nil
	}
	if err := json.Unmarshal(data, &records); err != nil || records == nil {
		return nil, path, fmt.Errorf("%s is not valid JSON; remove it to forget which permission rules connect added", path)
	}
	return records, path, nil
}

// readPermissionRecordFile reads the record file. Another process replacing the
// file at that moment can make the read fail with a sharing violation on
// Windows, so a failed read is retried briefly.
func readPermissionRecordFile(path string) ([]byte, error) {
	var data []byte
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		data, err = os.ReadFile(path)
		if err == nil || errors.Is(err, fs.ErrNotExist) || runtime.GOOS != "windows" {
			return data, err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return data, err
}

func savePermissionRecords(path string, records permissionRecords) error {
	if len(records) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	encoded, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	temporary, err := os.CreateTemp(dir, permissionRecordFile+".tmp-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil && runtime.GOOS != "windows" {
		_ = temporary.Close()
		return fmt.Errorf("protect %s: %w", path, err)
	}
	if _, err := temporary.Write(append(encoded, '\n')); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	committed = true
	return nil
}

// managedRulesToRemove returns the managed rules `connect --remove` takes out
// of the settings file at path: the ones connect recorded as its own, or every
// managed rule when no record exists (a connection made before records).
func managedRulesToRemove(scope Scope, path string) (map[string][]string, error) {
	unlock, err := lockPermissionRecords()
	if err != nil {
		return nil, err
	}
	defer unlock()
	records, _, err := loadPermissionRecords()
	if err != nil {
		return nil, err
	}
	if recorded, ok := records[permissionRecordKey(scope, path)]; ok {
		return recorded, nil
	}
	all := map[string][]string{}
	for _, rule := range claudeCLIRules {
		all[rule.key] = rule.values
	}
	return all, nil
}

// recordAddedPermissionRules notes the managed rules that applying a settings
// change added to the file whose content was before. Rules already there stay
// the user's. When there is no record yet and the file already holds an ask or
// deny rule (rules nobody writes by hand), the file belongs to an earlier
// connection and no record is made, so removal keeps taking out every managed
// rule.
func recordAddedPermissionRules(scope Scope, path string, before, after []byte) error {
	unlock, err := lockPermissionRecords()
	if err != nil {
		return err
	}
	defer unlock()
	records, recordPath, err := loadPermissionRecords()
	if err != nil {
		return err
	}
	key := permissionRecordKey(scope, path)
	previous, hadRecord := records[key]
	added := map[string][]string{}
	legacy := false
	for _, rule := range claudeCLIRules {
		was, _ := jsonStringArrayAt(before, []string{"permissions", rule.key})
		now, _ := jsonStringArrayAt(after, []string{"permissions", rule.key})
		for _, value := range rule.values {
			switch {
			case !slices.Contains(was, value) && slices.Contains(now, value),
				slices.Contains(was, value) && slices.Contains(previous[rule.key], value):
				added[rule.key] = append(added[rule.key], value)
			case slices.Contains(was, value) && rule.key != "allow":
				legacy = true
			}
		}
	}
	if !hadRecord && legacy {
		return nil
	}
	records[key] = added
	return savePermissionRecords(recordPath, records)
}

// setPermissionRecord replaces the record of a settings file.
func setPermissionRecord(scope Scope, path string, added map[string][]string) error {
	unlock, err := lockPermissionRecords()
	if err != nil {
		return err
	}
	defer unlock()
	records, recordPath, err := loadPermissionRecords()
	if err != nil {
		return err
	}
	records[permissionRecordKey(scope, path)] = added
	return savePermissionRecords(recordPath, records)
}

// forgetPermissionRecord drops the record of a settings file after connect
// removed its rules.
func forgetPermissionRecord(scope Scope, path string) error {
	unlock, err := lockPermissionRecords()
	if err != nil {
		return err
	}
	defer unlock()
	records, recordPath, err := loadPermissionRecords()
	if err != nil {
		return err
	}
	key := permissionRecordKey(scope, path)
	if _, ok := records[key]; !ok {
		return nil
	}
	delete(records, key)
	return savePermissionRecords(recordPath, records)
}
