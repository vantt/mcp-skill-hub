// Package hubtoken stores the shared bearer token that guards the network MCP
// listener. The token lives in one private file (mode 0600 on Unix) outside
// Git; it is never placed in a URL, a log line, telemetry, or command output
// other than an explicit `skillhub token show` on a terminal.
package hubtoken

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// FileName is the token file name inside the workspace config directory. The
// leading dot keeps it apart from the per-skill directories next to it, whose
// names always start with a letter or digit.
const FileName = ".hub-token"

const (
	tokenBytes     = 32
	minTokenLength = 32
	maxFileBytes   = 1024
)

// ErrInsecureMode reports a token file that other users can read or write.
var ErrInsecureMode = errors.New("the token file is accessible to other users")

// ErrInvalidToken reports a token file whose content is not a usable token.
var ErrInvalidToken = errors.New("the token file does not contain a valid token")

// DefaultPath returns the token file of the hub whose workspace root is given:
// <workspace>/runtime/config/.hub-token, next to the per-skill config
// directories that hold other local secrets.
func DefaultPath(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, "runtime", "config", FileName)
}

// ClientPath returns the suggested path of a client machine's copy of the
// token of the hub at hubURL: ~/.skillhub-token-<host>-<port>. One file per
// hub keeps the tokens of different hubs apart. The host keeps only letters,
// digits, dots and dashes (IPv6 brackets and colons are dropped); a URL
// without a port uses its scheme's default. The leading ~ is for a command
// printed for the user's shell on that machine.
func ClientPath(hubURL string) string {
	host, port := "hub", ""
	if parsed, err := url.Parse(strings.TrimSpace(hubURL)); err == nil {
		if name := parsed.Hostname(); name != "" {
			host = name
		}
		port = parsed.Port()
		if port == "" {
			port = map[string]string{"http": "80", "https": "443"}[parsed.Scheme]
		}
	}
	var sanitized strings.Builder
	for _, character := range host {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z', character >= '0' && character <= '9', character == '.', character == '-':
			sanitized.WriteRune(character)
		}
	}
	name := "~/.skillhub-token-" + sanitized.String()
	if port != "" {
		name += "-" + port
	}
	return name
}

// Ensure creates the token file with a new random token when it does not
// exist yet. created reports whether a token was generated. An existing file
// is validated and left unchanged.
func Ensure(path string) (created bool, err error) {
	if _, err := Read(path); err == nil {
		return false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	token, err := generate()
	if err != nil {
		return false, err
	}
	if err := writeExclusive(path, token); err != nil {
		if errors.Is(err, fs.ErrExist) {
			// Another process created it between the check and the write.
			_, readErr := Read(path)
			return false, readErr
		}
		return false, err
	}
	return true, nil
}

// Read returns the token stored at path after checking that the file is a
// regular, private file holding a well-formed token.
func Read(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %s is not a regular file", ErrInvalidToken, path)
	}
	// Windows has no Unix permission bits; the file inherits the ACL of its
	// directory, so it is private only when the workspace is (for example
	// under the user's profile).
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("%w: %s has mode %04o", ErrInsecureMode, path, info.Mode().Perm())
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxFileBytes {
		return "", fmt.Errorf("%w: %s is larger than %d bytes", ErrInvalidToken, path, maxFileBytes)
	}
	token := strings.TrimSpace(string(data))
	if !validToken(token) {
		return "", fmt.Errorf("%w: %s", ErrInvalidToken, path)
	}
	return token, nil
}

// Rotate replaces the token with a new random one. The new file is written
// next to the old one and renamed over it, so a reader sees either the old or
// the new token, never a partial file.
func Rotate(path string) error {
	token, err := generate()
	if err != nil {
		return err
	}
	return Save(path, token)
}

// Save writes token to path as a private file: the folder is created with
// mode 0700 and an older file is replaced in one step, so a reader never sees
// a half-written token. It refuses a string that is not a usable token.
func Save(path, token string) error {
	if !validToken(token) {
		return ErrInvalidToken
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, FileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("create token file: %w", err)
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
		return fmt.Errorf("protect token file: %w", err)
	}
	if _, err := temporary.WriteString(token + "\n"); err != nil {
		temporary.Close()
		return fmt.Errorf("write token file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("write token file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("write token file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace token file: %w", err)
	}
	committed = true
	return nil
}

func generate() (string, error) {
	var raw [tokenBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func writeExclusive(path, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(token + "\n"); err != nil {
		file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write token file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("write token file: %w", err)
	}
	return nil
}

// validToken accepts the generated hex form and any other long token made of
// characters that are safe in an HTTP header.
func validToken(token string) bool {
	if len(token) < minTokenLength || len(token) > maxFileBytes {
		return false
	}
	for _, character := range token {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z', character >= '0' && character <= '9':
		case strings.ContainsRune("-._~+/=", character):
		default:
			return false
		}
	}
	return true
}

// Source serves the current token to a running listener. It rereads the file
// only when the file changed, so `skillhub token rotate` takes effect at the
// next request without a restart, and the old token stops working at once.
type Source struct {
	path string

	mu    sync.Mutex
	info  fs.FileInfo
	token string
}

// NewSource returns a Source for the token file at path.
func NewSource(path string) *Source {
	return &Source{path: path}
}

// Path returns the token file path.
func (source *Source) Path() string { return source.path }

// Check loads the token and reports why it is unusable, if it is.
func (source *Source) Check() error {
	_, err := source.current()
	return err
}

// Valid reports, in constant time over the token bytes, whether presented is
// the current token. An unreadable or missing token file rejects everything.
func (source *Source) Valid(presented string) bool {
	token, err := source.current()
	if err != nil || token == "" || presented == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(token)) == 1
}

func (source *Source) current() (string, error) {
	info, err := os.Stat(source.path)
	if err != nil {
		source.forget()
		return "", err
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	if source.info != nil && os.SameFile(source.info, info) && source.info.ModTime().Equal(info.ModTime()) && source.info.Size() == info.Size() {
		return source.token, nil
	}
	token, err := Read(source.path)
	if err != nil {
		source.info, source.token = nil, ""
		return "", err
	}
	source.info, source.token = info, token
	return token, nil
}

func (source *Source) forget() {
	source.mu.Lock()
	source.info, source.token = nil, ""
	source.mu.Unlock()
}
