// Package userconfig stores the per-user Skill Hub settings of this machine:
// the hub folder that `skillhub init` set up last and the hub folders it
// recorded before. `skillhub connect` reads the hub from here, so a project
// can be connected without naming the hub.
package userconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DirEnv replaces the configuration directory. Tests, the web end-to-end
// suite and installer smoke runs point it at a temporary directory so they
// never touch the real user configuration.
const DirEnv = "SKILLHUB_USER_CONFIG_DIR"

// FileName is the configuration file inside the directory.
const FileName = "config.json"

const (
	maxFileBytes    = 64 << 10
	maxPreviousHubs = 5
)

// Config is the content of the user configuration that Skill Hub reads.
type Config struct {
	// Hub is the absolute path of the hub folder set up last on this machine.
	Hub string
	// PreviousHubs are hub folders recorded before, newest first.
	PreviousHubs []string
}

// Store loads and updates the user configuration.
type Store interface {
	// Load returns the configuration; a missing file is an empty Config.
	Load() (Config, error)
	// SetHub records path as the hub. A different previous hub moves to the
	// front of PreviousHubs.
	SetHub(path string) error
}

// Dir returns the configuration directory: $SKILLHUB_USER_CONFIG_DIR when it
// is set, else <os.UserConfigDir()>/skillhub.
func Dir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv(DirEnv)); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find the user configuration directory: %w; set %s to a writable directory", err, DirEnv)
	}
	return filepath.Join(base, "skillhub"), nil
}

// Default returns the store in the directory Dir names.
func Default() Store {
	dir, err := Dir()
	if err != nil {
		return fileStore{err: err}
	}
	return NewStore(dir)
}

// NewStore returns a store whose file is <dir>/config.json.
func NewStore(dir string) Store {
	return fileStore{dir: dir}
}

type fileStore struct {
	dir string
	err error
}

func (store fileStore) Path() string {
	if store.dir == "" {
		return FileName
	}
	return filepath.Join(store.dir, FileName)
}

func (store fileStore) Load() (Config, error) {
	raw, err := store.read()
	if err != nil {
		return Config{}, err
	}
	return decode(raw), nil
}

func (store fileStore) SetHub(path string) error {
	if store.err != nil {
		return store.err
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("hub path must be absolute: %s", path)
	}
	path = filepath.Clean(path)
	raw, err := store.read()
	if err != nil {
		return err
	}
	current := decode(raw)
	previous := current.PreviousHubs
	if current.Hub != "" && !samePath(current.Hub, path) {
		previous = append([]string{current.Hub}, previous...)
	}
	kept := make([]string, 0, maxPreviousHubs)
	for _, item := range previous {
		if item == "" || samePath(item, path) || containsPath(kept, item) {
			continue
		}
		if len(kept) == maxPreviousHubs {
			break
		}
		kept = append(kept, item)
	}
	raw["hub"] = mustJSON(path)
	raw["previous_hubs"] = mustJSON(kept)
	encoded, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", store.Path(), err)
	}
	return store.write(append(encoded, '\n'))
}

// read returns the raw top-level keys so a write keeps keys this version does
// not know. A missing file is an empty object.
func (store fileStore) read() (map[string]json.RawMessage, error) {
	if store.err != nil {
		return nil, store.err
	}
	file, err := os.Open(store.Path())
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", store.Path(), err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", store.Path(), err)
	}
	if len(data) > maxFileBytes {
		return nil, fmt.Errorf("%s is larger than %d bytes; remove it and run `skillhub init` in your hub folder", store.Path(), maxFileBytes)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	values := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &values); err != nil || values == nil {
		return nil, fmt.Errorf("%s is not valid JSON; remove it and run `skillhub init` in your hub folder", store.Path())
	}
	return values, nil
}

// write replaces the file atomically with a private file.
func (store fileStore) write(data []byte) error {
	if err := os.MkdirAll(store.dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", store.dir, err)
	}
	temporary, err := os.CreateTemp(store.dir, FileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", store.Path(), err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil && runtime.GOOS != "windows" {
		temporary.Close()
		return fmt.Errorf("protect %s: %w", store.Path(), err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write %s: %w", store.Path(), err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("write %s: %w", store.Path(), err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("write %s: %w", store.Path(), err)
	}
	if err := os.Rename(temporaryPath, store.Path()); err != nil {
		return fmt.Errorf("replace %s: %w", store.Path(), err)
	}
	committed = true
	return nil
}

// decode reads the known keys; a key with an unexpected type is ignored.
func decode(raw map[string]json.RawMessage) Config {
	var config Config
	if value, ok := raw["hub"]; ok {
		_ = json.Unmarshal(value, &config.Hub)
	}
	if value, ok := raw["previous_hubs"]; ok {
		_ = json.Unmarshal(value, &config.PreviousHubs)
	}
	return config
}

func mustJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		// Strings and string slices always encode.
		panic(err)
	}
	return encoded
}

func containsPath(paths []string, path string) bool {
	for _, item := range paths {
		if samePath(item, path) {
			return true
		}
	}
	return false
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
